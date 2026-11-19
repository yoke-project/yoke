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
			{ID: "station.spectra", MaxFrame: gate.MaxFrameDefault},
			{ID: "station.preview", ToleratesLoss: true, ToleratesReorder: true, MaxFrame: gate.MaxFrameDefault},
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
	want := []gate.Stream{{ID: "plain", MaxFrame: gate.MaxFrameDefault}, {ID: "shuffled", ToleratesReorder: true, MaxFrame: gate.MaxFrameDefault}}
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

// std: yoke:the-manifest.12
func TestAStreamDeclaresItsLargestFrame(t *testing.T) {
	r, m := manifest(t, installed, `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
streams:
  - id: s.none
  - id: s.four
    max_frame: 4194304
  - id: s.ceiling
    max_frame: 67108864
capabilities:
  - { name: a, governs: { stream: s.none } }
  - { name: b, governs: { stream: s.four } }
  - { name: c, governs: { stream: s.ceiling } }
`)
	if m == nil {
		t.Fatalf("no Manifest read: %v", codes(r))
	}
	sizes := map[string]int64{}
	for _, s := range m.Streams {
		sizes[s.ID] = s.MaxFrame
	}
	for id, want := range map[string]int64{"s.none": 1 << 20, "s.four": 4 << 20, "s.ceiling": 64 << 20} {
		if sizes[id] != want {
			t.Errorf("%s's largest frame is %d, want %d", id, sizes[id], want)
		}
	}
	r, _ = manifest(t, installed, `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
streams:
  - id: s.beyond
    max_frame: 67108865
  - id: s.zero
    max_frame: 0
  - id: s.word
    max_frame: big
`)
	refused := map[string]string{}
	for _, f := range r.Findings {
		refused[f.Location] = f.Code
	}
	for at, code := range map[string]string{"streams.0.max_frame": "stream.max_frame", "streams.1.max_frame": "stream.max_frame", "streams.2.max_frame": "field.type"} {
		if refused[at] != code {
			t.Errorf("%s: found %q, want %s (%v)", at, refused[at], code, codes(r))
		}
	}
}

const accountsUser = `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
needs: [ "unit:accounts", "secret:refresh" ]
uses:
  accounts:
    questions: [ oauth.token ]
    streams: [ oauth.status ]
rewrites: [ refresh ]
capabilities:
  - name: query.oauth.token.ask
    governs: { use: { need: accounts, query: oauth.token } }
  - name: stream.oauth.status.follow
    governs: { use: { need: accounts, stream: oauth.status } }
`

// std: yoke:the-manifest.13
func TestAPluginSaysWhichUnitItNeedsWhatItUsesAndWhatItRewrites(t *testing.T) {
	r, m := manifest(t, installed, accountsUser)
	if len(r.Findings) != 0 || m == nil {
		t.Fatalf("the Manifest gave %v", r.Findings)
	}
	if want := (map[string]gate.Use{"accounts": {Questions: []string{"oauth.token"}, Streams: []string{"oauth.status"}}}); !reflect.DeepEqual(m.Uses, want) {
		t.Errorf("uses: got %v, want %v", m.Uses, want)
	}
	if !slices.Equal(m.Rewrites, []string{"refresh"}) {
		t.Errorf("rewrites: got %v", m.Rewrites)
	}
	for _, c := range []struct{ from, to, code string }{
		{"    streams: [ oauth.status ]\n", "    streams: [ oauth.status ]\n    commands: [ x ]\n", "uses.key"},
		{"uses:\n  accounts:\n", "uses:\n  other:\n    questions: [ x ]\n  accounts:\n", "uses.need.undeclared"},
		{"uses:\n  accounts:\n    questions: [ oauth.token ]\n    streams: [ oauth.status ]\n", "uses: {}\n", "uses.missing"},
		{"    questions: [ oauth.token ]\n", "    questions: [ oauth.token, oauth.accounts ]\n", "uses.ungoverned"},
		{"{ need: accounts, query: oauth.token }", "{ need: accounts, query: oauth.other }", "capability.use.undeclared"},
		{"rewrites: [ refresh ]", "rewrites: [ refresh, other ]", "rewrites.undeclared"},
	} {
		document := strings.Replace(accountsUser, c.from, c.to, 1)
		r, _ := manifest(t, installed, document)
		got := codes(r)
		if c.code == "uses.missing" || c.code == "capability.use.undeclared" {
			// A use left without its entry also leaves its capabilities governing nothing listed.
			if !slices.Contains(got, c.code) {
				t.Errorf("%s: gives %v, want %s among them", c.code, got, c.code)
			}
			continue
		}
		if !slices.Equal(got, []string{c.code}) {
			t.Errorf("%s: gives %v (%v)", c.code, got, r.Findings)
		}
	}
}
