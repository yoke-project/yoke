package packages_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/packages"
)

// The repository's root, from this package's directory.
const root = "../.."

// sameTree reports, both ways, where the files under got and want differ.
func sameTree(t *testing.T, got, want map[string][]byte, what string) {
	t.Helper()
	if len(got) == 0 {
		t.Fatalf("%s: nothing was generated", what)
	}
	for name, data := range got {
		committed, ok := want[name]
		switch {
		case !ok:
			t.Errorf("%s: %s is generated and not committed", what, name)
		case !bytes.Equal(committed, data):
			t.Errorf("%s: %s is committed and differs from what the definitions generate", what, name)
		}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("%s: %s is committed and nothing generates it", what, name)
		}
	}
}

// files reads every file under dir, keyed by its path relative to dir.
func files(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	found := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(dir, path)
		found[filepath.ToSlash(relative)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// std: yoke:the-definitions-packages.01
func TestTheRustBuiltFromTheDefinitionsIsCommittedAndCurrent(t *testing.T) {
	out := t.TempDir()
	if err := packages.Generate(root, packages.Rust, out); err != nil {
		t.Fatal(err)
	}
	sameTree(t, files(t, out), files(t, filepath.Join(root, "proto", "rust", "src", "gen")), "the crate")
}

// std: yoke:the-definitions-packages.02
func TestThePythonBuiltFromTheDefinitionsIsCommittedAndCurrent(t *testing.T) {
	out := t.TempDir()
	if err := packages.Generate(root, packages.Python, out); err != nil {
		t.Fatal(err)
	}
	sameTree(t, files(t, out), files(t, filepath.Join(root, "proto", "python", "src")), "the wheel")
}

// std: yoke:the-definitions-packages.03
func TestTheCrateBuildsAndEncodesAsTheGoDoes(t *testing.T) {
	if err := packages.Test(root, packages.Rust); err != nil {
		t.Fatal(err)
	}
}

// std: yoke:the-definitions-packages.04
func TestTheWheelInstallsAndEncodesAsTheGoDoes(t *testing.T) {
	if err := packages.Test(root, packages.Python); err != nil {
		t.Fatal(err)
	}
}

// std: yoke:the-definitions-packages.05
func TestEachPackageIsNamedVersionedLicensedAndCarriesTheTree(t *testing.T) {
	out := t.TempDir()
	crate, wheel, err := packages.Package(root, "0.1.0", out)
	if err != nil {
		t.Fatal(err)
	}
	licence, _ := os.ReadFile(filepath.Join(root, "LICENSE"))
	notice, _ := os.ReadFile(filepath.Join(root, "NOTICE"))

	if filepath.Base(crate) != "yoke-proto-0.1.0.crate" {
		t.Errorf("the crate is %s, want yoke-proto-0.1.0.crate", filepath.Base(crate))
	}
	carried, err := packages.CrateSources(crate)
	if err != nil {
		t.Fatal(err)
	}
	manifest := string(carried["Cargo.toml.orig"])
	for _, line := range []string{`name = "yoke-proto"`, `version = "0.1.0"`, `license = "Apache-2.0"`} {
		if !strings.Contains(manifest, line) {
			t.Errorf("the crate's manifest does not say %s:\n%s", line, manifest)
		}
	}
	if !bytes.Equal(carried["LICENSE"], licence) || !bytes.Equal(carried["NOTICE"], notice) {
		t.Errorf("the crate does not carry the repository's LICENSE and NOTICE")
	}
	for name, data := range files(t, filepath.Join(root, "proto", "rust", "src")) {
		if !bytes.Equal(carried["src/"+name], data) {
			t.Errorf("the crate does not carry src/%s as the tree holds it", name)
		}
	}

	if filepath.Base(wheel) != "yoke_proto-0.1.0-py3-none-any.whl" {
		t.Errorf("the wheel is %s, want yoke_proto-0.1.0-py3-none-any.whl", filepath.Base(wheel))
	}
	carried, err = packages.WheelSources(wheel)
	if err != nil {
		t.Fatal(err)
	}
	metadata := string(carried["yoke_proto-0.1.0.dist-info/METADATA"])
	for _, line := range []string{"Name: yoke-proto\n", "Version: 0.1.0\n", "License-Expression: Apache-2.0\n"} {
		if !strings.Contains(metadata, line) {
			t.Errorf("the wheel's metadata does not say %q:\n%s", strings.TrimSpace(line), metadata)
		}
	}
	if !bytes.Equal(carried["yoke_proto-0.1.0.dist-info/licenses/LICENSE"], licence) ||
		!bytes.Equal(carried["yoke_proto-0.1.0.dist-info/licenses/NOTICE"], notice) {
		t.Errorf("the wheel does not carry the repository's LICENSE and NOTICE")
	}
	for name, data := range files(t, filepath.Join(root, "proto", "python", "src")) {
		if !bytes.Equal(carried[name], data) {
			t.Errorf("the wheel does not carry %s as the tree holds it", name)
		}
	}
}

// std: yoke:the-definitions-packages.06
func TestEveryImageThePackagesAreMadeInIsPinned(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(root, "ci", "definitions.sh"))
	if err != nil {
		t.Fatal(err)
	}
	images := packages.Images(string(script))
	if len(images) == 0 {
		t.Fatal("the definitions' script names no image")
	}
	pinned := regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	for _, image := range images {
		if !pinned.MatchString(image) {
			t.Errorf("%s is not named by its digest", image)
		}
	}
}
