package release_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/release"
)

// withCommands is a repository with two commands, tagged as given, and a file left uncommitted beside
// the tagged tree.
func withCommands(t *testing.T, tags ...string) string {
	t.Helper()
	root := t.TempDir()
	for path, content := range map[string]string{
		"go.mod":         "module example.com/yk\n\ngo 1.26\n",
		"cmd/a/main.go":  "package main\n\nfunc main() { println(\"a\") }\n",
		"cmd/b/main.go":  "package main\n\nfunc main() { println(\"b\") }\n",
		"LICENSE":        "Apache License\n",
		"proto/go.mod":   "module example.com/yk/proto\n\ngo 1.26\n",
		"proto/proto.go": "package proto\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(content), 0o644)
	}
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "one")
	for _, tag := range tags {
		git(t, root, "tag", tag)
	}
	os.WriteFile(filepath.Join(root, "scratch.txt"), []byte("never committed\n"), 0o644)
	return root
}

// shelf keeps what was uploaded, by file name, and the tag it went to.
type shelf struct {
	tag   string
	files map[string][]byte
	fail  bool
}

func (s *shelf) upload(tag string, files []string) error {
	if s.fail {
		return errors.New("the forge refused the upload")
	}
	s.tag, s.files = tag, map[string][]byte{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		s.files[filepath.Base(f)] = data
	}
	return nil
}

func handOver(t *testing.T, root string, s *shelf) (int, []map[string]any, string) {
	t.Helper()
	// The proxy serves each module's tree as it is tagged.
	serve := func(m, v string) (string, error) {
		if m == "example.com/yk/proto" {
			return treeHash(t, root, m, v, "proto/"+v, "proto"), nil
		}
		return treeHash(t, root, m, v, v, ""), nil
	}
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Out: &out, Err: &errs,
		Proxy:     serve,
		Today:     func() time.Time { return time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC) },
		Artifacts: []release.Artifact{{Name: "yk-tools", Packages: []string{"./cmd/a", "./cmd/b"}}},
		Source:    "yk", Releases: "https://forge.example/yk/releases/tag/", Upload: s.upload})
	_, lines, _, _ := parse(t, out.String())
	return code, lines, out.String() + errs.String()
}

// parse reads a run's standard output as lines of JSON.
func parse(t *testing.T, out string) (int, []map[string]any, string, string) {
	t.Helper()
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		var l map[string]any
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatalf("a line is not one JSON object: %q", line)
		}
		lines = append(lines, l)
	}
	return 0, lines, out, ""
}

// entries lists a gzipped tar's entries and their contents.
func entries(t *testing.T, archive []byte) map[string][]byte {
	t.Helper()
	z, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]byte{}
	r := tar.NewReader(z)
	for {
		h, err := r.Next()
		if err == io.EOF {
			return found
		}
		if err != nil {
			t.Fatal(err)
		}
		// A global header is the archive's own metadata — git writes the commit there — and no file.
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		data, _ := io.ReadAll(r)
		found[h.Name] = data
	}
}

// std: yoke:the-artifacts.01
func TestAProgramsTagBuildsEachArtifactForBothArchitectures(t *testing.T) {
	root := withCommands(t, "v0.1.0")
	s := &shelf{}
	if code, _, said := handOver(t, root, s); code != 0 {
		t.Fatalf("the verb failed: %s", said)
	}
	if s.tag != "v0.1.0" {
		t.Errorf("the files went to %q", s.tag)
	}
	for arch, machine := range map[string]elf.Machine{"amd64": elf.EM_X86_64, "arm64": elf.EM_AARCH64} {
		archive, found := s.files["yk-tools-0.1.0-linux-"+arch+".tar.gz"]
		if !found {
			t.Fatalf("no archive for %s among %v", arch, keys(s.files))
		}
		inside := entries(t, archive)
		if _, licensed := inside["LICENSE"]; !licensed || len(inside) != 3 {
			t.Errorf("the %s archive holds %v", arch, keys(inside))
		}
		for _, program := range []string{"a", "b"} {
			f, err := elf.NewFile(bytes.NewReader(inside[program]))
			if err != nil {
				t.Fatalf("%s for %s is not an ELF executable: %v", program, arch, err)
			}
			if f.Machine != machine {
				t.Errorf("%s for %s is built for %v", program, arch, f.Machine)
			}
			for _, p := range f.Progs {
				if p.Type == elf.PT_INTERP {
					t.Errorf("%s for %s names an interpreter: it is not statically linked", program, arch)
				}
			}
		}
	}
}

// std: yoke:the-artifacts.02
func TestTheSourceArchiveIsBuiltFromTheTag(t *testing.T) {
	root := withCommands(t, "v0.1.0")
	s := &shelf{}
	if code, _, said := handOver(t, root, s); code != 0 {
		t.Fatalf("the verb failed: %s", said)
	}
	archive, found := s.files["yk-0.1.0-source.tar.gz"]
	if !found {
		t.Fatalf("no source archive among %v", keys(s.files))
	}
	inside := entries(t, archive)
	for _, want := range []string{"yk-0.1.0/go.mod", "yk-0.1.0/cmd/a/main.go", "yk-0.1.0/cmd/b/main.go", "yk-0.1.0/LICENSE", "yk-0.1.0/proto/go.mod"} {
		if _, there := inside[want]; !there {
			t.Errorf("the source archive lacks %s: %v", want, keys(inside))
		}
	}
	for name := range inside {
		if strings.Contains(name, "scratch") || !strings.HasPrefix(name, "yk-0.1.0/") {
			t.Errorf("the source archive holds %s", name)
		}
	}
}

// std: yoke:the-artifacts.03
func TestEachFileIsNamedByOneLineWithItsDigest(t *testing.T) {
	root := withCommands(t, "v0.1.0")
	s := &shelf{}
	code, lines, said := handOver(t, root, s)
	if code != 0 || len(lines) != 1+len(s.files) || len(s.files) != 3 {
		t.Fatalf("exit %d, %d lines for %d files: %s", code, len(lines), len(s.files), said)
	}
	named := map[string]bool{}
	for _, l := range lines[1:] {
		published, _ := l["published"].(string)
		file := strings.Replace(published, "-linux-", "-0.1.0-linux-", 1) + ".tar.gz"
		if published == "yk-source" {
			file = "yk-0.1.0-source.tar.gz"
		}
		data, uploaded := s.files[file]
		sum := sha256.Sum256(data)
		digests, _ := l["digests"].([]any)
		if !uploaded || len(digests) != 1 || digests[0] != "sha256:"+hex.EncodeToString(sum[:]) || l["version"] != "v0.1.0" ||
			l["where"] != "https://forge.example/yk/releases/tag/v0.1.0" || l["authenticated"] != "https://forge.example" {
			t.Errorf("the line %v does not name %s as uploaded", l, file)
		}
		named[file] = true
	}
	if len(named) != len(s.files) {
		t.Errorf("the lines named %v, the files were %v", keys(named), keys(s.files))
	}
}

// std: yoke:the-artifacts.04
func TestBuiltTwiceTheFilesAreTheSameBytes(t *testing.T) {
	root := withCommands(t, "v0.1.0")
	first, second := &shelf{}, &shelf{}
	handOver(t, root, first)
	handOver(t, root, second)
	if len(first.files) != 3 {
		t.Fatalf("the first run uploaded %v", keys(first.files))
	}
	for name, data := range first.files {
		if !bytes.Equal(data, second.files[name]) {
			t.Errorf("%s differs between two runs", name)
		}
	}
}

// std: yoke:the-artifacts.05
func TestADefinitionsTagHandsOverNoFileAndAFailedUploadWritesNoLine(t *testing.T) {
	root := withCommands(t, "proto/v0.2.0")
	s := &shelf{}
	code, lines, said := handOver(t, root, s)
	if code != 0 || s.files != nil || len(lines) != 1 || lines[0]["published"] != "example.com/yk/proto" {
		t.Errorf("a definitions tag: exit %d, uploaded %v, lines %v: %s", code, keys(s.files), lines, said)
	}
	root = withCommands(t, "v0.1.0")
	failing := &shelf{fail: true}
	code, lines, said = handOver(t, root, failing)
	if code == 0 || len(lines) != 0 {
		t.Errorf("a failed upload: exit %d, lines %v: %s", code, lines, said)
	}
}

func keys[V any](m map[string]V) []string {
	var k []string
	for name := range m {
		k = append(k, name)
	}
	sort.Strings(k)
	return k
}
