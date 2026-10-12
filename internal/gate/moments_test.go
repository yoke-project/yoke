package gate_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/version"
)

const component = "#!/bin/sh\nexit 0\n"

// bundleOf writes a bundle whose descriptor has the head given and the units given after it, its
// component setup present with the digest stated unless written says otherwise.
func bundleOf(t *testing.T, head, units, written string) string {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	os.MkdirAll(filepath.Join(dir, "manifests"), 0o755)
	if written == "" {
		written = component
	}
	os.WriteFile(filepath.Join(dir, "components", "setup"), []byte(written), 0o755)
	os.WriteFile(filepath.Join(dir, "yoke.yaml"), []byte(fmt.Sprintf("model: 1\nid: com.yoke.station\nversion: 2.1.0\n%s"+
		"units:\n  setup: { kind: oneshot, exec: components/setup, digest: \"sha256:%x\" }\n%s", head, sha256.Sum256([]byte(component)), units)), 0o644)
	return dir
}

func checkAt(dir string, m gate.Moment, h *gate.Host) gate.Report {
	if h != nil {
		h.Bundle = dir
	}
	r, _ := gate.Check(gate.Input{Document: gate.Read(filepath.Join(dir, "yoke.yaml"), gate.Descriptor), Moment: m,
		Manifests: filepath.Join(dir, "manifests"), Host: h})
	return r
}

// std: yoke:the-moments.01
func TestAnotherArchitectureOrCoreIsRefusedWhereTheHostIsInHand(t *testing.T) {
	dir := bundleOf(t, "arch: riscv64\ncore: \">=99.0\"\n", "", "")
	core := version.MustParse("2.0.0")
	for _, m := range []gate.Moment{gate.Installing, gate.Creating, gate.Starting} {
		r := checkAt(dir, m, &gate.Host{Arch: "x86_64", Core: &core})
		wants(t, r, "arch.mismatch", "core.range")
		for _, f := range r.Findings {
			if !strings.Contains(f.Message, "riscv64") && !strings.Contains(f.Message, ">=99.0") {
				t.Errorf("%s does not name what disagrees: %s", f.Code, f.Message)
			}
		}
	}
	wants(t, checkAt(dir, gate.Composing, &gate.Host{Arch: "x86_64", Core: &core}))
}

// std: yoke:the-moments.02
func TestAComponentAndAManifestAreCheckedAgainstTheirDigests(t *testing.T) {
	dir := bundleOf(t, "arch: x86_64\ncore: \">=0.1\"\n",
		"  bench: { kind: plugin, plugin: com.example.bench, exec: components/setup, digest: \"sha256:"+fmt.Sprintf("%x", sha256.Sum256([]byte(component)))+
			"\", manifest_digest: \"sha256:"+strings.Repeat("0", 64)+"\" }\n", "#!/bin/sh\nexit 1\n")
	os.WriteFile(filepath.Join(dir, "manifests", "com.example.bench.yaml"), []byte(strings.Replace(minimal(""), "com.yoke.station.acquire", "com.example.bench", 1)), 0o644)
	r := checkAt(dir, gate.Creating, &gate.Host{})
	got := codes(r)
	if slices.Index(got, "component.digest") < 0 || slices.Index(got, "plugin.manifest.digest") < 0 {
		t.Fatalf("the findings are %v", got)
	}
	for _, f := range r.Findings {
		if f.Code == "plugin.manifest.digest" && !strings.Contains(f.Message, "com.example.bench.yaml") {
			t.Errorf("the finding does not name the Manifest: %s", f.Message)
		}
		if f.Code == "component.digest" && strings.Count(f.Message, "sha256:") != 2 {
			t.Errorf("the finding does not name both digests: %s", f.Message)
		}
	}
}

// std: yoke:the-moments.03
func TestAnAccountWithNoRuntimeDirectoryIsRefused(t *testing.T) {
	dir := bundleOf(t, "arch: x86_64\ncore: \">=0.1\"\n", "", "")
	for _, runtime := range []string{"", filepath.Join(t.TempDir(), "absent")} {
		for _, m := range []gate.Moment{gate.Creating, gate.Starting} {
			r := checkAt(dir, m, &gate.Host{RuntimeDir: &runtime})
			if !slices.Equal(codes(r), []string{"runtime.dir.missing"}) || !strings.Contains(r.Findings[0].Message, "loginctl enable-linger") {
				t.Errorf("%q: %v", runtime, r.Findings)
			}
		}
		wants(t, checkAt(dir, gate.Installing, &gate.Host{RuntimeDir: &runtime}))
	}
}
