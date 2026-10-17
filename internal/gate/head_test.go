package gate_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/gate"
)

const identity = "model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: aarch64\ncore: \">=2.0 <3.0\"\n"

func descriptor(t *testing.T, document string) (gate.Report, *gate.Deployment) {
	t.Helper()
	return gate.Check(gate.Input{Document: gate.Document{Path: "yoke.yaml", Kind: gate.Descriptor, Bytes: []byte(document)}})
}

const unit = "units: { a: { kind: oneshot, exec: components/a, digest: \"sha256:" +
	"abababababababababababababababababababababababababababababababab\" } }\n"

// std: yoke:the-descriptor-head.01
func TestModelIsReadFirst(t *testing.T) {
	for _, c := range []struct {
		document string
		want     []string
	}{
		{strings.Replace(identity, "model: 1\n", "", 1) + unit, []string{"field.required"}},
		{strings.Replace(identity, "model: 1", "model: one", 1) + unit, []string{"field.type"}},
		{strings.Replace(identity, "model: 1", "model: 2", 1) + "later: true\n" + unit, []string{"model.unknown"}},
	} {
		r, _ := descriptor(t, c.document)
		if got := codes(r); !slices.Equal(got, c.want) {
			t.Errorf("%s\n  gives %v, want %v", c.document, got, c.want)
		}
		if !slices.Contains(r.NotRun, gate.PhaseShape) {
			t.Errorf("%s\n  ran the shape phase: not run %v", c.document, r.NotRun)
		}
	}
	if r, _ := descriptor(t, identity+unit); r.Refused() {
		t.Errorf("model 1 was refused: %v", codes(r))
	}
}

// std: yoke:the-descriptor-head.02
func TestTheIdentityIsHandedOnWithTheDeployment(t *testing.T) {
	r, d := descriptor(t, identity+"data: \">=2.0\"\n"+unit)
	if r.Refused() {
		t.Fatalf("refused: %v", codes(r))
	}
	want := gate.Head{Model: 1, ID: "com.yoke.station", Version: "2.1.0", Arch: "aarch64", Core: ">=2.0 <3.0", Data: ">=2.0"}
	if d.Head != want {
		t.Errorf("the head is %+v, want %+v", d.Head, want)
	}
}

// std: yoke:the-descriptor-head.03
func TestEachFieldOfTheIdentityIsRequiredAndMustBeWhatItSays(t *testing.T) {
	for _, field := range []string{"id", "version", "arch", "core"} {
		var kept []string
		for _, line := range strings.Split(strings.TrimSpace(identity), "\n") {
			if !strings.HasPrefix(line, field+":") {
				kept = append(kept, line)
			}
		}
		r, _ := descriptor(t, strings.Join(kept, "\n")+"\n"+unit)
		if got := found(r, "field.required"); len(got) != 1 || got[0].Location != field {
			t.Errorf("without %s: %v", field, codes(r))
		}
	}
	for _, c := range []struct{ from, to, code, written string }{
		{"id: com.yoke.station", "id: Station", "field.value", "Station"},
		{"version: 2.1.0", "version: \"2.1\"", "version.not_semver", "2.1"},
		{"core: \">=2.0 <3.0\"", "core: \"2.0\"", "field.value", "2.0"},
		{"core: \">=2.0 <3.0\"", "core: \">=2.0 <3.0\"\ndata: \"~2\"", "field.value", "~2"},
	} {
		r, _ := descriptor(t, strings.Replace(identity, c.from, c.to, 1)+unit)
		got := found(r, c.code)
		if len(got) != 1 {
			t.Errorf("%s gives %v, want one %s", c.to, codes(r), c.code)
			continue
		}
		if !strings.Contains(got[0].Message, c.written) {
			t.Errorf("%s: the refusal does not name %q: %s", c.to, c.written, got[0].Message)
		}
	}
}

// std: yoke:the-descriptor-head.04
func TestInsideABundleAManifestIsNamedByItsPlugin(t *testing.T) {
	dir := t.TempDir()
	manifests := filepath.Join(dir, "manifests")
	if err := os.MkdirAll(manifests, 0o755); err != nil {
		t.Fatal(err)
	}
	document := identity + "units: { acquire: { kind: plugin, plugin: com.yoke.station.acquire, exec: components/acquire, digest: \"sha256:" +
		strings.Repeat("ab", 32) + "\" } }\n"
	in := gate.Input{Document: gate.Document{Path: filepath.Join(dir, "yoke.yaml"), Kind: gate.Descriptor, Bytes: []byte(document)},
		Moment: gate.Composing, Manifests: manifests}
	path := filepath.Join(manifests, "com.yoke.station.acquire.yaml")
	if err := os.WriteFile(path, []byte("manifest: 1\nid: com.yoke.station.acquire\nprotocol: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r, _ := gate.Check(in); r.Refused() {
		t.Errorf("with its Manifest: %v", r.Findings)
	}
	os.Remove(path)
	r, _ := gate.Check(in)
	got := found(r, "plugin.manifest.missing")
	if len(got) != 1 || !strings.Contains(got[0].Message, "manifests/com.yoke.station.acquire.yaml") {
		t.Errorf("without it: %v", r.Findings)
	}
}

// std: yoke:the-descriptor-head.05
func TestAStampOutsideTheRangeIsRefusedNamingTheRangeAsRead(t *testing.T) {
	err := gate.Readable(">=1.0", "1.1.0", "1.0.0")
	if err == nil || !strings.Contains(err.Error(), ">=1.0 up to 1.0.0") || !strings.Contains(err.Error(), "1.1.0") {
		t.Errorf("an open range: %v", err)
	}
	err = gate.Readable(">=1.0 <2.1", "2.1.0", "2.0.0")
	if err == nil || !strings.Contains(err.Error(), ">=1.0 <2.1,") || strings.Contains(err.Error(), "up to") || !strings.Contains(err.Error(), "2.1.0") {
		t.Errorf("a closed range: %v", err)
	}
}
