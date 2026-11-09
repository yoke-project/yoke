package compose

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// taggedGreeter is a repository reached by file:// holding the greeter, tagged v1.0.0 and v1.1.0 on two
// commits each building a different program, its main ahead of both; and the program each tag builds.
func taggedGreeter(t *testing.T) (url string, builds map[string]string) {
	t.Helper()
	dir := greeterSource(t)
	builds = map[string]string{}
	release := func(tag, says string) {
		run := `printf '#!/bin/sh\necho ` + says + `\n' > "$YOKE_OUT/greeter" && chmod +x "$YOKE_OUT/greeter"`
		file := strings.Replace(withRun(run), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1)
		os.WriteFile(filepath.Join(dir, PluginFile), []byte(file), 0o644)
		git(t, dir, "commit", "-q", "-am", says)
		if tag != "" {
			git(t, dir, "tag", tag)
			builds[tag] = "#!/bin/sh\necho " + says + "\n"
		}
	}
	release("v1.0.0", "one")
	release("v1.1.0", "one-one")
	release("", "main")
	git(t, dir, "tag", "latest", "v1.0.0")
	git(t, dir, "tag", "1.0", "v1.0.0")
	return "file://" + dir, builds
}

func commitOf(t *testing.T, url, ref string) string {
	t.Helper()
	return git(t, strings.TrimPrefix(url, "file://"), "rev-parse", ref+"^{commit}")
}

// std: yoke:a-plugin-at-a-tag.01
func TestARepositoryAtATagIsTakenAtTheTagsCommitAndBothAreRecorded(t *testing.T) {
	app := authored(t)
	url, builds := taggedGreeter(t)
	if code, out := takePlugin(t, "", url+"@v1.0.0", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin %s@v1.0.0: %d %s", url, code, out)
	}
	if raw(t, filepath.Join(app, Components, "plugins", "greeter")) != builds["v1.0.0"] {
		t.Errorf("the executable is not the one v1.0.0's source builds: %q", raw(t, filepath.Join(app, Components, "plugins", "greeter")))
	}
	e := taken(t, app)
	if len(e) != 1 || e[0]["source"] != url || e[0]["tag"] != "v1.0.0" || e[0]["commit"] != commitOf(t, url, "v1.0.0") {
		t.Errorf("plugins.yaml holds %v", e)
	}
}

// std: yoke:a-plugin-at-a-tag.02
func TestATagThatIsNotAVersionOrThatTheRepositoryDoesNotHaveIsRefused(t *testing.T) {
	app := authored(t)
	url, _ := taggedGreeter(t)
	before := snapshot(t, app)
	for _, c := range []struct{ source, says string }{
		{url + "@latest", "SemVer"},
		{url + "@1.0", "SemVer"},
		{url + "@v9.0.0", "v9.0.0"},
		{url, "tag"},
		{"file://" + filepath.Join(t.TempDir(), "nothing") + "@v1.0.0", "cannot be obtained"},
	} {
		code, out := takePlugin(t, "", c.source, "--yes", "--dir", app)
		if code == Done || !strings.Contains(out, c.says) {
			t.Errorf("%s: %d %s", c.source, code, out)
		}
		sameAs(t, app, before)
	}
}

// std: yoke:a-plugin-at-a-tag.03
func TestUpdatePluginTakesAnotherTagAndReplacesWhatTheFirstOnePlaced(t *testing.T) {
	app := authored(t)
	url, builds := taggedGreeter(t)
	if code, out := takePlugin(t, "", url+"@v1.0.0", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	if _, _, err := Build(app, BuildOptions{Out: t.TempDir()}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if bundled(t, app, "components/plugins/greeter") != builds["v1.0.0"] {
		t.Errorf("the first package does not carry v1.0.0's executable")
	}
	file := filepath.Join(app, Descriptor)
	before := written(t, file, "units.greeter")

	later := time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)
	code, out := updating(t, later, "com.example.greeter", "--to", "v1.1.0", "--yes", "--dir", app)
	if code != Done {
		t.Fatalf("update plugin: %d %s", code, out)
	}
	if raw(t, filepath.Join(app, Components, "plugins", "greeter")) != builds["v1.1.0"] {
		t.Errorf("the executable is not v1.1.0's")
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(builds["v1.1.0"])))
	after := written(t, file, "units.greeter").(map[string]any)
	if after["digest"] != digest {
		t.Errorf("the pinned digest is %v, not %s", after["digest"], digest)
	}
	for k, v := range before.(map[string]any) {
		if k != "digest" && fmt.Sprint(after[k]) != fmt.Sprint(v) {
			t.Errorf("the unit's %s changed: %v", k, after[k])
		}
	}
	e := taken(t, app)
	if len(e) != 1 || e[0]["unit"] != "greeter" || e[0]["tag"] != "v1.1.0" || e[0]["commit"] != commitOf(t, url, "v1.1.0") || e[0]["digest"] != digest ||
		!strings.Contains(raw(t, filepath.Join(app, Plugins)), "taken: "+later.Format(time.RFC3339)) {
		t.Errorf("plugins.yaml holds %v", e)
	}
	if _, _, err := Build(app, BuildOptions{Out: t.TempDir()}); err != nil {
		t.Fatalf("build: %v", err)
	}
	if bundled(t, app, "components/plugins/greeter") != builds["v1.1.0"] {
		t.Errorf("the second package does not carry v1.1.0's executable")
	}

	unchanged := snapshot(t, app)
	if code, out := updating(t, later, "com.example.greeter", "--to", "latest", "--yes", "--dir", app); code == Done || !strings.Contains(out, "SemVer") {
		t.Errorf("update to latest: %d %s", code, out)
	}
	sameAs(t, app, unchanged)
}

// std: yoke:a-plugin-at-a-tag.04
func TestPluginsListsWhatWasTakenAndSaysWhenATagHasMoved(t *testing.T) {
	app := authored(t)
	url, _ := taggedGreeter(t)
	if code, out := takePlugin(t, "", url+"@v1.0.0", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	loose := pluginProject(t, strings.Replace(strings.Replace(withRun(greeterBuild), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1),
		"com.example.greeter", "com.example.loose", -1))
	os.Remove(filepath.Join(loose, "plugin", "com.example.greeter.yaml"))
	os.WriteFile(filepath.Join(loose, "plugin", "com.example.loose.yaml"), []byte("manifest: 1\nid: com.example.loose\nprotocol: 1\n"), 0o644)
	if code, out := takePlugin(t, "", loose, "--as", "loose", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin %s: %d %s", loose, code, out)
	}
	first := commitOf(t, url, "v1.0.0")

	code, out := run("plugins", "--dir", app)
	if code != Done {
		t.Fatalf("plugins: %d %s", code, out)
	}
	for _, want := range []string{"com.example.greeter", "greeter", url, "v1.0.0", first, "com.example.loose", "loose", loose} {
		if !strings.Contains(out, want) {
			t.Errorf("plugins does not say %q: %s", want, out)
		}
	}
	if strings.Contains(out, "now names") {
		t.Errorf("plugins says a tag moved before one did: %s", out)
	}

	repo := strings.TrimPrefix(url, "file://")
	git(t, repo, "tag", "-f", "v1.0.0", "v1.1.0")
	moved := commitOf(t, url, "v1.0.0")
	before := snapshot(t, app)
	code, out = run("plugins", "--dir", app)
	if code != Done || !strings.Contains(out, "now names") || !strings.Contains(out, first) || !strings.Contains(out, moved) {
		t.Errorf("plugins does not say v1.0.0 moved, naming both commits: %d %s", code, out)
	}
	sameAs(t, app, before)

	code, out = updating(t, takenAt, "com.example.greeter", "--to", "v1.0.0", "--yes", "--dir", app)
	if code != Done {
		t.Fatalf("update plugin --to v1.0.0: %d %s", code, out)
	}
	if !strings.Contains(out, "now names") || strings.Index(out, "now names") > strings.Index(out, "command:") {
		t.Errorf("update does not report the moved tag before it builds: %s", out)
	}
	if e := taken(t, app); e[0]["commit"] != moved {
		t.Errorf("update recorded %v, not the commit the tag names now", e[0]["commit"])
	}
}

// updating runs update plugin with the clock at the time given.
func updating(t *testing.T, at time.Time, args ...string) (int, string) {
	t.Helper()
	clock := pluginClock
	pluginClock = func() time.Time { return at }
	defer func() { pluginClock = clock }()
	return run(append([]string{"update", "plugin"}, args...)...)
}

// bundled is the file at path in the project's bundle, as text.
func bundled(t *testing.T, dir, path string) string {
	t.Helper()
	data, err := archive(dir)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == path {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			b, _ := io.ReadAll(r)
			return string(b)
		}
	}
	t.Fatalf("the bundle holds no %s", path)
	return ""
}

// std: yoke:a-plugin-at-a-tag.05
func TestUpdatePluginWithoutATagRetakesAPathAsItNowStands(t *testing.T) {
	app := authored(t)
	dir := greeterSource(t)
	first := "#!/bin/sh\necho first\n"
	build := func(says string) {
		run := `printf '#!/bin/sh\necho ` + says + `\n' > "$YOKE_OUT/greeter" && chmod +x "$YOKE_OUT/greeter"`
		file := strings.Replace(withRun(run), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1)
		os.WriteFile(filepath.Join(dir, PluginFile), []byte(file), 0o644)
	}
	build("first")
	git(t, dir, "commit", "-q", "-am", "first")
	if code, out := takePlugin(t, "", dir, "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	if got := raw(t, filepath.Join(app, Components, "plugins", "greeter")); got != first {
		t.Fatalf("taken: %q", got)
	}
	build("second")
	later := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	code, out := updating(t, later, "com.example.greeter", "--yes", "--dir", app)
	if code != Done {
		t.Fatalf("update plugin: %d %s", code, out)
	}
	second := "#!/bin/sh\necho second\n"
	if got := raw(t, filepath.Join(app, Components, "plugins", "greeter")); got != second {
		t.Errorf("the executable is %q, not what the path builds now", got)
	}
	entries, err := readPlugins(app)
	if err != nil || len(entries) != 1 {
		t.Fatalf("the record: %v %v", entries, err)
	}
	e := entries[0]
	if e.Tag != "" || e.Source != dir || e.Clean == nil || *e.Clean || e.Digest != fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(second))) {
		t.Errorf("the record is %+v, want the path, no tag, not clean, the new digest", e)
	}
	if code, out := updating(t, later, "com.example.greeter", "--to", "v1.0.0", "--yes", "--dir", app); code == Done || !strings.Contains(out, "path") {
		t.Errorf("--to on a path: %d %s", code, out)
	}
}

// std: yoke:a-plugin-at-a-tag.06
func TestUpdatePluginOfARepositoryStillRequiresATag(t *testing.T) {
	app := authored(t)
	url, _ := taggedGreeter(t)
	if code, out := takePlugin(t, "", url+"@v1.0.0", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	code, out := updating(t, time.Now(), "com.example.greeter", "--yes", "--dir", app)
	if code == Done || !strings.Contains(out, "--to") {
		t.Errorf("no --to on a repository: %d %s", code, out)
	}
}
