package release_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/mod/zip"

	"github.com/yoke-project/yoke/internal/release"
)

// git runs git in dir, as a contributor with no configuration of their own would.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// repository is a checkout whose root module is example.com/yk and whose proto/ is a module of its own,
// with one commit, tagged as given.
func repository(t *testing.T, tags ...string) (root, commit string) {
	t.Helper()
	root = t.TempDir()
	for path, content := range map[string]string{
		"go.mod":         "module example.com/yk\n\ngo 1.26\n",
		"yk.go":          "package yk\n",
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
	return root, git(t, root, "rev-parse", "HEAD")
}

// treeHash is the digest Go's checksum database records for the module in subdir at a revision.
func treeHash(t *testing.T, root, path, version, revision, subdir string) string {
	t.Helper()
	file := filepath.Join(t.TempDir(), "module.zip")
	out, _ := os.Create(file)
	if err := zip.CreateFromVCS(out, module.Version{Path: path, Version: version}, root, revision, subdir); err != nil {
		t.Fatal(err)
	}
	out.Close()
	sum, err := dirhash.HashZip(file, dirhash.Hash1)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

// proxy serves what served says, and records what it was asked.
type proxy struct {
	asked  []string
	served map[string]string
}

func (p *proxy) serve(module, version string) (string, error) {
	p.asked = append(p.asked, module+"@"+version)
	return p.served[module+"@"+version], nil
}

func run(t *testing.T, root string, p *proxy) (int, []map[string]any, string, string) {
	t.Helper()
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Proxy: p.serve, Out: &out, Err: &errs,
		Today: func() time.Time { return time.Date(2026, 9, 26, 23, 30, 0, 0, time.UTC) }})
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var l map[string]any
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatalf("a line is not one JSON object: %q", line)
		}
		lines = append(lines, l)
	}
	return code, lines, out.String(), errs.String()
}

// std: yoke:the-release-verb.01
func TestAProgramsTagPublishesTheModule(t *testing.T) {
	root, commit := repository(t, "v0.1.0")
	h1 := treeHash(t, root, "example.com/yk", "v0.1.0", "v0.1.0", "")
	p := &proxy{served: map[string]string{"example.com/yk@v0.1.0": h1}}
	code, lines, out, errs := run(t, root, p)
	if code != 0 || len(p.asked) != 1 || p.asked[0] != "example.com/yk@v0.1.0" {
		t.Fatalf("exit %d, asked %v: %s", code, p.asked, errs)
	}
	want := `{"line":"publication","published":"example.com/yk","version":"v0.1.0","commit":"` + commit +
		`","digests":["` + h1 + `"],"where":"https://proxy.golang.org","authenticated":"https://sum.golang.org",` +
		`"licence":"Apache-2.0","notices":[],"day":"2026-09-26"}` + "\n"
	if out != want || len(lines) != 1 {
		t.Errorf("the verb emitted\n%s\nwant\n%s", out, want)
	}
}

// std: yoke:the-release-verb.02
func TestADefinitionsTagPublishesTheDefinitionsModule(t *testing.T) {
	root, _ := repository(t, "v0.1.0", "proto/v0.2.0")
	programs, definitions := treeHash(t, root, "example.com/yk", "v0.1.0", "v0.1.0", ""), treeHash(t, root, "example.com/yk/proto", "v0.2.0", "proto/v0.2.0", "proto")
	p := &proxy{served: map[string]string{"example.com/yk@v0.1.0": programs, "example.com/yk/proto@v0.2.0": definitions}}
	code, lines, _, errs := run(t, root, p)
	if code != 0 || len(lines) != 2 {
		t.Fatalf("exit %d, %d lines: %s", code, len(lines), errs)
	}
	got := map[string]string{}
	for _, l := range lines {
		digests, _ := l["digests"].([]any)
		if len(digests) != 1 {
			t.Fatalf("a line carries %v", l["digests"])
		}
		got[l["published"].(string)+"@"+l["version"].(string)] = digests[0].(string)
	}
	if got["example.com/yk@v0.1.0"] != programs || got["example.com/yk/proto@v0.2.0"] != definitions || programs == definitions {
		t.Errorf("the lines named %v", got)
	}
}

// std: yoke:the-release-verb.03
func TestAProxyServingAnotherTreeIsRefused(t *testing.T) {
	root, _ := repository(t, "v0.1.0")
	p := &proxy{served: map[string]string{"example.com/yk@v0.1.0": "h1:" + strings.Repeat("A", 43) + "="}}
	code, _, out, errs := run(t, root, p)
	if code == 0 || out != "" || !strings.Contains(errs, "differs") {
		t.Errorf("exit %d, emitted %q, said %q", code, out, errs)
	}
}

// std: yoke:the-release-verb.04
func TestACommitNoReleaseTagNamesPublishesNothing(t *testing.T) {
	for _, tags := range [][]string{nil, {"nightly"}} {
		root, _ := repository(t, tags...)
		p := &proxy{}
		code, _, out, errs := run(t, root, p)
		if code != 0 || out != "" || len(p.asked) != 0 || strings.Count(strings.TrimSpace(errs), "\n") != 0 || !strings.Contains(errs, "nothing") {
			t.Errorf("with tags %v: exit %d, asked %v, emitted %q, said %q", tags, code, p.asked, out, errs)
		}
	}
}
