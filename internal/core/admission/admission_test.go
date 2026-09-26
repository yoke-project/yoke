package admission_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/admission"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

const station = `
manifest: 1
id: com.yoke.station.acquire
protocol: 1
streams:
  - id: station.spectra
  - id: station.diagnostics
commands:
  - id: calibrate
queries:
  - id: head-status
capabilities:
  - { name: stream.spectra.publish, governs: { stream: station.spectra } }
  - { name: stream.diagnostics.publish, governs: { stream: station.diagnostics } }
  - { name: command.calibrate.accept, governs: { command: calibrate } }
  - { name: query.head-status.answer, governs: { query: head-status } }
`

const plugin = "com.yoke.station.acquire"

// declared is the surface the station's Manifest declares, as a request repeats it.
func declared() *pluginv1.Surface {
	return &pluginv1.Surface{
		Capabilities: []string{"stream.spectra.publish", "stream.diagnostics.publish", "command.calibrate.accept", "query.head-status.answer"},
		Streams:      []string{"station.spectra", "station.diagnostics"},
		Commands:     []string{"calibrate"},
		Queries:      []string{"head-status"},
	}
}

// bench is a deployment admission decides against: a Registry, the Manifests as read, the composed units.
type bench struct {
	reg    *registry.Registry
	tokens *admission.Tokens
	a      *admission.Admission
	log    *bytes.Buffer

	manifests map[string]*gate.Manifest
	units     map[string]admission.Composed

	mu   sync.Mutex
	told map[string][]string
}

func newBench(t *testing.T, waiver bool) *bench {
	t.Helper()
	reg, err := registry.Open(filepath.Join(t.TempDir(), registry.File))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reg.Close() })
	b := &bench{reg: reg, log: &bytes.Buffer{}, manifests: map[string]*gate.Manifest{}, units: map[string]admission.Composed{}, told: map[string][]string{}}
	b.manifest(t, station)
	b.tokens = admission.NewTokens(func(id string) time.Duration { return b.units[id].Policy.StartupWindow })
	b.a = admission.New(admission.Config{
		Registry:        reg,
		Manifest:        func(id string) (*gate.Manifest, bool) { m, ok := b.manifests[id]; return m, ok },
		Unit:            func(id string) (admission.Composed, bool) { u, ok := b.units[id]; return u, ok },
		Tokens:          b.tokens,
		AdmitUnlaunched: waiver,
		Log:             slog.New(slog.NewTextHandler(b.log, nil)),
		Observe: func(id string, in unit.Input) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.told[id] = append(b.told[id], fmt.Sprintf("%T", in))
		},
	})
	return b
}

// manifest reads a Manifest and declares its plugin.
func (b *bench) manifest(t *testing.T, document string) {
	t.Helper()
	_, m := gate.CheckManifest(gate.Document{Path: "manifests/" + strings.Split(strings.Split(document, "id: ")[1], "\n")[0] + ".yaml", Bytes: []byte(document)})
	if m == nil {
		t.Fatal("the test's Manifest was refused")
	}
	b.manifests[m.ID] = m
	if err := b.reg.Declare(registry.Declared{ID: m.ID, Protocol: m.Protocol, ManifestDigest: "sha256:00"}); err != nil {
		t.Fatal(err)
	}
}

// compose composes a unit of the plugin with the written defaults, and returns its token.
func (b *bench) compose(id, of string) string {
	b.units[id] = admission.Composed{Plugin: of, Policy: gate.Defaults()}
	return b.tokens.Issue(id)
}

func request(unitID, token string) *pluginv1.RegisterRequest {
	return &pluginv1.RegisterRequest{Plugin: plugin, Unit: unitID, Token: token, Protocol: 1,
		ArtifactVersion: "2.1.0", Language: "go", SdkLine: "yoke-sdk-go 1.4.2", Declared: declared()}
}

func (b *bench) register(t *testing.T, req *pluginv1.RegisterRequest) *pluginv1.RegisterResponse {
	t.Helper()
	resp, err := b.a.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("the registration failed on the wire: %v", err)
	}
	return resp
}

func refusedAt(t *testing.T, resp *pluginv1.RegisterResponse, stage pluginv1.Stage, code string) {
	t.Helper()
	if resp.Outcome != pluginv1.RegisterResponse_OUTCOME_REFUSED || resp.Stage != stage || resp.Code != code || resp.Message == "" {
		t.Errorf("the answer is %v at %v with %q (%q), want refused at %v with %s", resp.Outcome, resp.Stage, resp.Code, resp.Message, stage, code)
	}
}

func accepted(t *testing.T, resp *pluginv1.RegisterResponse) {
	t.Helper()
	if resp.Outcome != pluginv1.RegisterResponse_OUTCOME_ACCEPTED && resp.Outcome != pluginv1.RegisterResponse_OUTCOME_ACCEPTED_WITH_RESTRICTIONS {
		t.Errorf("the answer is %v at %v with %q (%q), want an acceptance", resp.Outcome, resp.Stage, resp.Code, resp.Message)
	}
}

// std: yoke:admission.01
func TestNineStagesInOrder(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(t *testing.T, b *bench, req *pluginv1.RegisterRequest)
		stage pluginv1.Stage
		code  string
	}{
		{"no unit", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) { req.Unit = "" }, pluginv1.Stage_STAGE_STRUCTURAL, "admission.structural.malformed"},
		{"unknown plugin", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) { req.Plugin = "com.example.nobody" }, pluginv1.Stage_STAGE_IDENTITY, "admission.identity.unknown_plugin"},
		{"unknown unit", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) { req.Unit = "stranger" }, pluginv1.Stage_STAGE_IDENTITY, "admission.identity.unknown_unit"},
		{"disabled", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) { b.reg.Disable(plugin, "davide") }, pluginv1.Stage_STAGE_ADMINISTRATIVE_STATE, "admission.state.disabled"},
		{"wrong token", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) { req.Token = "not-the-one" }, pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.invalid"},
		{"protocol", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) { req.Protocol = 7 }, pluginv1.Stage_STAGE_COMPATIBILITY, "admission.compat.unsupported"},
		{"divergent", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) {
			req.Declared.Streams = append(req.Declared.Streams, "station.extra")
		}, pluginv1.Stage_STAGE_DECLARATION_CONSISTENCY, "admission.consistency.divergent"},
		{"live", func(t *testing.T, b *bench, req *pluginv1.RegisterRequest) {
			// Admitted, and then a second process presenting a token issued for the same unit.
			accepted(t, b.register(t, request("acquire", req.Token)))
			req.Token = b.tokens.Issue("acquire")
		}, pluginv1.Stage_STAGE_UNIT_CONFLICT, "admission.conflict.unit_live"},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := newBench(t, false)
			req := request("acquire", b.compose("acquire", plugin))
			c.setup(t, b, req)
			refusedAt(t, b.register(t, req), c.stage, c.code)
		})
	}
}

// std: yoke:admission.02
func TestADisabledPluginIsRefusedWithoutItsCredentialExamined(t *testing.T) {
	b := newBench(t, false)
	token := b.compose("acquire", plugin)
	b.reg.Disable(plugin, "davide")
	refusedAt(t, b.register(t, request("acquire", token)), pluginv1.Stage_STAGE_ADMINISTRATIVE_STATE, "admission.state.disabled")
	b.reg.Enable(plugin, "davide")
	accepted(t, b.register(t, request("acquire", token)))
}

// std: yoke:admission.03
func TestATokenIsGoodOnceForItsUnitWithinItsWindow(t *testing.T) {
	b := newBench(t, false)
	b.manifest(t, strings.ReplaceAll(station, plugin, "com.yoke.station.other"))
	tokenA := b.compose("a", plugin)
	quick := b.units["a"]
	quick.Policy.StartupWindow = 200 * time.Millisecond
	b.units["a"] = quick
	tokenA = b.tokens.Issue("a")
	tokenB := b.compose("b", plugin)
	b.compose("c", "com.yoke.station.other")

	refusedAt(t, b.register(t, request("b", tokenA)), pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.invalid")
	other := request("b", tokenB)
	other.Plugin = "com.yoke.station.other"
	refusedAt(t, b.register(t, other), pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.invalid")
	accepted(t, b.register(t, request("b", tokenB)))
	refusedAt(t, b.register(t, request("b", tokenB)), pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.consumed")
	time.Sleep(300 * time.Millisecond)
	refusedAt(t, b.register(t, request("a", tokenA)), pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.expired")
	accepted(t, b.register(t, request("a", b.tokens.Issue("a"))))
}

// std: yoke:admission.04
func TestFromStageFiveARefusalHasSpentTheToken(t *testing.T) {
	b := newBench(t, false)
	token := b.compose("acquire", plugin)
	wrong := request("acquire", token)
	wrong.Protocol = 7
	refusedAt(t, b.register(t, wrong), pluginv1.Stage_STAGE_COMPATIBILITY, "admission.compat.unsupported")
	refusedAt(t, b.register(t, request("acquire", token)), pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.consumed")
}

// std: yoke:admission.05
func TestTheDeclaredSurfaceMatchesTheManifest(t *testing.T) {
	b := newBench(t, false)
	reordered := request("acquire", b.compose("acquire", plugin))
	slices.Reverse(reordered.Declared.Capabilities)
	slices.Reverse(reordered.Declared.Streams)
	accepted(t, b.register(t, reordered))

	b = newBench(t, false)
	missing := request("acquire", b.compose("acquire", plugin))
	missing.Declared.Capabilities = missing.Declared.Capabilities[1:]
	refusedAt(t, b.register(t, missing), pluginv1.Stage_STAGE_DECLARATION_CONSISTENCY, "admission.consistency.divergent")

	b = newBench(t, false)
	extra := request("acquire", b.compose("acquire", plugin))
	extra.Declared.Queries = append(extra.Declared.Queries, "uptime")
	refusedAt(t, b.register(t, extra), pluginv1.Stage_STAGE_DECLARATION_CONSISTENCY, "admission.consistency.divergent")
}

func surface(s *pluginv1.Surface) [4][]string {
	if s == nil {
		return [4][]string{}
	}
	out := [4][]string{s.Capabilities, s.Streams, s.Commands, s.Queries}
	for i := range out {
		out[i] = slices.Sorted(slices.Values(out[i]))
	}
	return out
}

// std: yoke:admission.06
func TestTheGrantIsWhatIsDeclaredAndAuthorised(t *testing.T) {
	b := newBench(t, false)
	b.reg.Grant(plugin, "stream.spectra.publish", "davide")
	b.reg.Grant(plugin, "stream.retired.publish", "davide")
	for _, c := range []string{"command.calibrate.accept", "query.head-status.answer"} {
		b.reg.Grant(plugin, c, "davide")
	}
	resp := b.register(t, request("acquire", b.compose("acquire", plugin)))
	if resp.Outcome != pluginv1.RegisterResponse_OUTCOME_ACCEPTED_WITH_RESTRICTIONS {
		t.Fatalf("the answer is %v, want accepted with restrictions", resp.Outcome)
	}
	wantGranted := [4][]string{{"command.calibrate.accept", "query.head-status.answer", "stream.spectra.publish"}, {"station.spectra"}, {"calibrate"}, {"head-status"}}
	wantWithheld := [4][]string{{"stream.diagnostics.publish"}, {"station.diagnostics"}, nil, nil}
	if got := surface(resp.Granted); !equalSurfaces(got, wantGranted) {
		t.Errorf("granted %v, want %v", got, wantGranted)
	}
	if got := surface(resp.Withheld); !equalSurfaces(got, wantWithheld) {
		t.Errorf("withheld %v, want %v", got, wantWithheld)
	}

	// The first incarnation ends, and the next admission reads the policy as it now is.
	b.a.Release("acquire")
	b.reg.Grant(plugin, "stream.diagnostics.publish", "davide")
	resp = b.register(t, request("acquire", b.tokens.Issue("acquire")))
	if all := surface(resp.Withheld); resp.Outcome != pluginv1.RegisterResponse_OUTCOME_ACCEPTED || len(slices.Concat(all[:]...)) != 0 {
		t.Errorf("with everything granted the answer is %v, withholding %v", resp.Outcome, resp.Withheld)
	}
}

func equalSurfaces(a, b [4][]string) bool {
	for i := range a {
		if len(a[i]) != len(b[i]) || (len(a[i]) > 0 && !slices.Equal(a[i], b[i])) {
			return false
		}
	}
	return true
}

// std: yoke:admission.07
func TestStageSevenNeverRefuses(t *testing.T) {
	b := newBench(t, false)
	resp := b.register(t, request("acquire", b.compose("acquire", plugin)))
	if resp.Outcome != pluginv1.RegisterResponse_OUTCOME_ACCEPTED_WITH_RESTRICTIONS {
		t.Fatalf("with nothing granted the answer is %v, want accepted with restrictions", resp.Outcome)
	}
	if got := surface(resp.Granted); len(slices.Concat(got[:]...)) != 0 {
		t.Errorf("granted %v, want nothing", got)
	}
	if got, want := surface(resp.Withheld), surface(declared()); !equalSurfaces(got, want) {
		t.Errorf("withheld %v, want everything declared %v", got, want)
	}
}

// std: yoke:admission.08
func TestOnePolicySeveralGrantsAndAConflictIsAboutAUnit(t *testing.T) {
	b := newBench(t, false)
	b.reg.Grant(plugin, "stream.spectra.publish", "davide")
	first := b.register(t, request("acquire-1", b.compose("acquire-1", plugin)))
	second := b.register(t, request("acquire-2", b.compose("acquire-2", plugin)))
	accepted(t, first)
	accepted(t, second)
	if !equalSurfaces(surface(first.Granted), surface(second.Granted)) {
		t.Errorf("the two grants differ: %v and %v", first.Granted, second.Granted)
	}
	if first.SessionId == second.SessionId || first.SessionId == "" {
		t.Errorf("the two Session identities are %q and %q", first.SessionId, second.SessionId)
	}
}

// std: yoke:admission.09
func TestAnAcceptanceCarriesASessionIdentityAndTheCoresTerms(t *testing.T) {
	b := newBench(t, false)
	token := b.compose("acquire", plugin)
	u := b.units["acquire"]
	u.Policy.HeartbeatInterval, u.Policy.HeartbeatTolerance = 4*time.Second, 5
	b.units["acquire"] = u
	resp := b.register(t, request("acquire", token))
	accepted(t, resp)
	raw, err := base64.RawURLEncoding.DecodeString(resp.SessionId)
	if len(resp.SessionId) != 43 || err != nil || len(raw) != 32 {
		t.Errorf("the Session identity %q is not 32 bytes of base64url", resp.SessionId)
	}
	if resp.Heartbeat.GetInterval().AsDuration() != 4*time.Second || resp.Heartbeat.GetTolerance() != 5 {
		t.Errorf("the heartbeat terms are %v", resp.Heartbeat)
	}
}

// std: yoke:admission.10
func TestWhatAdmissionRecords(t *testing.T) {
	b := newBench(t, false)
	good := b.compose("acquire-1", plugin)
	other := b.compose("acquire-2", plugin)
	accepted(t, b.register(t, request("acquire-1", good)))
	refusedAt(t, b.register(t, request("acquire-2", "a-token-nobody-issued")), pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.invalid")
	p, _, _ := b.reg.Plugin(plugin)
	if p.Version != "2.1.0" || p.Language != "go" || p.SDK != "yoke-sdk-go 1.4.2" {
		t.Errorf("the Registry recorded %+v", p)
	}
	logged := b.log.String()
	var acceptance, refusal string
	for _, line := range strings.Split(logged, "\n") {
		switch {
		case strings.Contains(line, "unit=acquire-1"):
			acceptance = line
		case strings.Contains(line, "unit=acquire-2"):
			refusal = line
		}
	}
	if !strings.Contains(acceptance, "withheld=8") {
		t.Errorf("the acceptance's line %q does not say how many items were withheld", acceptance)
	}
	if !strings.Contains(refusal, "credential=examined") {
		t.Errorf("the refusal's line %q does not mark that it reached the credential", refusal)
	}
	for _, token := range []string{good, other, "a-token-nobody-issued"} {
		if strings.Contains(logged, token) {
			t.Errorf("a token is in the log:\n%s", logged)
		}
	}
}

// std: yoke:admission.11
func TestTheDevelopmentWaiverRelaxesAuthenticationAndNothingElse(t *testing.T) {
	b := newBench(t, true)
	b.units["acquire"] = admission.Composed{Plugin: plugin, Policy: gate.Defaults()}
	accepted(t, b.register(t, request("acquire", "")))
	// A hand-started process is live until its life ends, like any other.
	refusedAt(t, b.register(t, request("stranger", "")), pluginv1.Stage_STAGE_IDENTITY, "admission.identity.unknown_unit")
	divergent := request("acquire", "")
	divergent.Declared.Commands = nil
	b.a.Release("acquire")
	refusedAt(t, b.register(t, divergent), pluginv1.Stage_STAGE_DECLARATION_CONSISTENCY, "admission.consistency.divergent")
	b.reg.Disable(plugin, "davide")
	refusedAt(t, b.register(t, request("acquire", "")), pluginv1.Stage_STAGE_ADMINISTRATIVE_STATE, "admission.state.disabled")

	strict := newBench(t, false)
	strict.units["acquire"] = admission.Composed{Plugin: plugin, Policy: gate.Defaults()}
	refusedAt(t, strict.register(t, request("acquire", "")), pluginv1.Stage_STAGE_AUTHENTICATION, "admission.auth.invalid")
}

// std: yoke:admission.12
func TestTheOutcomeReachesTheMachineWhenItIsTheUnits(t *testing.T) {
	b := newBench(t, false)
	first := b.compose("first", plugin)
	second := b.compose("second", plugin)
	b.compose("third", plugin)
	accepted(t, b.register(t, request("first", first)))
	wrong := request("second", second)
	wrong.Protocol = 7
	b.register(t, wrong)
	b.register(t, request("third", "forged"))
	b.mu.Lock()
	defer b.mu.Unlock()
	want := map[string][]string{"first": {"unit.AdmissionAccepted"}, "second": {"unit.AdmissionDeclined"}}
	for id, inputs := range want {
		if !slices.Equal(b.told[id], inputs) {
			t.Errorf("%s's machine was told %v, want %v", id, b.told[id], inputs)
		}
	}
	if len(b.told["third"]) != 0 {
		t.Errorf("a forged request told the third unit's machine %v", b.told["third"])
	}
}
