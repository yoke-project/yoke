// Package packages makes the definitions' crate and wheel: it generates their sources, tests them, and
// packages them at a version, each in a container image the repository pins. The work is the
// definitions' script's; this package runs it, and reads what a package carries.
package packages

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// A language the definitions are packaged for.
type Language string

const (
	Rust   Language = "rust"
	Python Language = "python"
)

// The definitions' script, relative to the repository's root.
const script = "ci/definitions.sh"

// run runs the definitions' script at root, and returns what it said when it fails.
func run(root string, args ...string) error {
	command := exec.Command("bash", append([]string{script}, args...)...)
	command.Dir = root
	if said, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %v\n%s", script, strings.Join(args, " "), err, said)
	}
	return nil
}

// Generate writes the sources the definitions generate for lang into out.
func Generate(root string, lang Language, out string) error {
	// The script resolves a relative directory against the definitions, so it is given an absolute one.
	out, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	return run(root, "generate-"+string(lang), out)
}

// Test builds the package for lang and runs its tests.
func Test(root string, lang Language) error { return run(root, "test-"+string(lang)) }

// Package packages both at version into out, and returns the crate's path and the wheel's.
func Package(root, version, out string) (crate, wheel string, err error) {
	if out, err = filepath.Abs(out); err != nil {
		return "", "", err
	}
	if err := run(root, "package", version, out); err != nil {
		return "", "", err
	}
	crate = filepath.Join(out, fmt.Sprintf("yoke-proto-%s.crate", version))
	wheel = filepath.Join(out, fmt.Sprintf("yoke_proto-%s-py3-none-any.whl", version))
	for _, path := range []string{crate, wheel} {
		if _, err := os.Stat(path); err != nil {
			return "", "", fmt.Errorf("packaging wrote no %s", filepath.Base(path))
		}
	}
	return crate, wheel, nil
}

// CrateSources reads the files a crate carries, keyed by their path inside it: a crate is a gzipped tar
// whose every entry sits under `<name>-<version>/`, which the key leaves out.
func CrateSources(path string) (map[string][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	unzipped, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("%s is not a crate: %v", path, err)
	}
	files := map[string][]byte{}
	archive := tar.NewReader(unzipped)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return files, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s is not a crate: %v", path, err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		_, name, found := strings.Cut(header.Name, "/")
		if !found {
			return nil, fmt.Errorf("%s carries %s outside its package directory", path, header.Name)
		}
		data, err := io.ReadAll(archive)
		if err != nil {
			return nil, err
		}
		files[name] = data
	}
}

// WheelSources reads the files a wheel carries, keyed by their path inside it.
func WheelSources(path string) (map[string][]byte, error) {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%s is not a wheel: %v", path, err)
	}
	defer archive.Close()
	files := map[string][]byte{}
	for _, f := range archive.File {
		if f.FileInfo().IsDir() {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			return nil, err
		}
		files[f.Name] = data
	}
	return files, nil
}

// An image reference: a registry host, a repository, and a tag, a digest or both.
var image = regexp.MustCompile(`\b(?:docker\.io|ghcr\.io|quay\.io|registry\.[a-z.]+)/[a-z0-9._/-]+(?::[A-Za-z0-9._-]+)?(?:@sha256:[0-9a-f]+)?`)

// Images returns every container image a script names.
func Images(script string) []string { return image.FindAllString(script, -1) }

// Carried reads the sources a package file carries — what the tree put in it — leaving out what the
// packaging tool writes of its own: cargo's normalised manifest, its lockfile and its note of the
// checkout, and a wheel's record of its files and its note of the tool that built it.
func Carried(lang Language, path string) (map[string][]byte, error) {
	read, left := CrateSources, func(name string) bool {
		return name == "Cargo.toml" || name == "Cargo.lock" || name == ".cargo_vcs_info.json"
	}
	if lang == Python {
		read, left = WheelSources, func(name string) bool {
			return strings.HasSuffix(name, ".dist-info/RECORD") || strings.HasSuffix(name, ".dist-info/WHEEL")
		}
	}
	files, err := read(path)
	if err != nil {
		return nil, err
	}
	for name := range files {
		if left(name) {
			delete(files, name)
		}
	}
	return files, nil
}
