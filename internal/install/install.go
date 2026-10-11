// Package install is installing a package, in its order: everything that can be decided without writing
// is decided first, and the first thing written is the record that says the installation is not
// finished. Nothing the package contains is executed.
package install

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/pkgfile"
	"github.com/yoke-project/yoke/internal/version"
)

// Options are what an installation is given.
type Options struct {
	Package    string // the package file
	Root       string // the install root
	Keys       pkgfile.Keys
	Policy     pkgfile.Policy
	Credential []byte // the seal credential, where there is one
	Host       pkgfile.Host
	Now        func() time.Time
	// Engine reaches the container engine; it is asked for only where a unit names an image.
	Engine func(ctx context.Context) (*engine.Engine, error)
	// Out is where obtaining images reports its progress.
	Out io.Writer
}

// Result is where an installation is, and whether the version was there already.
type Result struct {
	Dir     string
	Present bool
}

// Refusal is an installation refused: the answer will not change until something is changed.
type Refusal struct{ err error }

func (r *Refusal) Error() string { return r.err.Error() }
func (r *Refusal) Unwrap() error { return r.err }

func refuse(format string, args ...any) error { return &Refusal{fmt.Errorf(format, args...)} }

// refusing wraps a package's refusal, which every one of its checks gives, as an installation's.
func refusing(err error) error {
	var r *pkgfile.Refusal
	if errors.As(err, &r) {
		return &Refusal{err}
	}
	return err
}

// Install installs the package in the order the steps are numbered. A refusal before the version's
// directory is made leaves nothing behind; one after it leaves the version incomplete, and nothing that
// was already installed is touched.
func Install(o Options) (Result, error) {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	// 1 · the footer, against the trust store and this host, holding nothing but its last 512 bytes.
	file, err := os.Open(o.Package)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Result{}, err
	}
	footer, err := pkgfile.ReadFooter(file, info.Size())
	if err != nil {
		return Result{}, refusing(err)
	}
	if err := footer.Check(o.Keys, o.Policy, o.Host); err != nil {
		return Result{}, refusing(err)
	}
	m := footer.Meta
	// 2 · the root's properties.
	root, err := installroot.Check(o.Root)
	if err != nil {
		return Result{}, &Refusal{err}
	}
	// 3, 4 · the version's directory, made exclusively, and its record, incomplete.
	inst, err := root.Begin(m.ID, m.Version)
	switch {
	case errors.Is(err, installroot.ErrPresent):
		return Result{Dir: filepath.Join(o.Root, m.ID, m.Version), Present: true}, nil
	case errors.Is(err, installroot.ErrInProgress):
		return Result{}, &Refusal{err}
	case err != nil:
		return Result{}, err
	}
	res := Result{Dir: inst.Dir}
	begun, err := installroot.ReadRecord(inst.Dir)
	if err != nil {
		return res, err
	}
	// 5, 6 · unsealed where sealed, and the digest recomputed: nothing is used before it holds.
	payload, err := footer.Open(file, o.Credential)
	if err != nil {
		return res, refusing(err)
	}
	// 7 · the bundle, expanded, with nothing outside it.
	bundle := filepath.Join(inst.Dir, "bundle")
	if err := expand(payload, bundle); err != nil {
		return res, err
	}
	// 8 · the descriptor, read for the first time, and gated.
	report, d := gate.Check(gate.Input{Document: gate.Read(filepath.Join(bundle, "yoke.yaml"), gate.Descriptor),
		Moment: gate.Installing, Manifests: filepath.Join(bundle, "manifests")})
	if report.Refused() {
		var lines []string
		for _, f := range report.Findings {
			if f.Class == gate.Refusal {
				lines = append(lines, fmt.Sprintf("%s at %s: %s", f.Code, f.Location, f.Message))
			}
		}
		return res, refuse("the descriptor is refused: %s", strings.Join(lines, "; "))
	}
	if h := d.Head; h.ID != m.ID || h.Version != m.Version || h.Arch != m.Arch || h.Core != m.Core || uint16(h.Model) != m.Model {
		return res, refuse("the descriptor says it is %s %s for %s, Core %s, model %d, and the footer that was checked says %s %s for %s, Core %s, model %d",
			h.ID, h.Version, h.Arch, h.Core, h.Model, m.ID, m.Version, m.Arch, m.Core, m.Model)
	}
	// 9 · images, by digest: loaded where the package carries them, pulled where it does not.
	if err := obtain(o, bundle, d, m); err != nil {
		return res, err
	}
	// 10 · every component the descriptor names, against the digest it states; executables made so.
	if err := components(bundle, d); err != nil {
		return res, err
	}
	// 11 · complete.
	verified := "unsigned"
	if m.SigID != pkgfile.SigNone {
		verified = hex.EncodeToString(m.KeyID[:])
	}
	err = inst.Write(installroot.Record{State: installroot.Complete, InstalledBy: begun.InstalledBy, InstalledAt: now().UTC(),
		VerifiedBy: verified, Sealed: m.SealID != pkgfile.SealNone, Digest: "sha256:" + hex.EncodeToString(m.Digest[:]), Shared: true})
	return res, err
}

// obtain holds every image the descriptor names under its pinned reference, tagged in the reserved
// namespace. The engine is reached only where a unit names an image, and once.
func obtain(o Options, bundle string, d *gate.Deployment, m pkgfile.Meta) error {
	var refs []string
	for _, name := range d.Order {
		if ref := d.Units[name].Image; ref != "" && !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	ctx := context.Background()
	if o.Engine == nil {
		return refuse("engine.unreachable: the descriptor names containerised units, and no engine is configured")
	}
	e, err := o.Engine(ctx)
	if err != nil {
		return refuse("engine.unreachable: the descriptor names containerised units, and the engine cannot be reached: %v", err)
	}
	for _, ref := range refs {
		held, err := e.HasImage(ctx, ref)
		if err != nil {
			return err
		}
		switch archive := carried(bundle, ref); {
		case held:
			fmt.Fprintf(out, "%s: already held\n", ref)
		case archive != "":
			f, err := os.Open(archive)
			if err != nil {
				return err
			}
			err = e.Load(ctx, f, func(line string) { fmt.Fprintf(out, "%s: %s\n", ref, line) })
			f.Close()
			if err != nil {
				return err
			}
			if held, err := e.HasImage(ctx, ref); err != nil {
				return err
			} else if !held {
				return refuse("image.undigested: the engine loaded the archive of %s and holds no image under its digest: its store keeps no digest for an image loaded from an archive", ref)
			}
			fmt.Fprintf(out, "%s: loaded from the package\n", ref)
		default:
			if err := e.Pull(ctx, ref, func(line string) { fmt.Fprintf(out, "%s: %s\n", ref, line) }); err != nil {
				return err
			}
			if held, err := e.HasImage(ctx, ref); err != nil {
				return err
			} else if !held {
				return refuse("image.unavailable: %s was pulled and the engine holds no image under it", ref)
			}
			fmt.Fprintf(out, "%s: pulled\n", ref)
		}
		if err := e.Tag(ctx, ref, m.ID, m.Version); err != nil {
			return err
		}
	}
	return nil
}

// carried is the archive the bundle carries for an image, `components/images/<algorithm>-<hex>.tar`, or
// nothing where it carries none.
func carried(bundle, ref string) string {
	at := strings.LastIndex(ref, "@")
	if at < 0 {
		return ""
	}
	p := filepath.Join(bundle, "components", "images", strings.Replace(ref[at+1:], ":", "-", 1)+".tar")
	if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
		return p
	}
	return ""
}

// expand writes the bundle's entries under dir, refusing one that would leave it, that is not a regular
// file or a directory, or that is not one of the bundle's three entries.
func expand(payload []byte, dir string) error {
	z, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return refuse("the payload is not a Zip archive: %v", err)
	}
	for _, e := range z.File {
		name := e.Name
		clean := path.Clean(name)
		switch {
		case path.IsAbs(name) || clean == ".." || strings.HasPrefix(clean, "../"):
			return refuse("the entry %s would leave the bundle", name)
		case clean != "yoke.yaml" && !strings.HasPrefix(clean, "manifests/") && !strings.HasPrefix(clean, "components/"):
			return refuse("the entry %s is none of the bundle's three: yoke.yaml, manifests/, components/", name)
		case e.Mode()&os.ModeSymlink != 0 || (!e.Mode().IsRegular() && !e.Mode().IsDir()):
			return refuse("the entry %s is not a regular file", name)
		}
		target := filepath.Join(dir, filepath.FromSlash(clean))
		if e.Mode().IsDir() {
			if err := os.MkdirAll(target, 0o775); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o775); err != nil {
			return err
		}
		r, err := e.Open()
		if err != nil {
			return refuse("the entry %s cannot be read: %v", name, err)
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			r.Close()
			return err
		}
		_, err = io.Copy(out, r)
		r.Close()
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
		if err := os.Chmod(target, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// components recomputes the digest of every executable and Manifest the descriptor states one for, and
// makes each executable so.
func components(bundle string, d *gate.Deployment) error {
	for _, name := range d.Order {
		u := d.Units[name]
		if u.Exec != "" {
			p := filepath.Join(bundle, filepath.FromSlash(u.Exec))
			if err := matches(p, u.Digest); err != nil {
				return refuse("component.digest at units.%s.exec: %s %v", name, u.Exec, err)
			}
			if err := os.Chmod(p, 0o755); err != nil {
				return err
			}
		}
		if u.ManifestDigest != "" {
			p := gate.BundleManifestPath(filepath.Join(bundle, "manifests"), u.Plugin)
			if err := matches(p, u.ManifestDigest); err != nil {
				return refuse("plugin.manifest.digest at units.%s.manifest_digest: %s %v", name, u.Plugin, err)
			}
		}
	}
	return nil
}

func matches(p, digest string) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return fmt.Errorf("is not there")
	}
	sum := sha256.Sum256(data)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != digest {
		return fmt.Errorf("is %s, and the descriptor states %s", got, digest)
	}
	return nil
}

// CoreVersion is the version a Core binary states in the build information stamped into it, read
// without executing it.
func CoreVersion(binary string) (version.Version, error) {
	info, err := buildinfo.ReadFile(binary)
	if err != nil {
		return version.Version{}, fmt.Errorf("the Core %s states no version: %v", binary, err)
	}
	v, err := version.Parse(strings.TrimPrefix(info.Main.Version, "v"))
	if err != nil {
		return version.Version{}, fmt.Errorf("the Core %s states no version a range can hold: %q", binary, info.Main.Version)
	}
	return v, nil
}
