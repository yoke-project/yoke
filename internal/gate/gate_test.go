package gate_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/gate"
)

func check(t *testing.T, kind gate.Kind, document string) (gate.Report, *gate.Deployment) {
	t.Helper()
	return gate.Check(gate.Input{Document: gate.Document{Path: "deployment.yaml", Kind: kind, Bytes: []byte(document)}})
}

func composition(t *testing.T, document string) (gate.Report, *gate.Deployment) {
	t.Helper()
	return check(t, gate.Composition, document)
}

func codes(r gate.Report) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.Code)
	}
	return out
}

// found returns the findings with the given code.
func found(r gate.Report, code string) []gate.Finding {
	var out []gate.Finding
	for _, f := range r.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

// wants fails unless the report holds exactly these codes, in any order.
func wants(t *testing.T, r gate.Report, want ...string) {
	t.Helper()
	got := codes(r)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("the findings are %v, want %v", got, want)
	}
}

// std: yoke:the-gate.01
func TestAPassReportsEverythingItFound(t *testing.T) {
	r, _ := composition(t, `
extra: 1
units:
  a:
    plugin: com.example.a
  b:
    kind: oneshot
    exec: /usr/bin/b
    env: { YOKE_UNIT: b }
channels:
  c: { transport: local, clients: many }
policy:
  startup_window: 30
`)
	wants(t, r, "key.unknown", "field.required", "field.value", "format.duration", "unit.env.reserved")
	if !r.Refused() {
		t.Fatal("a report with refusals is not refused")
	}
}

// std: yoke:the-gate.02
func TestAFindingCarriesCodeClassLocationAndMessage(t *testing.T) {
	r, _ := gate.Check(gate.Input{Document: gate.Document{Path: "/etc/yoke/deployments/bench.yaml", Kind: gate.Composition, Bytes: []byte(`
units:
  acquire:
    kind: plugin
    plugin: com.yoke.station.acquire
    autostart: sometimes
`)}})
	if len(r.Findings) != 1 {
		t.Fatalf("the findings are %+v, want one", r.Findings)
	}
	f := r.Findings[0]
	if f.Code != "field.type" || f.Class != gate.Refusal || f.Document != "/etc/yoke/deployments/bench.yaml" || f.Location != "units.acquire.autostart" || !strings.Contains(f.Message, "sometimes") {
		t.Fatalf("the finding is %+v", f)
	}
}

// std: yoke:the-gate.03
func TestAPhaseWhoseInputsAreUntrustworthyDoesNotRun(t *testing.T) {
	r, _ := composition(t, "units: [unclosed")
	wants(t, r, "document.malformed")
	if !slices.Equal(r.NotRun, []string{gate.PhaseShape, gate.PhaseInternalJoins, gate.PhaseCrossDocument, gate.PhaseHostFacts, gate.PhaseWeaker}) {
		t.Errorf("after a malformed document the phases not run are %v", r.NotRun)
	}

	r, _ = composition(t, `
units:
  a:
    kind: oneshot
    exec: /usr/bin/a
    depends_on: [ nobody ]
    colour: red
`)
	wants(t, r, "key.unknown")
	if !slices.Equal(r.NotRun, []string{gate.PhaseInternalJoins, gate.PhaseCrossDocument, gate.PhaseHostFacts, gate.PhaseWeaker}) {
		t.Errorf("after a refusal of shape the phases not run are %v", r.NotRun)
	}
}

// std: yoke:the-gate.04
func TestADocumentWithNoRefusalPasses(t *testing.T) {
	r, d := composition(t, `
units:
  panel:
    kind: interface
    exec: /usr/lib/yoke/interfaces/panel
channels:
  remote:
    transport: http+ws
    clients: single
    address: { class: loopback, port: 8080 }
`)
	if r.Refused() || d == nil {
		t.Fatalf("a document with only a weaker arrangement was refused: %+v", r.Findings)
	}
	wants(t, r, "channel.address.loopback")
	if f := r.Findings[0]; f.Class != gate.Weaker || f.Location != "channels.remote.address" {
		t.Errorf("the weaker finding is %+v", f)
	}
}

// std: yoke:the-gate.05
func TestADocumentAbsentOrNotYAMLIsRefusedAtReading(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nothing.yaml")
	r, _ := gate.Check(gate.Input{Document: gate.Read(missing, gate.Composition)})
	wants(t, r, "document.unreadable")
	if r.Findings[0].Document != missing {
		t.Errorf("the finding names %q", r.Findings[0].Document)
	}

	broken := filepath.Join(t.TempDir(), "broken.yaml")
	os.WriteFile(broken, []byte("units: [unclosed"), 0o644)
	r, _ = gate.Check(gate.Input{Document: gate.Read(broken, gate.Composition)})
	wants(t, r, "document.malformed")
	if r.Findings[0].Document != broken {
		t.Errorf("the finding names %q", r.Findings[0].Document)
	}
}

// std: yoke:the-gate.06
func TestAnUnknownKeyIsRefusedAtAnyLevel(t *testing.T) {
	r, _ := composition(t, `
colour: red
units:
  a:
    kind: oneshot
    exec: /usr/bin/a
    colour: red
    policy: { colour: red }
channels:
  c:
    transport: http+ws
    clients: single
    colour: red
    address: { class: loopback, port: 1, colour: red }
arbitration:
  - { prevails: c, over: [ d ], colour: red }
  - { prevails: d, over: [ c ] }
`)
	var locations []string
	for _, f := range found(r, "key.unknown") {
		locations = append(locations, f.Location)
	}
	slices.Sort(locations)
	want := []string{"arbitration.0.colour", "channels.c.address.colour", "channels.c.colour", "colour", "units.a.colour", "units.a.policy.colour"}
	if !slices.Equal(locations, want) {
		t.Errorf("unknown keys refused at %v, want %v", locations, want)
	}
}

// std: yoke:the-gate.07
func TestEveryFieldHasItsTypeAndARequiredOneItsPresence(t *testing.T) {
	for document, want := range map[string][]string{
		"units: { a: { exec: /usr/bin/a } }":                                             {"field.required"},
		"units: { a: { kind: plugin } }":                                                 {"field.required"},
		"units: {}\nchannels: { c: {} }":                                                 {"field.required", "field.required"},
		"units: {}\nchannels: { c: { transport: local, clients: single, address: {} } }": {"field.required"},
		"units: {}\nchannels: { c: { transport: http+ws, clients: single, address: { class: routable, port: 1, security: { transport: encrypted, caller: authenticated } } } }": {"field.required"},
		"units: {}\nchannels: { c: { transport: http+ws, clients: single, address: { class: loopback } } }":                                                                     {"field.required"},
		"units: {}\nchannels: { c: { transport: local, clients: single } }\narbitration: [ {} ]":                                                                                {"field.required", "field.required"},
		"units: { a: { kind: oneshot, exec: /usr/bin/a, args: --verbose } }":                                                                                                    {"field.type"},
		"units: { a: { kind: oneshot, exec: /usr/bin/a, depends_on: { b: c } } }":                                                                                               {"field.type"},
		"units: { a: { kind: service, exec: /usr/bin/a } }":                                                                                                                     {"field.value"},
		"units: {}\nchannels: { c: { transport: local, clients: many } }":                                                                                                       {"field.value"},
		"units: {}\nchannels: { c: { transport: http+ws, clients: single, address: { class: public } } }":                                                                       {"field.value"},
	} {
		r, _ := composition(t, document)
		got := codes(r)
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s\n  gives %v, want %v", document, got, want)
		}
	}
}

// std: yoke:the-gate.08
func TestEveryScalarIsWrittenInItsForm(t *testing.T) {
	for document, want := range map[string]string{
		"units: {}\npolicy: { startup_window: 30 }":            "format.duration",
		"units: {}\npolicy: { retention: { bytes: 50mb } }":    "format.size",
		"units: {}\npolicy: { heartbeat: { tolerance: 0.5 } }": "field.value",
	} {
		r, _ := composition(t, document)
		if got := codes(r); !slices.Equal(got, []string{want}) {
			t.Errorf("%s\n  gives %v, want [%s]", document, got, want)
		}
	}
	// A composition document carries no digest at all, so the format is shown on a descriptor.
	image := "ghcr.io/example/a@sha256:" + strings.Repeat("ab", 32)
	r, _ := check(t, gate.Descriptor, "units: { a: { kind: plugin, plugin: com.example.a, image: \""+image+"\", manifest_digest: \"md5:ab\" } }")
	if got := codes(r); !slices.Equal(got, []string{"format.digest"}) {
		t.Errorf("a manifest_digest of md5 gives %v, want [format.digest]", got)
	}
	r, d := composition(t, "units: { a: { kind: oneshot, exec: /usr/bin/a, args: [no, 1.0, 01] } }")
	if r.Refused() {
		t.Fatalf("arguments written plainly were refused: %v", codes(r))
	}
	if got := d.Units["a"].Args; !slices.Equal(got, []string{"no", "1.0", "01"}) {
		t.Errorf("the arguments were read as %q", got)
	}
}

// std: yoke:the-gate.09
func TestANameIsValidatedAndNeverTransformed(t *testing.T) {
	r, _ := composition(t, "units: { head/a: { kind: oneshot, exec: /usr/bin/a } }\nchannels: { \"pa\\0nel\": { transport: local, clients: single } }")
	if n := len(found(r, "name.not_a_path_component")); n != 2 {
		t.Errorf("%d names refused, want 2: %v", n, codes(r))
	}
	r, d := composition(t, "units: { station.acquire-1: { kind: oneshot, exec: /usr/bin/a } }")
	if r.Refused() {
		t.Fatalf("a dotted name was refused: %v", codes(r))
	}
	if _, there := d.Units["station.acquire-1"]; !there {
		t.Errorf("the unit reached the deployment as %v", d.Units)
	}
}

// std: yoke:the-gate.10
func TestWhatAUnitsKindPermitsItToCarry(t *testing.T) {
	// A composition document carries no migrates_from at all, so the kind's rule is shown on a descriptor.
	digest := "sha256:" + strings.Repeat("ab", 32)
	r, _ := check(t, gate.Descriptor, "units: { a: { kind: interface, exec: bin/a, digest: \""+digest+"\", migrates_from: \"<2.0\" } }")
	if got := codes(r); !slices.Equal(got, []string{"unit.migrates_from.kind"}) {
		t.Errorf("migrates_from on an interface gives %v, want [unit.migrates_from.kind]", got)
	}
	for document, want := range map[string]string{
		"units: { a: { kind: plugin, plugin: com.example.a, needs: [ display ] } }":    "unit.needs.on_plugin",
		"units: { a: { kind: oneshot, exec: /usr/bin/a, env: { YOKE_TOKEN: mine } } }": "unit.env.reserved",
		"units: { a: { kind: interface, image: \"ghcr.io/yoke/panel:latest\" } }":      "unit.image.tagged",
	} {
		r, _ := composition(t, document)
		if got := codes(r); !slices.Equal(got, []string{want}) {
			t.Errorf("%s\n  gives %v, want [%s]", document, got, want)
		}
	}
}

// std: yoke:the-gate.11
func TestHowTheProgramIsNamedDiffersBetweenTheDocuments(t *testing.T) {
	digest := "sha256:" + strings.Repeat("ab", 32)
	image := "ghcr.io/yoke/a@" + digest
	r, _ := check(t, gate.Descriptor, `
units:
  both: { kind: oneshot, exec: bin/a, digest: "`+digest+`", image: "`+image+`" }
  neither: { kind: oneshot }
  undigested: { kind: oneshot, exec: bin/a }
`)
	wants(t, r, "unit.program.ambiguous", "unit.program.ambiguous", "unit.exec.no_digest")

	r, _ = composition(t, `
units:
  step: { kind: oneshot }
  acquire: { kind: plugin, plugin: com.yoke.station.acquire }
`)
	wants(t, r, "unit.program.ambiguous")
	if f := r.Findings[0]; f.Location != "units.step" {
		t.Errorf("the refusal is at %q", f.Location)
	}
}

// std: yoke:the-gate.12
func TestTheNeedsAreAClosedSetAndASecretIsNeverBound(t *testing.T) {
	r, _ := composition(t, `
units:
  a: { kind: interface, exec: /usr/bin/a, needs: [ serial ] }
  b: { kind: interface, exec: /usr/bin/b, needs: [ "display:main" ] }
  c: { kind: interface, exec: /usr/bin/c, needs: [ storage ] }
  d: { kind: interface, exec: /usr/bin/d, needs: [ "secret:token" ], bind: { token: /etc/token } }
`)
	wants(t, r, "field.value", "field.value", "field.value", "bind.secret")

	r, _ = composition(t, `
units:
  e:
    kind: interface
    exec: /usr/bin/e
    needs: [ "device:head-a", display, "storage:datasets", "secret:token" ]
    bind: { head-a: /dev/spectro-head-a }
`)
	if r.Refused() {
		t.Fatalf("needs from the set were refused: %v", codes(r))
	}
}

// std: yoke:the-gate.13
func TestSubstitutionIsPermittedInExactlyFourPlaces(t *testing.T) {
	r, _ := composition(t, `
units:
  ok:
    kind: interface
    exec: /usr/bin/ok
    needs: [ "device:head-a" ]
    bind: { head-a: "${bind.head-a}" }
    args: [ "${bind.head-a}" ]
    env: { DEVICE: "${bind.head-a}" }
  exec: { kind: oneshot, exec: "/usr/bin/${bind.x}" }
  image: { kind: oneshot, image: "${bind.x}" }
  plugin: { kind: plugin, plugin: "${bind.x}" }
  needs: { kind: oneshot, exec: /usr/bin/n, needs: [ "${bind.x}" ] }
  depends: { kind: oneshot, exec: /usr/bin/d, depends_on: [ "${bind.x}" ] }
channels:
  remote: { transport: http+ws, clients: single, address: { class: loopback, port: "${bind.head-a}" } }
`)
	var locations []string
	for _, f := range found(r, "substitution.place") {
		locations = append(locations, f.Location)
	}
	slices.Sort(locations)
	want := []string{"units.depends.depends_on.0", "units.exec.exec", "units.image.image", "units.needs.needs.0", "units.plugin.plugin"}
	if !slices.Equal(locations, want) {
		t.Errorf("substitution refused at %v, want %v", locations, want)
	}
}

// std: yoke:the-gate.14
func TestAReferenceResolvedInNeitherScopeIsRefused(t *testing.T) {
	r, _ := composition(t, `
units:
  panel:
    kind: interface
    exec: /usr/bin/panel
    needs: [ "device:head-a", "secret:token" ]
    bind: { head-a: /dev/spectro-head-a }
    args: [ "${bind.head-a}", "${bind.token}", "${bind.missing}", "${site_label}" ]
`)
	unresolved := found(r, "substitution.unresolved")
	if len(unresolved) != 2 || len(r.Findings) != 2 {
		t.Fatalf("the findings are %v, want two unresolved references", codes(r))
	}
	for i, reference := range []string{"${bind.missing}", "${site_label}"} {
		if m := unresolved[i].Message; !strings.Contains(m, reference) || !strings.Contains(m, "panel") {
			t.Errorf("the refusal %q does not name %s and panel", m, reference)
		}
	}
}

// std: yoke:the-gate.15
func TestTheInternalJoins(t *testing.T) {
	r, _ := composition(t, `
units:
  a: { kind: oneshot, exec: /usr/bin/a, depends_on: [ ghost ] }
  b: { kind: oneshot, exec: /usr/bin/b, depends_on: [ panel ] }
  panel: { kind: interface, exec: /usr/bin/panel }
channels:
  one: { unit: nobody, transport: local, clients: single }
  two: { unit: a, transport: local, clients: single }
  three: { transport: grpc, clients: single }
  four:
    transport: http+ws
    clients: single
    address: { class: routable, host: 0.0.0.0, port: 8443, security: { transport: encrypted } }
arbitration:
  - { prevails: one, over: [ missing ] }
`)
	wants(t, r, "depends_on.unknown", "depends_on.interface", "channel.unit.unknown", "channel.unit.kind",
		"channel.transport.unknown", "arbitration.channel.unknown", "channel.address.security")
}

// std: yoke:the-gate.16
func TestPolicyWrittenDefaultsTwoScopesAQuantityAndNeverARule(t *testing.T) {
	r, d := composition(t, `
policy:
  startup_window: 60s
  retention: { entries: 0 }
units:
  slow: { kind: plugin, plugin: com.example.slow, policy: { startup_window: 90s } }
  plain: { kind: plugin, plugin: com.example.plain }
  step: { kind: oneshot, exec: /usr/bin/step, policy: { restart: { on_failure: true } } }
`)
	if r.Refused() {
		t.Fatalf("the policy was refused: %v", codes(r))
	}
	want := gate.Policy{
		HeartbeatInterval: 10 * time.Second, HeartbeatTolerance: 3,
		StartupWindow: 60 * time.Second, StopWindow: 10 * time.Second,
		Backoff: 5 * time.Second, Ceiling: 5 * time.Minute, StabilityWindow: 60 * time.Second,
		RetentionAge: 7 * 24 * time.Hour, RetentionBytes: 50_000_000, RetentionEntries: 0,
	}
	if got := d.Units["plain"].Policy; got != want {
		t.Errorf("plain's policy is %+v, want %+v", got, want)
	}
	want.StartupWindow = 90 * time.Second
	if got := d.Units["slow"].Policy; got != want {
		t.Errorf("slow's policy is %+v, want %+v", got, want)
	}
	if !d.Units["step"].OnFailure || d.Units["plain"].OnFailure {
		t.Error("restart on failure reached the wrong unit")
	}

	for _, document := range []string{
		"units: {}\npolicy: { restart: { on_failure: true } }",
		"units: { a: { kind: plugin, plugin: com.example.a, policy: { restart: { on_failure: true } } } }",
		"units: {}\npolicy: { restart: { attempts: 5 } }",
	} {
		r, _ := composition(t, document)
		if got := codes(r); !slices.Equal(got, []string{"key.unknown"}) {
			t.Errorf("%s\n  gives %v, want [key.unknown]", document, got)
		}
	}
}
