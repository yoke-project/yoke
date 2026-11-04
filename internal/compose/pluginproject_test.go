package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/gate"
)

// greeterFile is the Plugin project's file the Go template would write for com.example.greeter.
const greeterFile = `plugin: 1
id: com.example.greeter
language: go
program: greeter
manifest: plugin/com.example.greeter.yaml
build:
  in: plugin/
  run: CGO_ENABLED=0 GOOS=linux GOARCH="$YOKE_GOARCH" go build -trimpath -o "$YOKE_OUT/greeter" .
architectures: [x86_64, aarch64]
`

const greeterManifest = "manifest: 1\nid: com.example.greeter\nprotocol: 1\n"

const pinned = "registry.example/build/go@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// pluginProject is a Plugin project holding the file given, the Manifest kept in plugin/, and try/, an
// application project made by init.
func pluginProject(t *testing.T, file string) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "plugin"), 0o755)
	os.WriteFile(filepath.Join(dir, PluginFile), []byte(file), 0o644)
	os.WriteFile(filepath.Join(dir, "plugin", "com.example.greeter.yaml"), []byte(greeterManifest), 0o644)
	if code, out := run("init", "com.example.greeter-try", "--dir", filepath.Join(dir, "try")); code != Done {
		t.Fatalf("init try/: %d %s", code, out)
	}
	return dir
}

// edited is the greeter's file with one line replaced, removed (new empty) or added (old empty).
func edited(old, new string) string {
	if old == "" {
		return greeterFile + new + "\n"
	}
	if !strings.Contains(greeterFile, old) {
		panic("the greeter's file has no " + old)
	}
	if new == "" {
		return strings.Replace(greeterFile, old+"\n", "", 1)
	}
	return strings.Replace(greeterFile, old, new, 1)
}

// refusedAt is the refusal at the location given, which must be there and say each of the words.
func refusedAt(t *testing.T, r gate.Report, location string, words ...string) {
	t.Helper()
	for _, f := range r.Findings {
		if f.Class != gate.Refusal || f.Location != location {
			continue
		}
		for _, w := range words {
			if !strings.Contains(f.Message, w) {
				t.Errorf("the refusal at %q does not say %q: %s", location, w, f.Message)
			}
		}
		return
	}
	t.Errorf("nothing refused at %q; the report: %+v", location, r.Findings)
}

// std: yoke:the-plugin-project.01
func TestAFileTheTemplateWouldWriteIsAcceptedWithItsManifest(t *testing.T) {
	dir := pluginProject(t, greeterFile)
	report, p := CheckPlugin(dir)
	if report.Refused() || len(report.Findings) != 0 || p == nil {
		t.Fatalf("the project is not accepted: %+v", report.Findings)
	}
	if p.ID != "com.example.greeter" || p.Language != "go" || p.Program != "greeter" ||
		p.Manifest != "plugin/com.example.greeter.yaml" || p.BuildIn != "plugin/" || p.BuildContainer != "" ||
		!strings.Contains(p.BuildRun, `-o "$YOKE_OUT/greeter"`) || strings.Join(p.Architectures, ",") != "x86_64,aarch64" {
		t.Errorf("what was read is not what was written: %+v", p)
	}
	if p.Declared == nil || p.Declared.ID != "com.example.greeter" ||
		p.Declared.Path != filepath.Join(dir, "plugin", "com.example.greeter.yaml") {
		t.Errorf("the Manifest is not read from the path the file names: %+v", p.Declared)
	}
	// try/ is an application project of its own: a refusal there is nothing of the Plugin's.
	os.WriteFile(filepath.Join(dir, "try", Descriptor), []byte("not: [a descriptor"), 0o644)
	if report, _ := CheckPlugin(dir); report.Refused() {
		t.Errorf("something under try/ was read as the Plugin's: %+v", report.Findings)
	}

	dir = pluginProject(t, edited("architectures:", "  container: "+pinned+"\narchitectures:"))
	report, p = CheckPlugin(dir)
	if report.Refused() || p == nil || p.BuildContainer != pinned {
		t.Errorf("the image the build runs in is not read: %+v %+v", report.Findings, p)
	}
}

// std: yoke:the-plugin-project.02
func TestAFieldMissingOrOfTheWrongKindIsRefusedByItsName(t *testing.T) {
	removed := map[string]string{
		"plugin": "plugin: 1", "id": "id: com.example.greeter", "language": "language: go", "program": "program: greeter",
		"manifest": "manifest: plugin/com.example.greeter.yaml", "build.in": "  in: plugin/",
		"build.run":     `  run: CGO_ENABLED=0 GOOS=linux GOARCH="$YOKE_GOARCH" go build -trimpath -o "$YOKE_OUT/greeter" .`,
		"architectures": "architectures: [x86_64, aarch64]",
	}
	for field, line := range removed {
		report, p := CheckPlugin(pluginProject(t, edited(line, "")))
		if p != nil {
			t.Errorf("without %s the project is accepted", field)
		}
		refusedAt(t, report, field)
	}
	for _, c := range []struct{ old, new, at, says string }{
		{"plugin: 1", "plugin: 2", "plugin", "2"},
		{"language: go", "language: java", "language", "java"},
		{"architectures: [x86_64, aarch64]", "architectures: []", "architectures", ""},
		{"", "colour: blue", "colour", ""},
	} {
		report, p := CheckPlugin(pluginProject(t, edited(c.old, c.new)))
		if p != nil {
			t.Errorf("%q is accepted", c.new)
		}
		refusedAt(t, report, c.at, c.says)
	}
	// The families are named, so that the author sees which word was meant.
	report, _ := CheckPlugin(pluginProject(t, edited("language: go", "language: java")))
	refusedAt(t, report, "language", "go", "rust", "python", "c", "cpp")

	dir := pluginProject(t, greeterFile)
	os.WriteFile(filepath.Join(dir, PluginFile), []byte("plugin: [1\n"), 0o644)
	report, p := CheckPlugin(dir)
	if p != nil {
		t.Errorf("a file that is not YAML is accepted")
	}
	refusedAt(t, report, "", PluginFile)
}

// std: yoke:the-plugin-project.03
func TestTheFileCarriesNoVersion(t *testing.T) {
	report, p := CheckPlugin(pluginProject(t, edited("", "version: 1.2.0")))
	if p != nil {
		t.Errorf("a file carrying a version is accepted")
	}
	refusedAt(t, report, "version", "tag")
	if report, p := CheckPlugin(pluginProject(t, greeterFile)); report.Refused() || p == nil {
		t.Errorf("the same file without it is refused: %+v", report.Findings)
	}
}

// std: yoke:the-plugin-project.04
func TestTheIdentityIsTheManifests(t *testing.T) {
	report, p := CheckPlugin(pluginProject(t, edited("id: com.example.greeter", "id: com.example.other")))
	if p != nil {
		t.Errorf("an identity the Manifest does not carry is accepted")
	}
	refusedAt(t, report, "id", "com.example.other", "com.example.greeter")

	dir := pluginProject(t, greeterFile)
	os.Remove(filepath.Join(dir, "plugin", "com.example.greeter.yaml"))
	report, p = CheckPlugin(dir)
	if p != nil {
		t.Errorf("a project whose Manifest is missing is accepted")
	}
	refusedAt(t, report, "manifest", "plugin/com.example.greeter.yaml")

	dir = pluginProject(t, greeterFile)
	os.WriteFile(filepath.Join(dir, "plugin", "com.example.greeter.yaml"), []byte("manifest: 1\nid: com.example.greeter\n"), 0o644)
	report, p = CheckPlugin(dir)
	if p != nil {
		t.Errorf("a project whose Manifest the gate refuses is accepted")
	}
	found := false
	for _, f := range report.Findings {
		if f.Class == gate.Refusal && f.Document == filepath.Join(dir, "plugin", "com.example.greeter.yaml") && f.Location == "protocol" {
			found = true
		}
	}
	if !found {
		t.Errorf("the gate's finding on the Manifest is not reported: %+v", report.Findings)
	}
}

// std: yoke:the-plugin-project.05
func TestEveryPathStaysInsideTheProjectAndTheManifestInsideTheBuildsDirectory(t *testing.T) {
	cases := []struct{ old, new, at, says string }{
		{"  in: plugin/", "  in: ../elsewhere/", "build.in", "../elsewhere/"},
		{"  in: plugin/", "  in: /srv/greeter/", "build.in", "/srv/greeter/"},
		{"manifest: plugin/com.example.greeter.yaml", "manifest: ../greeter.yaml", "manifest", "../greeter.yaml"},
		{"manifest: plugin/com.example.greeter.yaml", "manifest: try/manifests/com.example.greeter.yaml", "manifest", "plugin/"},
		{"  in: plugin/", "  in: source/", "build.in", "source/"},
		{"program: greeter", "program: bin/greeter", "program", "bin/greeter"},
	}
	for _, c := range cases {
		dir := pluginProject(t, edited(c.old, c.new))
		os.WriteFile(filepath.Join(dir, "try", Manifests, "com.example.greeter.yaml"), []byte(greeterManifest), 0o644)
		report, p := CheckPlugin(dir)
		if p != nil {
			t.Errorf("%q is accepted", c.new)
		}
		refusedAt(t, report, c.at, c.says)
	}
}

// std: yoke:the-plugin-project.06
func TestAnArchitectureTheBuildCannotBeToldIsRefused(t *testing.T) {
	for _, c := range []struct{ list, says string }{
		{"[amd64]", "x86_64"},
		{"[x86_64, riscv64]", "riscv64"},
		{"[x86_64, x86_64]", "x86_64"},
	} {
		report, p := CheckPlugin(pluginProject(t, edited("architectures: [x86_64, aarch64]", "architectures: "+c.list)))
		if p != nil {
			t.Errorf("%s is accepted", c.list)
		}
		refusedAt(t, report, "architectures", c.says)
	}
}

// std: yoke:the-plugin-project.07
func TestADirectoryIsAPluginProjectAnApplicationProjectOrNeither(t *testing.T) {
	dir := pluginProject(t, greeterFile)
	if kind, err := KindOf(dir); err != nil || kind != PluginProject {
		t.Errorf("the Plugin project is %v, %v", kind, err)
	}
	if kind, err := KindOf(filepath.Join(dir, "try")); err != nil || kind != ApplicationProject {
		t.Errorf("try/ is %v, %v", kind, err)
	}
	both := pluginProject(t, greeterFile)
	os.WriteFile(filepath.Join(both, Descriptor), []byte("model: 1\n"), 0o644)
	if _, err := KindOf(both); err == nil || !strings.Contains(err.Error(), PluginFile) || !strings.Contains(err.Error(), Descriptor) {
		t.Errorf("a directory holding both files is not refused naming them: %v", err)
	}
	if _, err := KindOf(t.TempDir()); err == nil || !strings.Contains(err.Error(), PluginFile) || !strings.Contains(err.Error(), Descriptor) {
		t.Errorf("an empty directory is not refused naming the two files: %v", err)
	}
}
