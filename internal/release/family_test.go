package release_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/packages"
	"github.com/yoke-project/yoke/internal/release"
)

// A family's packaging script that writes the crate yoke-sdk at the version it is asked for, and records it.
const packageScript = `#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == package ]] || exit 2
echo "$2" >> "$(dirname "$0")/../asked"
stage="$(mktemp -d)/yoke-sdk-$2"
mkdir -p "$stage/src"
printf 'pub mod plugin {}\n' > "$stage/src/lib.rs"
printf '[package]\nname = "yoke-sdk"\nversion = "%s"\n' "$2" > "$stage/Cargo.toml.orig"
tar -czf "$3/yoke-sdk-$2.crate" -C "$(dirname "$stage")" "yoke-sdk-$2"
`

// A family's packaging script refusing, as one does when the version it states is not the tag's.
const refusingScript = `#!/usr/bin/env bash
echo "package: the tree states 0.2.0 and the tag 0.3.0" >&2
exit 1
`

// family is a family's checkout, with no Go module and the packaging script given, tagged as given.
func family(t *testing.T, script string, tags ...string) (root, commit string) {
	t.Helper()
	root = t.TempDir()
	for path, content := range map[string]string{
		"Cargo.toml": "[package]\nname = \"yoke-sdk\"\n", "LICENSE": "Apache License\n", "NOTICE": "Yk\n",
		"ci/package.sh": script, ".gitignore": "asked\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(content), 0o755)
	}
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "one")
	for _, tag := range tags {
		git(t, root, "tag", tag)
	}
	return root, git(t, root, "rev-parse", "HEAD")
}

// publishingFamily runs the verb as a family publishing the crate yoke-sdk to the registry given.
func publishingFamily(t *testing.T, root string, crates *registry) (int, []map[string]any, string, string, *proxy) {
	t.Helper()
	p := &proxy{served: map[string]string{}}
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Proxy: p.serve, Out: &out, Err: &errs, Family: true,
		Packages: release.Scripted(root, packages.Rust, "yoke-sdk"), Registries: []release.Registry{crates}, Settle: 1,
		Upload: func(string, []string) error { t.Error("a file was handed over"); return nil },
		Today:  func() time.Time { return time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC) }})
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
	return code, lines, out.String(), errs.String(), p
}

// std: yoke:the-family-packages.01
func TestAFamilysReleaseTagPublishesItsPackage(t *testing.T) {
	root, commit := family(t, packageScript, "v0.3.0")
	crates := &registry{kind: packages.Rust, name: "crates.io/yoke-sdk", host: "crates.io", held: map[string][]byte{}}
	code, lines, _, errs, p := publishingFamily(t, root, crates)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if len(p.asked) != 0 {
		t.Errorf("the proxy was asked %v", p.asked)
	}
	if asked, _ := os.ReadFile(filepath.Join(root, "asked")); string(asked) != "0.3.0\n" {
		t.Errorf("the script was asked to package %q", asked)
	}
	if len(crates.published) != 1 || len(lines) != 1 {
		t.Fatalf("published %v, emitted %v", crates.published, lines)
	}
	l := lines[0]
	want := map[string]any{"line": "publication", "published": "crates.io/yoke-sdk", "version": "v0.3.0", "commit": commit,
		"licence": "Apache-2.0", "day": "2026-10-04"}
	for field, value := range want {
		if l[field] != value {
			t.Errorf("the line says %s %v, want %v", field, l[field], value)
		}
	}
	if d, _ := l["digests"].([]any); len(d) != 1 || d[0] != digestOf(crates.held["0.3.0"]) {
		t.Errorf("the line carries the digests %v, want the served file's", l["digests"])
	}
	if n, _ := l["notices"].([]any); len(n) != 1 || n[0] != "NOTICE" {
		t.Errorf("the line names the notices %v", l["notices"])
	}
}

// std: yoke:the-family-packages.02
func TestAScriptThatWritesNoPackageFailsTheVerb(t *testing.T) {
	root, _ := family(t, refusingScript, "v0.3.0")
	crates := &registry{kind: packages.Rust, name: "crates.io/yoke-sdk", host: "crates.io", held: map[string][]byte{}}
	code, _, out, errs, _ := publishingFamily(t, root, crates)
	if code == 0 || out != "" || !strings.Contains(errs, "the tree states 0.2.0 and the tag 0.3.0") {
		t.Errorf("exit %d, emitted %q, said %q", code, out, errs)
	}
	if crates.asked != 0 {
		t.Errorf("the registry was asked %d times", crates.asked)
	}
}

// familyWheel is the wheel yoke-sdk at version.
func familyWheel(version string) []byte {
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	for name, content := range map[string]string{
		"yoke_sdk-" + version + ".dist-info/METADATA": "Metadata-Version: 2.4\nName: yoke-sdk\nVersion: " + version + "\n",
		"yoke_sdk/plugin.py":                          "# the library\n",
	} {
		w, _ := archive.Create(name)
		w.Write([]byte(content))
	}
	archive.Close()
	return buf.Bytes()
}

// std: yoke:the-family-packages.03
func TestEachRegistryAsksForPublishesAndNamesThePackageItIsGiven(t *testing.T) {
	crate, wheel := []byte("the crate"), familyWheel("0.3.0")
	var asked []string
	var uploaded string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/crates/yoke-sdk/0.3.0":
			fmt.Fprint(w, `{"version":{"num":"0.3.0"}}`)
		case "/crates/yoke-sdk/yoke-sdk-0.3.0.crate":
			w.Write(crate)
		case "/pypi/yoke-sdk/0.3.0/json":
			fmt.Fprintf(w, `{"urls":[{"filename":"yoke_sdk-0.3.0-py3-none-any.whl","url":"%s/files/yoke_sdk.whl"}]}`, server.URL)
		case "/files/yoke_sdk.whl":
			w.Write(wheel)
		case "/legacy/":
			_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			form := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, err := form.NextPart()
				if err != nil {
					break
				}
				value, _ := io.ReadAll(part)
				if part.FormName() == "name" {
					uploaded = string(value)
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	crates := release.Crates{API: server.URL, Static: server.URL, Name: "yoke-sdk"}
	if served, found, err := crates.Served("0.3.0"); err != nil || !found || !bytes.Equal(served, crate) {
		t.Errorf("the crate registry served %q %v %v, having been asked %v", served, found, err, asked)
	}
	pypi := release.PyPI{Index: server.URL, Upload: server.URL + "/legacy/", Name: "yoke-sdk",
		Getenv: func(name string) string { return map[string]string{"PYPI_TOKEN": "pypi-given"}[name] }}
	if served, found, err := pypi.Served("0.3.0"); err != nil || !found || !bytes.Equal(served, wheel) {
		t.Errorf("the index served %d bytes %v %v, having been asked %v", len(served), found, err, asked)
	}
	path := filepath.Join(t.TempDir(), "yoke_sdk-0.3.0-py3-none-any.whl")
	os.WriteFile(path, wheel, 0o644)
	if err := pypi.Publish("0.3.0", path); err != nil || uploaded != "yoke-sdk" {
		t.Errorf("the upload named %q: %v", uploaded, err)
	}
	for _, c := range []struct{ got, want string }{
		{crates.Published(), "crates.io/yoke-sdk"},
		{pypi.Published(), "pypi.org/yoke-sdk"},
		{crates.Where("0.3.0"), server.URL + "/crates/yoke-sdk/0.3.0"},
		{pypi.Where("0.3.0"), server.URL + "/project/yoke-sdk/0.3.0/"},
		{release.Crates{}.Published(), "crates.io/yoke-proto"},
		{release.PyPI{}.Published(), "pypi.org/yoke-proto"},
	} {
		if c.got != c.want {
			t.Errorf("named %q, want %q", c.got, c.want)
		}
	}
}
