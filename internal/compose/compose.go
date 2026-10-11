// Package compose is the composer's producing half: `init` writes a project, `check` runs the one gate over
// it, and `build` makes a package from it.
//
// A project is a working directory: the descriptor, `manifests/` and `components/` are the application and
// are what a bundle holds; everything else beside them is the authoring and is never packaged. A build
// refuses on any refusal the gate reports, archives the bundle so that two builds of one tree produce one
// digest, and runs nothing the project contains.
package compose

import (
	"archive/zip"
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/pkgfile"
)

// The bundle's three entries.
const (
	Descriptor = "yoke.yaml"
	Manifests  = "manifests"
	Components = "components"
)

// epoch is the one date every archive entry carries: the earliest a Zip entry can state.
var epoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// ErrRefused is a build refused because the gate refused the project.
var ErrRefused = errors.New("the project is refused")

// InitOptions are the identity fields `init` writes besides the identifier. An empty one takes its
// default: this host's architecture, version 0.1.0, and the Core from 0.5, the first with the application
// form.
type InitOptions struct {
	Arch    string
	Version string
	Core    string
}

// Init writes a project into dir: a descriptor stating the five identity fields, and `manifests/` and
// `components/` beside it. A descriptor already there is refused and left as it is.
func Init(dir, id string, o InitOptions) error {
	if !gate.IsIdentifier(id) {
		return fmt.Errorf("the id %q is not a namespaced identifier of dot-separated lowercase labels", id)
	}
	if o.Arch == "" {
		o.Arch = hostArch()
	}
	if o.Version == "" {
		o.Version = "0.1.0"
	}
	if o.Core == "" {
		o.Core = ">=0.5"
	}
	descriptor := filepath.Join(dir, Descriptor)
	if _, err := os.Lstat(descriptor); err == nil {
		return fmt.Errorf("%s is already there: a project is made once", descriptor)
	}
	for _, sub := range []string{Manifests, Components} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	body := fmt.Sprintf("model: %d\nid: %s\nversion: %s\narch: %s\ncore: %q\n", gate.DescriptionModel, id, o.Version, o.Arch, o.Core)
	f, err := os.OpenFile(descriptor, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// hostArch is this host's machine name, as `uname -m` reports it.
func hostArch() string {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return ""
	}
	return unix.ByteSliceToString(u.Machine[:])
}

// Check runs the gate over the project in dir, composing: the descriptor, joined against the Manifests in
// `manifests/`. Host facts are not checked on a machine that is not the target.
func Check(dir string) gate.Report {
	r, _ := check(dir)
	return r
}

func check(dir string) (gate.Report, *gate.Deployment) {
	return gate.Check(gate.Input{
		Document:  gate.Read(filepath.Join(dir, Descriptor), gate.Descriptor),
		Moment:    gate.Composing,
		Manifests: filepath.Join(dir, Manifests),
	})
}

// BuildOptions are what a build is given besides the project.
type BuildOptions struct {
	// Out is the directory the package is written into.
	Out string
	// Sign, where given, signs the metadata; without it the package is unsigned, the development case.
	Sign crypto.Signer
	// Seal, where given, is the 32-byte credential the payload is sealed with.
	Seal []byte
	// Now dates the build; it is carried in the footer and checked by nothing.
	Now func() time.Time
}

// Build makes a package from the project in dir and returns its path, with the gate's report. Nothing is
// written unless every step succeeded.
func Build(dir string, o BuildOptions) (string, gate.Report, error) {
	report, d := check(dir)
	if report.Refused() {
		var lines []string
		for _, f := range report.Findings {
			if f.Class == gate.Refusal {
				lines = append(lines, fmt.Sprintf("%s at %s: %s", f.Code, f.Location, f.Message))
			}
		}
		return "", report, fmt.Errorf("%w: %s", ErrRefused, strings.Join(lines, "; "))
	}
	for _, name := range d.Order {
		if exec := d.Units[name].Exec; exec != "" {
			if err := inBundle(dir, name, exec); err != nil {
				return "", report, err
			}
		}
	}
	payload, err := archive(dir)
	if err != nil {
		return "", report, err
	}
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	h := d.Head
	var pkg bytes.Buffer
	err = pkgfile.Write(&pkg, payload, pkgfile.Options{Model: uint16(h.Model), ID: h.ID, Version: h.Version,
		Arch: h.Arch, Core: h.Core, BuiltAt: now(), Seal: o.Seal, Sign: o.Sign})
	if err != nil {
		return "", report, err
	}
	out := filepath.Join(o.Out, pkgfile.Name(h.ID, h.Version))
	tmp, err := os.CreateTemp(o.Out, ".build-*")
	if err != nil {
		return "", report, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(pkg.Bytes()); err != nil {
		tmp.Close()
		return "", report, err
	}
	if err := tmp.Close(); err != nil {
		return "", report, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return "", report, err
	}
	return out, report, os.Rename(tmp.Name(), out)
}

// inBundle requires a unit's program to be a file under `components/`, named by a clean relative path.
func inBundle(dir, unit, exec string) error {
	clean := path.Clean(exec)
	if path.IsAbs(exec) || clean != exec || !strings.HasPrefix(clean, Components+"/") {
		return fmt.Errorf("the unit %s names %s, which is outside the bundle: a program is a path under %s/", unit, exec, Components)
	}
	info, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(clean)))
	if err != nil {
		return fmt.Errorf("the unit %s names %s, which is not there", unit, exec)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("the unit %s names %s, which is not a regular file", unit, exec)
	}
	return nil
}

// archive writes the bundle as a Zip archive, deterministically: the entries in order of their names, each
// dated at one constant, carrying no attribute but whether it is executable.
func archive(dir string) ([]byte, error) {
	names := []string{Descriptor}
	for _, sub := range []string{Manifests, Components} {
		root := filepath.Join(dir, sub)
		err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) && p == root {
					return nil
				}
				return err
			}
			if e.IsDir() {
				return nil
			}
			if !e.Type().IsRegular() {
				return fmt.Errorf("%s is not a regular file, and a bundle holds only files", p)
			}
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(names)
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, name := range names {
		p := filepath.Join(dir, filepath.FromSlash(name))
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		mode := os.FileMode(0o644)
		if info.Mode().Perm()&0o111 != 0 {
			mode = 0o755
		}
		h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: epoch}
		h.SetMode(mode)
		w, err := z.CreateHeader(h)
		if err != nil {
			return nil, err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(content); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
