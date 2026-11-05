package compose

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// templateFiles is a family's template as its archive holds it, under template/.
func templateFiles(build string) map[string]string {
	return map[string]string{
		"template/yoke-plugin.yaml": "plugin: 1\nid: @YOKE_ID@\nlanguage: go\nprogram: @YOKE_PROGRAM@\nmanifest: plugin/@YOKE_ID@.yaml\n" +
			"build:\n  in: plugin/\n  run: " + build + "\narchitectures: [" + HostArch() + "]\n",
		"template/plugin/@YOKE_ID@.yaml": "manifest: 1\nid: @YOKE_ID@\nprotocol: 1\n",
		"template/plugin/main.go":        "package main // @YOKE_ID@ runs as @YOKE_PROGRAM@; @YOKE_OTHER@ is no value\n",
		"template/try/README.md":         "# Trying @YOKE_PROGRAM@\n",
	}
}

// templateBuild writes a fixed executable named for the program.
const templateBuild = `printf '#!/bin/sh\necho hello\n' > "$YOKE_OUT/@YOKE_PROGRAM@" && chmod +x "$YOKE_OUT/@YOKE_PROGRAM@"`

// templateArchive is the gzip-compressed tar of the files given, by name.
func templateArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	tw.Close()
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	zw.Write(raw.Bytes())
	zw.Close()
	return out.Bytes()
}

// carried writes an archive to a file, as an author who obtained it by hand holds it.
func carried(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "yoke-plugin-template-go-0.9.0.tar.gz")
	os.WriteFile(path, templateArchive(t, files), 0o644)
	return path
}

// forge serves a manifest and the template's release; hits counts every request.
type forge struct {
	srv  *httptest.Server
	hits atomic.Int32
	body []byte
	// manifest is the manifest's lines, made once the server's address is known.
	manifest func(base string) string
}

func newForge(t *testing.T, body []byte, manifest func(base, digest string) string) *forge {
	t.Helper()
	f := &forge{body: body}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(body))
	f.manifest = func(base string) string { return manifest(base, digest) }
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		switch r.URL.Path {
		case "/releases/manifest.jsonl":
			fmt.Fprint(w, f.manifest(f.srv.URL))
		case "/yoke-project/yoke-sdk-go/releases/download/v0.9.0/yoke-plugin-template-go-0.9.0.tar.gz":
			w.Write(f.body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	old := manifestURL
	manifestURL = f.srv.URL + "/releases/manifest.jsonl"
	t.Cleanup(func() { manifestURL = old })
	return f
}

func line(v any) string {
	b, _ := json.Marshal(v)
	return string(b) + "\n"
}

// released is a manifest whose cut for v0.12.0 names Go's template at v0.9.0, stated with the digest given.
func released(base, digest string) string { return releasedAs("v0.12.0")(base, digest) }

// releasedAs is released with the cut's version the one given.
func releasedAs(cut string) func(base, digest string) string {
	return func(base, digest string) string { return releasedLines(base, digest, cut) }
}

func releasedLines(base, digest, cut string) string {
	return line(map[string]any{"line": "publication", "published": "yoke-plugin-template-go", "version": "v0.9.0",
		"commit": "c0ffee", "digests": []string{digest}, "where": base + "/yoke-project/yoke-sdk-go/releases/tag/v0.9.0"}) +
		line(map[string]any{"line": "publication", "published": "yoke-source", "version": "v0.12.0", "digests": []string{"sha256:00"}}) +
		line(map[string]any{"line": "cut", "version": cut, "day": "2026-11-20", "publications": []map[string]string{
			{"published": "yoke-source", "version": "v0.12.0"}, {"published": "yoke-plugin-template-go", "version": "v0.9.0"}}})
}

// asComposer has the composer state the version given, for the test.
func asComposer(t *testing.T, v string) {
	t.Helper()
	old := composerVersion
	composerVersion = func() string { return v }
	t.Cleanup(func() { composerVersion = old })
}

// within runs the test from an empty directory of its own.
func within(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	return dir
}

func newPlugin(t *testing.T, args ...string) (int, string) {
	t.Helper()
	return run(append([]string{"new", "plugin"}, args...)...)
}

// madeFromTemplate checks the tree .01 expects in dest, for the program given and the Core range.
func madeFromTemplate(t *testing.T, dest, program, core string) {
	t.Helper()
	read := func(rel string) string {
		data, err := os.ReadFile(filepath.Join(dest, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		return string(data)
	}
	if got := read("plugin/com.example.greeter.yaml"); got != "manifest: 1\nid: com.example.greeter\nprotocol: 1\n" {
		t.Errorf("the Manifest is %q", got)
	}
	if got, want := read("plugin/main.go"), "package main // com.example.greeter runs as "+program+"; @YOKE_OTHER@ is no value\n"; got != want {
		t.Errorf("the source is %q, want %q", got, want)
	}
	if got := read(PluginFile); !strings.Contains(got, "id: com.example.greeter\n") || !strings.Contains(got, "program: "+program+"\n") {
		t.Errorf("the Plugin's file is %q", got)
	}
	descriptor := read("try/" + Descriptor)
	for _, want := range []string{"id: com.example.greeter-try\n", "arch: " + HostArch() + "\n", fmt.Sprintf("core: %q\n", core), "plugin: com.example.greeter"} {
		if !strings.Contains(descriptor, want) {
			t.Errorf("try/%s lacks %q:\n%s", Descriptor, want, descriptor)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "try", Components, "plugins", program)); err != nil {
		t.Errorf("the executable is not placed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "try", Manifests, "com.example.greeter.yaml")); err != nil {
		t.Errorf("the Manifest is not placed: %v", err)
	}
	if got := taken(t, filepath.Join(dest, "try")); len(got) != 1 || got[0]["source"] != ".." || got[0]["unit"] != program {
		t.Errorf("plugins.yaml holds %v", got)
	}
}

func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("%s is left", path)
	}
}

// std: yoke:a-new-plugin.01
func TestANewPluginFromItsRelease(t *testing.T) {
	newForge(t, templateArchive(t, templateFiles(templateBuild)), released)
	asComposer(t, "v0.12.0")
	dir := within(t)
	if code, out := newPlugin(t, "com.example.greeter", "--language", "go"); code != Done {
		t.Fatalf("new plugin: %d\n%s", code, out)
	}
	madeFromTemplate(t, filepath.Join(dir, "greeter"), "greeter", ">=0.12.0")
}

// std: yoke:a-new-plugin.02
func TestANewPluginNamed(t *testing.T) {
	newForge(t, templateArchive(t, templateFiles(templateBuild)), released)
	asComposer(t, "v0.12.0")
	dir := within(t)
	if code, out := newPlugin(t, "com.example.greeter", "--language", "go", "--dir", "hello-plugin", "--program", "hello"); code != Done {
		t.Fatalf("new plugin: %d\n%s", code, out)
	}
	madeFromTemplate(t, filepath.Join(dir, "hello-plugin"), "hello", ">=0.12.0")
	absent(t, filepath.Join(dir, "greeter"))
}

// std: yoke:a-new-plugin.03
func TestANewPluginFromNoRelease(t *testing.T) {
	f := newForge(t, templateArchive(t, templateFiles(templateBuild)), released)
	dir := within(t)
	for _, v := range []string{"v0.12.1-0.20261104100000-0123456789ab", "v0.12.0+dirty", "(devel)"} {
		asComposer(t, v)
		code, out := newPlugin(t, "com.example.greeter", "--language", "go")
		if code != Refused || !strings.Contains(out, "--release") || !strings.Contains(out, "--template") {
			t.Errorf("a composer stating %s: %d\n%s", v, code, out)
		}
		absent(t, filepath.Join(dir, "greeter"))
	}
	if n := f.hits.Load(); n != 0 {
		t.Errorf("a composer that is no release read the network %d times", n)
	}
	asComposer(t, "v0.12.1-0.20261104100000-0123456789ab")
	if code, out := newPlugin(t, "com.example.greeter", "--language", "go", "--release", "0.12.0"); code != Done {
		t.Fatalf("new plugin --release: %d\n%s", code, out)
	}
	madeFromTemplate(t, filepath.Join(dir, "greeter"), "greeter", ">=0.12.0")
}

// std: yoke:a-new-plugin.04
func TestANewPluginFromACarriedArchive(t *testing.T) {
	body := templateArchive(t, templateFiles(templateBuild))
	f := newForge(t, body, released)
	path := filepath.Join(t.TempDir(), "yoke-plugin-template-go-0.9.0.tar.gz")
	os.WriteFile(path, body, 0o644)
	asComposer(t, "(devel)")
	dir := within(t)
	code, out := newPlugin(t, "com.example.greeter", "--language", "go", "--template", path)
	if code != Done {
		t.Fatalf("new plugin --template: %d\n%s", code, out)
	}
	if want := fmt.Sprintf("sha256:%x", sha256.Sum256(body)); !strings.Contains(out, want) || !strings.Contains(out, "checked against no record") {
		t.Errorf("the output does not state the digest %s, checked against no record:\n%s", want, out)
	}
	if n := f.hits.Load(); n != 0 {
		t.Errorf("a carried archive read the network %d times", n)
	}
	madeFromTemplate(t, filepath.Join(dir, "greeter"), "greeter", ">=0.5")
}

// std: yoke:a-new-plugin.05
func TestANewPluginTheRecordRefuses(t *testing.T) {
	body := templateArchive(t, templateFiles(templateBuild))
	cases := []struct {
		name, language, says string
		manifest             func(base, digest string) string
	}{
		{"no cut for the version", "go", "v0.12.0", releasedAs("v0.11.0")},
		{"no template for the family", "rust", "rust", released},
		{"another digest", "go", "digest", func(base, _ string) string { return released(base, "sha256:"+strings.Repeat("0", 64)) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			newForge(t, body, c.manifest)
			asComposer(t, "v0.12.0")
			dir := within(t)
			code, out := newPlugin(t, "com.example.greeter", "--language", c.language)
			if code != Refused || !strings.Contains(out, c.says) {
				t.Errorf("%d, not naming %q:\n%s", code, c.says, out)
			}
			absent(t, filepath.Join(dir, "greeter"))
		})
	}
}

// std: yoke:a-new-plugin.06
func TestANewPluginItsArguments(t *testing.T) {
	f := newForge(t, templateArchive(t, templateFiles(templateBuild)), released)
	asComposer(t, "v0.12.0")
	dir := within(t)
	cases := []struct {
		args []string
		says string
	}{
		{[]string{"Com.Example", "--language", "go"}, "Com.Example"},
		{[]string{"com.example.greeter", "--language", "java"}, "java"},
		{[]string{"com.example.greeter", "--language", "python"}, "later"},
		{[]string{"com.example.greeter", "--language", "go", "--release", "0.12.0", "--template", "t.tar.gz"}, "--release"},
		{[]string{"com.example.greeter", "--language", "go", "--program", "a/b"}, "a/b"},
		{[]string{"com.example.greeter"}, "--language"},
	}
	for _, c := range cases {
		code, out := newPlugin(t, c.args...)
		if code != Refused || !strings.Contains(out, c.says) {
			t.Errorf("%v: %d, not naming %q:\n%s", c.args, code, c.says, out)
		}
		absent(t, filepath.Join(dir, "greeter"))
	}
	os.Mkdir(filepath.Join(dir, "greeter"), 0o755)
	os.WriteFile(filepath.Join(dir, "greeter", "kept"), []byte("mine"), 0o644)
	if code, out := newPlugin(t, "com.example.greeter", "--language", "go"); code != Refused || !strings.Contains(out, "greeter") {
		t.Errorf("a destination there: %d\n%s", code, out)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "greeter", "kept")); string(data) != "mine" {
		t.Errorf("the destination there was changed")
	}
	if n := f.hits.Load(); n != 0 {
		t.Errorf("arguments refused read the network %d times", n)
	}
}

// std: yoke:a-new-plugin.07
func TestANewPluginTheArchiveRefuses(t *testing.T) {
	with := func(change func(map[string]string)) map[string]string {
		files := templateFiles(templateBuild)
		change(files)
		return files
	}
	cases := []struct {
		name, says string
		files      map[string]string
	}{
		{"no template/", "template/", map[string]string{"other/yoke-plugin.yaml": "plugin: 1\n"}},
		{"an entry leaving", "../escaped", with(func(f map[string]string) { f["template/../escaped"] = "out" })},
		{"a descriptor in try/", "try/" + Descriptor, with(func(f map[string]string) { f["template/try/"+Descriptor] = "model: 1\n" })},
		{"a file the validator refuses", "architectures", with(func(f map[string]string) {
			f["template/yoke-plugin.yaml"] = strings.Replace(f["template/yoke-plugin.yaml"], "architectures: ["+HostArch()+"]", "architectures: [sparc]", 1)
		})},
	}
	asComposer(t, "v0.12.0")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := carried(t, c.files)
			dir := within(t)
			code, out := newPlugin(t, "com.example.greeter", "--language", "go", "--template", path)
			if code != Refused || !strings.Contains(out, c.says) {
				t.Errorf("%d, not naming %q:\n%s", code, c.says, out)
			}
			absent(t, filepath.Join(dir, "greeter"))
			absent(t, filepath.Join(dir, "escaped"))
		})
	}
}

// std: yoke:a-new-plugin.08
func TestANewPluginAFailedTake(t *testing.T) {
	path := carried(t, templateFiles("exit 3"))
	asComposer(t, "v0.12.0")
	dir := within(t)
	code, out := newPlugin(t, "com.example.greeter", "--language", "go", "--template", path)
	if code != Refused || !strings.Contains(out, "add plugin ..") || !strings.Contains(out, filepath.Join("greeter", "try")) {
		t.Errorf("%d, not naming the command that repeats the take:\n%s", code, out)
	}
	dest := filepath.Join(dir, "greeter")
	for _, rel := range []string{PluginFile, "plugin/com.example.greeter.yaml", "try/" + Descriptor} {
		if _, err := os.Stat(filepath.Join(dest, rel)); err != nil {
			t.Errorf("%s is not left: %v", rel, err)
		}
	}
	absent(t, filepath.Join(dest, "try", Plugins))
	if data, _ := os.ReadFile(filepath.Join(dest, "try", Descriptor)); strings.Contains(string(data), "units") {
		t.Errorf("a unit is declared:\n%s", data)
	}
}
