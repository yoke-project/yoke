package compose

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

var takenAt = time.Date(2026, 11, 4, 18, 20, 0, 0, time.UTC)

// greeterProgram is what the greeter's build writes, the same at every build.
const greeterProgram = "#!/bin/sh\necho hello\n"

// greeterBuild writes greeterProgram, and a mark beside the project's root when it runs.
const greeterBuild = `touch ../.built && printf '#!/bin/sh\necho hello\n' > "$YOKE_OUT/greeter" && chmod +x "$YOKE_OUT/greeter"`

// greeterSource is a Plugin project for the workstation's architecture whose build writes greeterProgram,
// committed in a git tree of its own.
func greeterSource(t *testing.T) string {
	t.Helper()
	file := strings.Replace(withRun(greeterBuild), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1)
	dir := pluginProject(t, file)
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "the greeter")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// takePlugin runs add plugin with the clock fixed and the answer given on standard input.
func takePlugin(t *testing.T, answer string, args ...string) (int, string) {
	t.Helper()
	clock, input := pluginClock, pluginInput
	pluginClock = func() time.Time { return takenAt }
	pluginInput = strings.NewReader(answer)
	defer func() { pluginClock, pluginInput = clock, input }()
	return run(append([]string{"add", "plugin"}, args...)...)
}

// taken is the project's plugins.yaml, entry by entry, or nil where there is none.
func taken(t *testing.T, dir string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, Plugins))
	if err != nil {
		return nil
	}
	var doc struct {
		Plugins []map[string]any `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s does not parse: %v", Plugins, err)
	}
	return doc.Plugins
}

// snapshot is every file of the application project but its package outputs, by path, as bytes.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			data, _ := os.ReadFile(path)
			files[rel] = string(data)
		}
		return nil
	})
	return files
}

func sameAs(t *testing.T, dir string, before map[string]string) {
	t.Helper()
	after := snapshot(t, dir)
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed", k)
		}
	}
	for k := range after {
		if _, there := before[k]; !there {
			t.Errorf("%s was written", k)
		}
	}
}

// packaged is every path the project's bundle holds.
func packaged(t *testing.T, dir string) map[string]bool {
	t.Helper()
	data, err := archive(dir)
	if err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	held := map[string]bool{}
	for _, f := range z.File {
		held[f.Name] = true
	}
	return held
}

// std: yoke:taking-a-plugin.01
func TestAPluginTakenFromAPathIsBuiltPlacedDeclaredPinnedAndRecorded(t *testing.T) {
	app, source := authored(t), greeterSource(t)
	commit := git(t, source, "rev-parse", "HEAD")
	if code, out := takePlugin(t, "", source, "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	if raw(t, filepath.Join(app, Components, "plugins", "greeter")) != greeterProgram {
		t.Errorf("the executable placed is not what the build wrote")
	}
	if raw(t, filepath.Join(app, Manifests, "com.example.greeter.yaml")) != greeterManifest {
		t.Errorf("the Manifest placed is not the one kept in the source")
	}
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(greeterProgram)))
	file := filepath.Join(app, Descriptor)
	if written(t, file, "units.greeter.kind") != "plugin" || written(t, file, "units.greeter.plugin") != "com.example.greeter" ||
		written(t, file, "units.greeter.exec") != "components/plugins/greeter" || written(t, file, "units.greeter.digest") != digest {
		t.Errorf("the unit is not declared and pinned: %s", raw(t, file))
	}
	if r := Check(app); r.Refused() {
		t.Errorf("check refuses the project: %+v", r.Findings)
	}
	entries := taken(t, app)
	if len(entries) != 1 {
		t.Fatalf("plugins.yaml holds %v", entries)
	}
	want := map[string]any{"id": "com.example.greeter", "unit": "greeter", "source": source, "tag": nil, "commit": commit,
		"clean": true, "arch": HostArch(), "digest": digest}
	for k, v := range want {
		if fmt.Sprint(entries[0][k]) != fmt.Sprint(v) {
			t.Errorf("plugins.yaml's %s is %v, not %v", k, entries[0][k], v)
		}
	}
	if !strings.Contains(raw(t, filepath.Join(app, Plugins)), "taken: "+takenAt.Format(time.RFC3339)) {
		t.Errorf("plugins.yaml does not date the taking at the fixed time: %s", raw(t, filepath.Join(app, Plugins)))
	}
	if _, there := entries[0]["tag"]; !there {
		t.Errorf("plugins.yaml does not say the Plugin has no tag")
	}

	if _, report, err := Build(app, BuildOptions{Out: t.TempDir()}); err != nil {
		t.Fatalf("build: %v %+v", err, report.Findings)
	}
	inside := packaged(t, app)
	if !inside["components/plugins/greeter"] || !inside["manifests/com.example.greeter.yaml"] {
		t.Errorf("the package does not hold the executable and the Manifest: %v", inside)
	}
	if inside[Plugins] {
		t.Errorf("the package holds %s", Plugins)
	}
}

// std: yoke:taking-a-plugin.02
func TestAsNamesTheUnitAndATreeWithUncommittedChangesIsRecordedAsNotClean(t *testing.T) {
	app, source := authored(t), greeterSource(t)
	os.WriteFile(filepath.Join(source, "plugin", "main.go"), []byte("package main\n"), 0o644)
	if code, out := takePlugin(t, "", source, "--as", "hello", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin --as hello: %d %s", code, out)
	}
	if raw(t, filepath.Join(app, Components, "plugins", "hello")) != greeterProgram ||
		written(t, filepath.Join(app, Descriptor), "units.hello.exec") != "components/plugins/hello" {
		t.Errorf("the unit is not placed as hello")
	}
	if e := taken(t, app); len(e) != 1 || e[0]["unit"] != "hello" || e[0]["clean"] != false {
		t.Errorf("plugins.yaml holds %v", e)
	}

	loose := pluginProject(t, strings.Replace(strings.Replace(withRun(greeterBuild), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1),
		"com.example.greeter", "com.example.loose", -1))
	os.Rename(filepath.Join(loose, "plugin", "com.example.greeter.yaml"), filepath.Join(loose, "plugin", "com.example.loose.yaml"))
	os.WriteFile(filepath.Join(loose, "plugin", "com.example.loose.yaml"), []byte("manifest: 1\nid: com.example.loose\nprotocol: 1\n"), 0o644)
	if code, out := takePlugin(t, "", loose, "--as", "loose", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin of a tree that is not git's: %d %s", code, out)
	}
	e := taken(t, app)
	if len(e) != 2 || e[1]["unit"] != "loose" || e[1]["tag"] != nil {
		t.Fatalf("plugins.yaml holds %v", e)
	}
	if _, there := e[1]["commit"]; there {
		t.Errorf("a Plugin outside any git tree is recorded with a commit: %v", e[1])
	}
	if written(t, filepath.Join(app, Descriptor), "units.loose.digest") == nil {
		t.Errorf("the second unit is not pinned")
	}
}

// std: yoke:taking-a-plugin.03
func TestTheComposerShowsWhatItWillRunAndAsksBeforeTheAuthorsBuild(t *testing.T) {
	app, source := authored(t), greeterSource(t)
	commit := git(t, source, "rev-parse", "HEAD")
	mark := filepath.Join(filepath.Dir(source), ".built")
	before := snapshot(t, app)
	for _, answer := range []string{"n\n", ""} {
		code, out := takePlugin(t, answer, source, "--dir", app)
		if code == Done {
			t.Errorf("answering %q took the Plugin", answer)
		}
		for _, shown := range []string{source, commit, greeterBuild} {
			if !strings.Contains(out, shown) {
				t.Errorf("answering %q, %q was not shown: %s", answer, shown, out)
			}
		}
		if !strings.Contains(out, "declined") {
			t.Errorf("answering %q is not refused as declined: %s", answer, out)
		}
		if _, err := os.Stat(mark); err == nil {
			t.Errorf("answering %q, the build ran", answer)
		}
		sameAs(t, app, before)
	}
	code, out := takePlugin(t, "yes\n", source, "--dir", app)
	if code != Done {
		t.Fatalf("answering yes: %d %s", code, out)
	}
	if strings.Index(out, greeterBuild) > strings.Index(out, "took") && strings.Contains(out, "took") {
		t.Errorf("the command was shown after the Plugin was taken: %s", out)
	}
	if len(taken(t, app)) != 1 {
		t.Errorf("answering yes did not take the Plugin")
	}
}

// std: yoke:taking-a-plugin.04
func TestWhatTakingRefusesLeavesTheProjectAsItWas(t *testing.T) {
	app := authored(t)
	mark := func(dir string) string { return filepath.Join(filepath.Dir(dir), ".built") }

	nothing := t.TempDir()
	refused := pluginProject(t, strings.Replace(greeterFile, "language: go", "language: java", 1))
	elsewhere := pluginProject(t, strings.Replace(withRun(greeterBuild), "architectures: [x86_64, aarch64]", "architectures: [riscv]", 1))
	if HostArch() == "x86_64" {
		elsewhere = pluginProject(t, strings.Replace(withRun(greeterBuild), "architectures: [x86_64, aarch64]", "architectures: [aarch64]", 1))
	}
	failing := pluginProject(t, strings.Replace(withRun("exit 4"), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1))

	for _, c := range []struct {
		source, says string
		built        bool
	}{
		{nothing, PluginFile, false},
		{refused, "java", false},
		{elsewhere, HostArch(), false},
		{failing, "4", true},
	} {
		before := snapshot(t, app)
		code, out := takePlugin(t, "", c.source, "--yes", "--dir", app)
		if code == Done || !strings.Contains(out, c.says) {
			t.Errorf("%s: %d %s", c.source, code, out)
		}
		if _, err := os.Stat(mark(c.source)); err == nil && !c.built {
			t.Errorf("%s: the build ran", c.source)
		}
		sameAs(t, app, before)
	}

	// A file nobody's record accounts for, and a unit already declared, under the name the Plugin takes.
	source := greeterSource(t)
	os.MkdirAll(filepath.Join(app, Components, "plugins"), 0o755)
	os.WriteFile(filepath.Join(app, Components, "plugins", "greeter"), []byte("someone else's"), 0o755)
	before := snapshot(t, app)
	if code, out := takePlugin(t, "", source, "--yes", "--dir", app); code == Done || !strings.Contains(out, filepath.Join(Components, "plugins", "greeter")) {
		t.Errorf("a file not accounted for is not refused naming its path: %d %s", code, out)
	}
	sameAs(t, app, before)
	os.Remove(filepath.Join(app, Components, "plugins", "greeter"))
	must(t, "add", "unit", "greeter", "--kind", "oneshot", "--exec", "components/setup", "--dir", app)
	before = snapshot(t, app)
	if code, out := takePlugin(t, "", source, "--yes", "--dir", app); code == Done || !strings.Contains(out, "greeter") {
		t.Errorf("a unit already declared is not refused: %d %s", code, out)
	}
	sameAs(t, app, before)

	// The same Plugin twice: another revision is update plugin's.
	if code, out := takePlugin(t, "", source, "--as", "hello", "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin --as hello: %d %s", code, out)
	}
	before = snapshot(t, app)
	code, out := takePlugin(t, "", source, "--as", "hello-2", "--yes", "--dir", app)
	if code == Done || !strings.Contains(out, "com.example.greeter") || !strings.Contains(out, "update plugin") {
		t.Errorf("the same Plugin taken again is not refused pointing to update plugin: %d %s", code, out)
	}
	sameAs(t, app, before)
}

// std: yoke:taking-a-plugin.05
func TestRemovingTheUnitTakesThePluginsLineOutOfPluginsYaml(t *testing.T) {
	app, source := authored(t), greeterSource(t)
	other := pluginProject(t, strings.Replace(strings.Replace(withRun(greeterBuild), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1),
		"id: com.example.greeter\n", "id: com.example.hello\n", 1))
	os.WriteFile(filepath.Join(other, "plugin", "com.example.greeter.yaml"), []byte("manifest: 1\nid: com.example.hello\nprotocol: 1\n"), 0o644)
	os.Rename(filepath.Join(other, "plugin", "com.example.greeter.yaml"), filepath.Join(other, "plugin", "com.example.hello.yaml"))
	os.WriteFile(filepath.Join(other, PluginFile), []byte(strings.Replace(raw(t, filepath.Join(other, PluginFile)),
		"manifest: plugin/com.example.greeter.yaml", "manifest: plugin/com.example.hello.yaml", 1)), 0o644)
	for _, c := range [][]string{{source, "greeter"}, {other, "hello"}} {
		if code, out := takePlugin(t, "", c[0], "--as", c[1], "--yes", "--dir", app); code != Done {
			t.Fatalf("add plugin --as %s: %d %s", c[1], code, out)
		}
	}
	must(t, "remove", "units.hello", "--dir", app)
	if e := taken(t, app); len(e) != 1 || e[0]["unit"] != "greeter" {
		t.Errorf("after removing hello, plugins.yaml holds %v", e)
	}
	if written(t, filepath.Join(app, Descriptor), "units.hello") != nil {
		t.Errorf("the unit hello is still declared")
	}
	must(t, "remove", "units.greeter", "--dir", app)
	if _, err := os.Stat(filepath.Join(app, Plugins)); err == nil {
		t.Errorf("a project that holds no Plugin taken still has %s", Plugins)
	}
}

// std: yoke:taking-a-plugin.06
func TestTakingAPluginGrantsItNothing(t *testing.T) {
	app, source := authored(t), greeterSource(t)
	declaring := "manifest: 1\nid: com.example.greeter\nprotocol: 1\ncommands:\n  - id: greet\ncapabilities:\n  - { name: greet, governs: { command: greet } }\n"
	os.WriteFile(filepath.Join(source, "plugin", "com.example.greeter.yaml"), []byte(declaring), 0o644)
	git(t, source, "commit", "-q", "-am", "it declares a capability")
	before := snapshot(t, app)
	if code, out := takePlugin(t, "", source, "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	after := snapshot(t, app)
	for path := range after {
		if _, there := before[path]; there {
			continue
		}
		switch path {
		case filepath.Join(Components, "plugins", "greeter"), filepath.Join(Manifests, "com.example.greeter.yaml"), Plugins:
		default:
			t.Errorf("taking a Plugin wrote %s", path)
		}
	}
	for path, v := range before {
		if after[path] != v && path != Descriptor {
			t.Errorf("taking a Plugin changed %s", path)
		}
	}
	// The test's own name is in every path it writes, so a word is looked for as a field or a state.
	for _, word := range []string{"grant:", "grants:", "granted", "authorised", "authorized", "authorisation:", "authorization:"} {
		for path, v := range after {
			if path != filepath.Join(Manifests, "com.example.greeter.yaml") && strings.Contains(strings.ToLower(v), word) && !strings.Contains(strings.ToLower(before[path]), word) {
				t.Errorf("%s now names %q", path, word)
			}
		}
	}
	if units := written(t, filepath.Join(app, Descriptor), "units.greeter"); units != nil {
		for k := range units.(map[string]any) {
			if k != "kind" && k != "plugin" && k != "exec" && k != "digest" {
				t.Errorf("the unit carries %s", k)
			}
		}
	}
}
