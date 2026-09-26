package gate_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/gate"
)

const acquire = `
manifest: 1
id: com.yoke.station.acquire
protocol: 1

needs:
  - device:instrument

streams:
  - id: station.spectra
  - id: station.preview
    tolerates_loss: true
    tolerates_reorder: true

commands:
  - id: calibrate

queries:
  - id: head-status

occurrences:
  - id: calibration.drift

capabilities:
  - name: stream.spectra.publish
    governs: { stream: station.spectra }
  - name: stream.preview.publish
    governs: { stream: station.preview }
  - name: command.calibrate.accept
    governs: { command: calibrate }
  - name: query.head-status.answer
    governs: { query: head-status }
  - name: event.calibration-drift.report
    governs: { occurrence: calibration.drift }
`

const installed = "/etc/yoke/plugins.d/com.yoke.station.acquire/manifest.yaml"

func manifest(t *testing.T, path, document string) (gate.Report, *gate.Manifest) {
	t.Helper()
	return gate.CheckManifest(gate.Document{Path: path, Bytes: []byte(document)})
}

// minimal is a Manifest of one stream and its capability, with extra appended at the top level.
func minimal(extra string) string {
	return `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
streams:
  - id: station.spectra
capabilities:
  - name: stream.spectra.publish
    governs: { stream: station.spectra }
` + extra
}

// std: yoke:the-manifest.01
func TestAManifestIsReadIntoWhatItDeclares(t *testing.T) {
	r, m := manifest(t, installed, acquire)
	if len(r.Findings) != 0 || m == nil {
		t.Fatalf("the reference Manifest gave %v", codes(r))
	}
	want := &gate.Manifest{
		Path: installed, ID: "com.yoke.station.acquire", Protocol: 1,
		Needs: []gate.Need{{Class: "device", Name: "instrument"}},
		Streams: []gate.Stream{
			{ID: "station.spectra"},
			{ID: "station.preview", ToleratesLoss: true, ToleratesReorder: true},
		},
		Commands:    []string{"calibrate"},
		Queries:     []string{"head-status"},
		Occurrences: []string{"calibration.drift"},
		Capabilities: []gate.Capability{
			{Name: "stream.spectra.publish", Governs: gate.Object{Kind: "stream", ID: "station.spectra"}},
			{Name: "stream.preview.publish", Governs: gate.Object{Kind: "stream", ID: "station.preview"}},
			{Name: "command.calibrate.accept", Governs: gate.Object{Kind: "command", ID: "calibrate"}},
			{Name: "query.head-status.answer", Governs: gate.Object{Kind: "query", ID: "head-status"}},
			{Name: "event.calibration-drift.report", Governs: gate.Object{Kind: "occurrence", ID: "calibration.drift"}},
		},
	}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("read as\n%+v\nwant\n%+v", m, want)
	}
}

// std: yoke:the-manifest.02
func TestManifestIsReadFirst(t *testing.T) {
	r, _ := manifest(t, installed, "manifest: 2\ncolour: red\nprotocol: 1\n")
	wants(t, r, "plugin.manifest.model")
	if !strings.Contains(r.Findings[0].Message, "2") {
		t.Errorf("the refusal %q does not name the model", r.Findings[0].Message)
	}
	if !slices.Equal(r.NotRun, []string{gate.PhaseShape, gate.PhaseInternalJoins, gate.PhaseCrossDocument, gate.PhaseHostFacts, gate.PhaseWeaker}) {
		t.Errorf("after an unknown model the phases not run are %v", r.NotRun)
	}
	r, _ = manifest(t, installed, "id: com.yoke.station.acquire\nprotocol: 1\n")
	wants(t, r, "field.required")
	if r.Findings[0].Location != "manifest" {
		t.Errorf("the refusal is at %q", r.Findings[0].Location)
	}
}

// std: yoke:the-manifest.03
func TestAnUnknownKeyIsRefusedAtAnyLevelOfAManifest(t *testing.T) {
	r, _ := manifest(t, installed, `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
colour: red
startup_window: 90s
streams:
  - { id: station.spectra, colour: red }
capabilities:
  - name: stream.spectra.publish
    colour: red
    governs: { stream: station.spectra, colour: red }
`)
	var locations []string
	for _, f := range found(r, "key.unknown") {
		locations = append(locations, f.Location)
	}
	slices.Sort(locations)
	want := []string{"capabilities.0.colour", "capabilities.0.governs.colour", "colour", "startup_window", "streams.0.colour"}
	if !slices.Equal(locations, want) || len(r.Findings) != len(want) {
		t.Errorf("refused at %v (%v), want %v", locations, codes(r), want)
	}
}

// std: yoke:the-manifest.04
func TestEveryValueIsTypedByTheSchema(t *testing.T) {
	for _, c := range []struct {
		path, document, want string
	}{
		{installed, "manifest: 1\nid: com.yoke.station.acquire\n", "field.required"},
		{installed, "manifest: 1\nid: com.yoke.station.acquire\nprotocol: one\n", "field.type"},
		{installed, strings.Replace(minimal(""), "- id: station.spectra", "- { id: station.spectra, tolerates_loss: yes }", 1), "field.type"},
		{"/etc/yoke/plugins.d/no/manifest.yaml", "manifest: 1\nid: no\nprotocol: 1\n", "field.value"},
	} {
		r, _ := manifest(t, c.path, c.document)
		if got := codes(r); !slices.Equal(got, []string{c.want}) {
			t.Errorf("%s\n  gives %v, want [%s]", c.document, got, c.want)
		}
	}
}

// std: yoke:the-manifest.05
func TestTheIdentityInThePathAndInIDAgree(t *testing.T) {
	r, _ := manifest(t, "bundle/manifests/com.yoke.station.acquire.yaml", acquire)
	if len(r.Findings) != 0 {
		t.Errorf("a bundle's Manifest gave %v", codes(r))
	}
	r, _ = manifest(t, "/etc/yoke/plugins.d/com.yoke.station.pipeline/manifest.yaml", acquire)
	wants(t, r, "plugin.manifest.id")
	if m := r.Findings[0].Message; !strings.Contains(m, "com.yoke.station.pipeline") || !strings.Contains(m, "com.yoke.station.acquire") {
		t.Errorf("the refusal %q does not name both identities", m)
	}
}

// std: yoke:the-manifest.06
func TestAProtocolTheCoreDoesNotSupportIsRefused(t *testing.T) {
	r, _ := manifest(t, installed, strings.Replace(minimal(""), "protocol: 1", "protocol: 7", 1))
	wants(t, r, "plugin.protocol.unsupported")
	if m := r.Findings[0].Message; !strings.Contains(m, "7") || !strings.Contains(m, "1") {
		t.Errorf("the refusal %q does not name 7 and what this Core speaks", m)
	}
}

// std: yoke:the-manifest.07
func TestANeedIsAClassFromTheClosedSet(t *testing.T) {
	r, m := manifest(t, installed, minimal("needs: [ \"device:instrument\", \"secret:token\", network ]\n"))
	if len(r.Findings) != 0 || len(m.Needs) != 3 {
		t.Fatalf("the needs gave %v and %+v", codes(r), m)
	}
	r, _ = manifest(t, installed, minimal("needs: [ /dev/ttyUSB0 ]\n"))
	wants(t, r, "field.value")
}

// std: yoke:the-manifest.08
func TestAStreamIdentifierIsAPathComponentAndACapabilityNameIsNot(t *testing.T) {
	for _, stream := range []string{"a/b", "..", "."} {
		document := strings.ReplaceAll(minimal(""), "station.spectra", stream)
		r, _ := manifest(t, installed, strings.ReplaceAll(document, "id: "+stream, "id: \""+stream+"\""))
		if got := codes(r); !slices.Equal(got, []string{"name.not_a_path_component"}) {
			t.Errorf("the stream %q gives %v", stream, got)
		}
	}
	r, _ := manifest(t, installed, strings.Replace(minimal(""), "name: stream.spectra.publish", "name: \"stream/spectra:publish\"", 1))
	if len(r.Findings) != 0 {
		t.Errorf("a capability name with a separator gave %v", codes(r))
	}
}

// std: yoke:the-manifest.09
func TestBothTolerancesDefaultToFalse(t *testing.T) {
	_, m := manifest(t, installed, `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
streams:
  - id: plain
  - { id: shuffled, tolerates_reorder: true }
capabilities:
  - { name: a, governs: { stream: plain } }
  - { name: b, governs: { stream: shuffled } }
`)
	if m == nil {
		t.Fatal("the Manifest was refused")
	}
	want := []gate.Stream{{ID: "plain"}, {ID: "shuffled", ToleratesReorder: true}}
	if !reflect.DeepEqual(m.Streams, want) {
		t.Errorf("the streams are %+v, want %+v", m.Streams, want)
	}
}

// std: yoke:the-manifest.10
func TestCapabilitiesAndObjectsAreJoinedInBothDirections(t *testing.T) {
	r, _ := manifest(t, installed, `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
streams:
  - id: station.spectra
commands:
  - id: calibrate
queries:
  - id: head-status
capabilities:
  - { name: a, governs: { stream: station.spectra } }
  - { name: b, governs: { command: calibrate } }
  - { name: c, governs: { command: reboot } }
  - { name: d, governs: {} }
  - { name: e, governs: { stream: station.spectra, command: calibrate } }
`)
	wants(t, r, "capability.governs.count", "capability.governs.count")
	// A governs block that names no single object leaves the join untrustworthy; once it is right, the
	// join runs in both directions.
	r, _ = manifest(t, installed, `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
streams:
  - id: station.spectra
commands:
  - id: calibrate
queries:
  - id: head-status
capabilities:
  - { name: a, governs: { stream: station.spectra } }
  - { name: b, governs: { command: calibrate } }
  - { name: c, governs: { command: reboot } }
`)
	wants(t, r, "capability.governs.undeclared", "capability.ungoverned")
}

// std: yoke:the-manifest.11
func TestTheFourAbsencesAreRefused(t *testing.T) {
	r, _ := manifest(t, installed, `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
endpoint: /run/acquire.sock
autostart: true
digest: "sha256:`+strings.Repeat("ab", 32)+`"
streams:
  - id: station.spectra
  - id: station.other
capabilities:
  - name: stream.spectra.publish
    governs: { stream: station.spectra }
  - name: stream.other.publish
    description: publishes the other thing
    governs: { stream: station.other }
`)
	removed := found(r, "manifest.field.removed")
	if len(removed) != 4 {
		t.Fatalf("the findings are %v, want four removed fields", codes(r))
	}
	want := map[string]string{
		"endpoint": "identity", "autostart": "composing document", "digest": "descriptor", "capabilities.1.description": "vocabulary",
	}
	for _, f := range removed {
		if where, there := want[f.Location]; !there || !strings.Contains(f.Message, where) {
			t.Errorf("%s: %q does not say where it lives now (%q)", f.Location, f.Message, where)
		}
	}
}
