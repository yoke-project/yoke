package gate_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/gate"
)

// scanned writes each Manifest under dir as the service form lays them out, `<plugin>/manifest.yaml`.
func scanned(t *testing.T, manifests map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for id, document := range manifests {
		os.MkdirAll(filepath.Join(dir, id), 0o755)
		if err := os.WriteFile(filepath.Join(dir, id, "manifest.yaml"), []byte(document), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// plain is a Manifest of the given plugin declaring nothing but the needs given.
func plain(id string, needs ...string) string {
	document := "manifest: 1\nid: " + id + "\nprotocol: 1\n"
	if len(needs) > 0 {
		document += "needs: [ \"" + strings.Join(needs, "\", \"") + "\" ]\n"
	}
	return document
}

// host is a host whose executables directory holds the given plugins' executables.
func host(t *testing.T, plugins ...string) *gate.Host {
	t.Helper()
	h := &gate.Host{Executables: t.TempDir(), StateDir: t.TempDir(), RuntimeRoot: t.TempDir()}
	for _, id := range plugins {
		if err := os.WriteFile(filepath.Join(h.Executables, id), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func started(t *testing.T, document, manifests string, h *gate.Host) (gate.Report, *gate.Deployment) {
	t.Helper()
	return gate.Check(gate.Input{
		Document:  gate.Document{Path: "/etc/yoke/deployments/bench.yaml", Kind: gate.Composition, Bytes: []byte(document)},
		Moment:    gate.Starting,
		Manifests: manifests,
		Host:      h,
	})
}

// std: yoke:the-composition-document.01
func TestACompositionDocumentHasNoHeadNoParametersAndNoDigests(t *testing.T) {
	digest := "sha256:" + strings.Repeat("ab", 32)
	r, _ := composition(t, `
model: 1
id: com.yoke.station
version: 2.1.0
arch: aarch64
core: ">=2.0 <3.0"
data: ">=2.0"
parameters: {}
units:
  exec: { kind: plugin, plugin: com.example.a, exec: /usr/bin/a }
  image: { kind: plugin, plugin: com.example.b, image: "ghcr.io/example/b@`+digest+`" }
  pinned: { kind: plugin, plugin: com.example.c, manifest_digest: "`+digest+`" }
  panel: { kind: interface, exec: /usr/bin/panel, digest: "`+digest+`" }
  step: { kind: oneshot, exec: /usr/bin/step, migrates_from: "<2.0" }
`)
	wants(t, r, "head.present", "head.present", "head.present", "head.present", "head.present", "head.present",
		"parameters.present", "unit.program.on_plugin", "unit.program.on_plugin", "digest.present", "digest.present",
		"unit.migrates_from.present")
}

// std: yoke:the-composition-document.02
func TestTheManifestsAreFoundInTheScannedDirectory(t *testing.T) {
	dir := scanned(t, map[string]string{"com.yoke.station.acquire": plain("com.yoke.station.acquire")})
	r, _ := gate.Check(gate.Input{
		Document:  gate.Document{Path: "bench.yaml", Kind: gate.Composition, Bytes: []byte("units:\n  acquire: { kind: plugin, plugin: com.yoke.station.acquire }\n  pipeline: { kind: plugin, plugin: com.yoke.station.pipeline }\n")},
		Manifests: dir,
	})
	wants(t, r, "plugin.manifest.missing")
	f := r.Findings[0]
	path := filepath.Join(dir, "com.yoke.station.pipeline", "manifest.yaml")
	if f.Location != "units.pipeline.plugin" || !strings.Contains(f.Message, "com.yoke.station.pipeline") || !strings.Contains(f.Message, path) {
		t.Errorf("the refusal is %+v", f)
	}
}

// std: yoke:the-composition-document.03
func TestAManifestTheCompositionNamesIsChecked(t *testing.T) {
	dir := scanned(t, map[string]string{
		"com.example.old":    "manifest: 2\nid: com.example.old\nprotocol: 1\n",
		"com.example.odd":    plain("com.example.odd") + "colour: red\n",
		"com.example.unused": "manifest: [unclosed",
	})
	r, _ := gate.Check(gate.Input{
		Document:  gate.Document{Path: "bench.yaml", Kind: gate.Composition, Bytes: []byte("units:\n  old: { kind: plugin, plugin: com.example.old }\n  odd: { kind: plugin, plugin: com.example.odd }\n")},
		Manifests: dir,
	})
	wants(t, r, "plugin.manifest.model", "key.unknown")
	for _, f := range r.Findings {
		want := map[string]string{"plugin.manifest.model": "com.example.old", "key.unknown": "com.example.odd"}[f.Code]
		if f.Document != filepath.Join(dir, want, "manifest.yaml") {
			t.Errorf("%s is located in %q, want %s's Manifest", f.Code, f.Document, want)
		}
	}
}

// std: yoke:the-composition-document.04
func TestAPluginUnitsNeedsAreJoinedToItsBinding(t *testing.T) {
	dir := scanned(t, map[string]string{"com.example.acq": plain("com.example.acq", "device:instrument", "device:spare", "secret:token")})
	unit := func(bind, args string) string {
		return "units:\n  acq:\n    kind: plugin\n    plugin: com.example.acq\n    bind: { " + bind + " }\n    args: [ " + args + " ]\n"
	}
	both := "instrument: /dev/a, spare: /dev/b"
	for _, c := range []struct {
		document, code, names string
	}{
		{unit(both, `"${bind.instrument}", "${bind.token}"`), "", ""},
		{unit("instrument: /dev/a", `"x"`), "unit.needs.unbound", "spare"},
		{unit(both+", other: /dev/c", `"x"`), "bind.undeclared", "other"},
		{unit(both+", token: /etc/token", `"x"`), "bind.secret", "token"},
		{unit(both, `"${bind.missing}"`), "substitution.unresolved", "${bind.missing}"},
	} {
		r, _ := gate.Check(gate.Input{Document: gate.Document{Path: "bench.yaml", Kind: gate.Composition, Bytes: []byte(c.document)}, Manifests: dir})
		if c.code == "" {
			if len(r.Findings) != 0 {
				t.Errorf("%s\n  gives %v, want nothing", c.document, codes(r))
			}
			continue
		}
		if got := codes(r); !slices.Equal(got, []string{c.code}) || !strings.Contains(r.Findings[0].Message, c.names) {
			t.Errorf("%s\n  gives %v (%v), want [%s] naming %s", c.document, got, r.Findings, c.code, c.names)
		}
	}
}

// hostCase is case 05's composition, its Manifests and its host.
func hostCase(t *testing.T) (string, string, *gate.Host) {
	t.Helper()
	dir := scanned(t, map[string]string{
		"com.example.present": plain("com.example.present", "device:instrument", "secret:token"),
		"com.example.absent":  plain("com.example.absent"),
	})
	h := host(t, "com.example.present")
	unexecutable := filepath.Join(t.TempDir(), "step")
	os.WriteFile(unexecutable, []byte("#!/bin/sh\n"), 0o644)
	document := fmt.Sprintf(`
units:
  panel: { kind: interface, exec: %s }
  step: { kind: oneshot, exec: %s }
  absent: { kind: plugin, plugin: com.example.absent }
  present: { kind: plugin, plugin: com.example.present, bind: { instrument: %s } }
`, filepath.Join(t.TempDir(), "nowhere"), unexecutable, filepath.Join(t.TempDir(), "spectro-head-a"))
	return document, dir, h
}

// std: yoke:the-composition-document.05
func TestWhatTheHostMustHold(t *testing.T) {
	document, dir, h := hostCase(t)
	r, _ := started(t, document, dir, h)
	wants(t, r, "component.missing", "component.missing", "component.not_executable", "path.missing", "secret.missing")
	for _, f := range found(r, "path.missing") {
		if !strings.Contains(f.Message, "spectro-head-a") {
			t.Errorf("the refusal %q does not name the path", f.Message)
		}
	}
	for _, f := range found(r, "secret.missing") {
		if !strings.Contains(f.Message, "token") {
			t.Errorf("the refusal %q does not name the secret", f.Message)
		}
	}
}

// std: yoke:the-composition-document.06
func TestTheLongestSocketPathMustFit(t *testing.T) {
	h := host(t, "com.example.acq")
	unitName := "acq"
	fixed := len(h.RuntimeRoot) + len("/plugins/") + len(unitName) + len("/subscribers/") + len("/") + gate.SubscriberWidth
	for _, total := range []int{107, 108} {
		stream := strings.Repeat("s", total-fixed)
		manifest := "manifest: 1\nid: com.example.acq\nprotocol: 1\nstreams: [ { id: " + stream + " } ]\ncapabilities: [ { name: a, governs: { stream: " + stream + " } } ]\n"
		dir := scanned(t, map[string]string{"com.example.acq": manifest})
		r, _ := started(t, "units:\n  "+unitName+": { kind: plugin, plugin: com.example.acq }\n", dir, h)
		switch total {
		case 107:
			if len(r.Findings) != 0 {
				t.Errorf("a longest path of 107 gives %v", codes(r))
			}
		case 108:
			if got := codes(r); !slices.Equal(got, []string{"socket.path.ceiling"}) || !strings.Contains(r.Findings[0].Message, "108") || !strings.Contains(r.Findings[0].Message, "107") {
				t.Errorf("a longest path of 108 gives %v", r.Findings)
			}
		}
	}
}

// std: yoke:the-composition-document.07
func TestEachCheckRunsAtTheMomentsThatCanReachIt(t *testing.T) {
	document, dir, h := hostCase(t)
	r, _ := gate.Check(gate.Input{
		Document: gate.Document{Path: "bench.yaml", Kind: gate.Composition, Bytes: []byte(document)},
		Moment:   gate.Composing, Manifests: dir, Host: h,
	})
	if len(r.Findings) != 0 {
		t.Errorf("composing reported %v", codes(r))
	}
	if !slices.Contains(r.NotRun, gate.PhaseHostFacts) || slices.Contains(r.NotRun, gate.PhaseCrossDocument) {
		t.Errorf("composing left %v not run, want the host facts and not the cross-document phase", r.NotRun)
	}
}

// std: yoke:the-composition-document.08
func TestTheServiceFormsBenchPasses(t *testing.T) {
	dir := scanned(t, map[string]string{
		"com.yoke.station.acquire":  plain("com.yoke.station.acquire", "device:instrument"),
		"com.yoke.station.pipeline": plain("com.yoke.station.pipeline"),
		"com.yoke.station.archive":  plain("com.yoke.station.archive"),
	})
	h := host(t, "com.yoke.station.acquire", "com.yoke.station.pipeline", "com.yoke.station.archive")
	devices := t.TempDir()
	for _, head := range []string{"a", "b", "c", "d"} {
		os.WriteFile(filepath.Join(devices, "spectro-head-"+head), nil, 0o644)
	}
	panel := filepath.Join(t.TempDir(), "station-panel")
	os.WriteFile(panel, []byte("#!/bin/sh\n"), 0o755)
	var acquire strings.Builder
	for i, head := range []string{"a", "b", "c", "d"} {
		fmt.Fprintf(&acquire, "  acquire-%d:\n    kind: plugin\n    plugin: com.yoke.station.acquire\n    bind: { instrument: %s }\n    args: [ \"--device\", \"${bind.instrument}\" ]\n",
			i+1, filepath.Join(devices, "spectro-head-"+head))
	}
	document := `
policy:
  startup_window: 60s

units:
` + acquire.String() + `
  pipeline:
    kind: plugin
    plugin: com.yoke.station.pipeline
    args: [ "--site", "Bench A — Metrology" ]
    depends_on: [ acquire-1, acquire-2, acquire-3, acquire-4 ]

  archive:
    kind: plugin
    plugin: com.yoke.station.archive

  panel:
    kind: interface
    exec: ` + panel + `
    needs: [ display ]

channels:
  panel:   { unit: panel, transport: local, clients: single }
  remote:  { transport: http+ws, clients: single,
             address: { class: loopback, port: 8080 } }

arbitration:
  - prevails: panel
    over: [ remote ]
`
	r, d := started(t, document, dir, h)
	if r.Refused() || d == nil {
		t.Fatalf("the bench was refused: %+v", r.Findings)
	}
	wants(t, r, "channel.address.loopback")
	if len(d.Units) != 7 || len(d.Channels) != 2 || len(d.Arbitration) != 1 {
		t.Errorf("the deployment has %d units, %d channels and %d rules", len(d.Units), len(d.Channels), len(d.Arbitration))
	}
}
