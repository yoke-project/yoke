package admin_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/admin"
	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

const station = "com.example.station"

type fakeUnit struct {
	plugin      string
	state       unit.State
	incarnation int
	since       time.Time
	condition   *unit.Condition
}

// fakeUnits is a supervisor that does at once what it is asked.
type fakeUnits struct {
	mu          sync.Mutex
	units       map[string]*fakeUnit
	unreachable bool
}

func (f *fakeUnits) Plugin(id string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.units[id]
	if !ok {
		return "", false
	}
	return u.plugin, true
}

func (f *fakeUnits) Of(plugin string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id, u := range f.units {
		if u.plugin == plugin {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func (f *fakeUnits) IDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id := range f.units {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (f *fakeUnits) Kind(id string) unit.Kind {
	if plugin, ok := f.Plugin(id); ok && plugin != "" {
		return unit.Plugin
	} else if ok {
		return unit.Oneshot
	}
	return ""
}

func (f *fakeUnits) Status(id string) supervisor.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	if u, ok := f.units[id]; ok {
		st := supervisor.Status{State: u.state, Incarnation: u.incarnation, Since: u.since}
		if u.condition != nil {
			st.Condition, st.HasCondition, st.ConditionSince = *u.condition, true, u.since
		}
		return st
	}
	return supervisor.Status{}
}

func (f *fakeUnits) act(id string, do func(u *fakeUnit)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unreachable {
		return fmt.Errorf("the unit %s: %w", id, admin.ErrUnreachable)
	}
	u, ok := f.units[id]
	if !ok {
		return fmt.Errorf("no unit %s", id)
	}
	do(u)
	return nil
}

func (f *fakeUnits) StartUnit(id string) error {
	return f.act(id, func(u *fakeUnit) {
		if u.state.Terminal() {
			u.incarnation++
			u.state = unit.Running
		}
	})
}

func (f *fakeUnits) StopUnit(id string) error {
	return f.act(id, func(u *fakeUnit) { u.state = unit.Stopped })
}

func (f *fakeUnits) RestartUnit(id string) error {
	return f.act(id, func(u *fakeUnit) { u.incarnation++; u.state = unit.Running })
}

// fakeSessions are Sessions that answer by the unit: reverse answers the bytes it was asked, reversed,
// and silent never answers.
type fakeSessions struct {
	mu      sync.Mutex
	open    map[string]string // unit → how it answers
	revoked map[string]pluginv1.SessionMessage_Revoked_Cause
	// instructed is every control instruction a unit was handed, and present whether the stream's socket
	// existed when it was.
	instructed []instruction
}

type instruction struct {
	unit    string
	control *pluginv1.Control
	present bool
}

func (f *fakeSessions) Open(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.open[id]
	return ok
}

func (f *fakeSessions) Ask(ctx context.Context, id string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error) {
	f.mu.Lock()
	how, ok := f.open[id]
	f.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no Session")
	}
	if how == "silent" {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	answer := slices.Clone(q.GetPayload())
	slices.Reverse(answer)
	return &pluginv1.Query_Answer{Payload: answer}, nil
}

func (f *fakeSessions) Revoke(id string, cause pluginv1.SessionMessage_Revoked_Cause, line string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.open[id]; !ok {
		return fmt.Errorf("no Session")
	}
	delete(f.open, id)
	f.revoked[id] = cause
	return nil
}

// bench is a Core with a real Registry and log store, the plugin station declared with one capability,
// and a fake supervisor and Sessions.
type bench struct {
	core     *admin.Core
	units    *fakeUnits
	sessions *fakeSessions
	events   *published
	operator administrativev1.OperatorClient
	shell    administrativev1.ShellClient
	dir      string
	bus      *bus.Bus
}

func newBench(t *testing.T, units map[string]*fakeUnit, sessions map[string]string) *bench {
	t.Helper()
	dir := t.TempDir()
	r, err := registry.Open(filepath.Join(dir, registry.File))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	if err := r.Declare(registry.Declared{ID: station, Protocol: 1, ManifestDigest: "sha256:00"}); err != nil {
		t.Fatal(err)
	}
	logs, err := logstore.Open(filepath.Join(dir, logstore.File), func(err error) { t.Error(err) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logs.Close() })
	b := &bench{units: &fakeUnits{units: units}, sessions: &fakeSessions{open: sessions, revoked: map[string]pluginv1.SessionMessage_Revoked_Cause{}}, events: &published{}}
	manifest := &gate.Manifest{ID: station, Capabilities: []gate.Capability{{Name: "stream.data.publish"}}}
	b.core = &admin.Core{
		Registry: r, Logs: logs, Units: b.units, Sessions: b.sessions, Publish: b.events.publish,
		Manifest: func(id string) (*gate.Manifest, bool) { return manifest, id == station },
	}
	b.bus = bus.New()
	b.core.Bus = b.bus
	var s *admin.Surface
	s, b.operator, b.shell = served(t, admin.Config{Operations: b.core.Operations()})
	b.core.Connections = s.Connections
	b.dir = dir
	return b
}

// call issues r by Call, and returns what it was answered.
func (b *bench) call(t *testing.T, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := b.operator.Call(ctx, r)
	if err != nil {
		if ref := refusalOf(err); ref != nil {
			return nil, ref
		}
		t.Fatalf("the call failed with no refusal: %v", err)
	}
	return resp, nil
}

// changed is the answer a change carries, whichever change it was.
func changed(resp *administrativev1.Response) *administrativev1.Changed {
	m := resp.ProtoReflect()
	f := m.WhichOneof(m.Descriptor().Oneofs().ByName("answer"))
	if f == nil {
		return nil
	}
	c, _ := m.Get(f).Message().Interface().(*administrativev1.Changed)
	return c
}

// entries waits for the log store to hold entries that satisfy holds, and returns them.
func entries(t *testing.T, logs *logstore.Store, holds func([]logstore.Entry) bool) []logstore.Entry {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		all, err := logs.Entries(0)
		if err != nil {
			t.Fatal(err)
		}
		if holds(all) || time.Now().After(deadline) {
			return all
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func v1(r *administrativev1.Request) *administrativev1.Request { r.Version = 1; return r }

func disable(plugin string) *administrativev1.Request {
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_PluginDisable{PluginDisable: &administrativev1.PluginPolicy{Plugin: plugin}}})
}

func enable(plugin string) *administrativev1.Request {
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_PluginEnable{PluginEnable: &administrativev1.PluginPolicy{Plugin: plugin}}})
}

func grant(plugin, capability string) *administrativev1.Request {
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: plugin, Capability: capability}}})
}

func unitAct(op string, id string) *administrativev1.Request {
	act := &administrativev1.UnitAct{Unit: id}
	r := &administrativev1.Request{}
	switch op {
	case "start":
		r.Operation = &administrativev1.Request_UnitStart{UnitStart: act}
	case "stop":
		r.Operation = &administrativev1.Request_UnitStop{UnitStop: act}
	case "restart":
		r.Operation = &administrativev1.Request_UnitRestart{UnitRestart: act}
	case "retention.clear":
		r.Operation = &administrativev1.Request_UnitRetentionClear{UnitRetentionClear: act}
	}
	return v1(r)
}

func ask(id string, question []byte) *administrativev1.Request {
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_UnitAsk{UnitAsk: &administrativev1.UnitAsk{Unit: id, Type: "status", Question: question}}})
}

func retain(id string, age time.Duration, entries *uint64) *administrativev1.Request {
	r := &administrativev1.UnitRetention{Unit: id, Entries: entries}
	if age > 0 {
		r.Age = durationpb.New(age)
	}
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_UnitRetentionSet{UnitRetentionSet: r}})
}

func count(n uint64) *uint64 { return &n }

func running(plugin string, incarnation int) *fakeUnit {
	return &fakeUnit{plugin: plugin, state: unit.Running, incarnation: incarnation}
}

// std: yoke:the-operations.01
func TestAVersionThisCoreDoesNotSpeakIsRefused(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{}, map[string]string{})
	for _, version := range []uint32{0, 2} {
		r := enable(station)
		r.Version = version
		if _, ref := b.call(t, r); ref.GetCode() != "compat.unsupported" {
			t.Errorf("the version %d was answered %v, want compat.unsupported", version, ref)
		}
	}
}

// std: yoke:the-operations.02
func TestDisablingAPluginRevokesEachLiveSession(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 3), "archive": running(station, 5)},
		map[string]string{"acquire": "reverse", "archive": "reverse"})
	resp, ref := b.call(t, disable(station))
	if ref != nil {
		t.Fatal(ref)
	}
	c := changed(resp)
	if !c.GetPreviously().GetEnabled() || c.GetEffective() != administrativev1.Changed_EFFECTIVE_IMMEDIATELY {
		t.Errorf("the disable answered %v", c)
	}
	var got []string
	for _, q := range c.GetConsequences() {
		got = append(got, fmt.Sprintf("%s#%d", q.GetUnit(), q.GetIncarnation()))
		if !strings.Contains(q.GetWhat(), "revoked") {
			t.Errorf("the consequence for %s says %q, want that its Session was revoked", q.GetUnit(), q.GetWhat())
		}
	}
	if strings.Join(got, ",") != "acquire#3,archive#5" {
		t.Errorf("the consequences are %v, want acquire#3 and archive#5", got)
	}
	for _, id := range []string{"acquire", "archive"} {
		if cause, ok := b.sessions.revoked[id]; !ok || cause != pluginv1.SessionMessage_Revoked_CAUSE_PLUGIN_DISABLED {
			t.Errorf("%s's Session was revoked %v (%v), want as the plugin being disabled", id, ok, cause)
		}
	}
	history, _ := b.core.Registry.History(station)
	who := me(t).Username
	if len(history) != 1 || history[0].Action != "disabled" || history[0].Actor != who {
		t.Errorf("the Registry holds %+v, want one decision by %s", history, who)
	}
	if policy := b.events.of("plugin.policy.changed"); len(policy) != 1 || policy[0].Actor != (event.Actor{Class: event.ByOperator, Person: who}) {
		t.Errorf("plugin.policy.changed was published as %+v", policy)
	}

	resp, ref = b.call(t, disable(station))
	if ref != nil {
		t.Fatal(ref)
	}
	if c := changed(resp); c.GetPreviously().GetEnabled() || len(c.GetConsequences()) != 0 {
		t.Errorf("disabling again answered %v, want that it was disabled and nothing followed", c)
	}
	if history, _ := b.core.Registry.History(station); len(history) != 1 {
		t.Errorf("disabling again left %d decisions, want 1", len(history))
	}
}

// std: yoke:the-operations.03
func TestAGrantIsCheckedAndReachesTheNextAdmission(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 2)}, map[string]string{"acquire": "reverse"})
	resp, ref := b.call(t, grant(station, "stream.data.publish"))
	if ref != nil {
		t.Fatal(ref)
	}
	c := changed(resp)
	if c.GetPreviously().GetGranted() || c.GetEffective() != administrativev1.Changed_EFFECTIVE_AT_NEXT_ADMISSION ||
		len(c.GetConsequences()) != 1 || c.GetConsequences()[0].GetUnit() != "acquire" || c.GetConsequences()[0].GetIncarnation() != 2 {
		t.Errorf("the grant answered %v", c)
	}
	if _, ref := b.call(t, grant(station, "head.move")); ref.GetCode() != "capability.undeclared" || ref.GetItem() != "head.move" {
		t.Errorf("an undeclared capability was answered %v", ref)
	}
	if _, ref := b.call(t, grant("com.example.nobody", "stream.data.publish")); ref.GetCode() != "subject.unknown" ||
		ref.GetSubject().GetKind() != "plugin" || ref.GetSubject().GetIdentity() != "com.example.nobody" {
		t.Errorf("a plugin nobody declared was answered %v", ref)
	}
	if _, ref := b.call(t, grant("acquire", "stream.data.publish")); ref.GetCode() != "subject.wrong_kind" {
		t.Errorf("a unit named as a plugin was answered %v", ref)
	}
}

// std: yoke:the-operations.04
func TestAUnitsLivesEndedAndBegunAreAnswered(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 2)}, map[string]string{})
	step := func(op string, wantPrevious string, want ...string) {
		t.Helper()
		resp, ref := b.call(t, unitAct(op, "acquire"))
		if ref != nil {
			t.Fatalf("%s was refused %v", op, ref)
		}
		c := changed(resp)
		var got []string
		for _, q := range c.GetConsequences() {
			got = append(got, fmt.Sprintf("%d %s", q.GetIncarnation(), q.GetWhat()))
		}
		if c.GetPreviously().GetState() != wantPrevious || c.GetEffective() != administrativev1.Changed_EFFECTIVE_IMMEDIATELY || !slices.Equal(got, want) {
			t.Errorf("%s answered %s, %v, %v; want %s and %v", op, c.GetPreviously().GetState(), c.GetEffective(), got, wantPrevious, want)
		}
	}
	step("stop", "Running", "2 ended")
	step("start", "Stopped", "3 begun")
	step("start", "Running")
	step("restart", "Running", "3 ended", "4 begun")
	if _, ref := b.call(t, unitAct("stop", "nobody")); ref.GetCode() != "subject.unknown" || ref.GetSubject().GetKind() != "unit" {
		t.Errorf("a unit nobody declared was answered %v", ref)
	}
	b.units.unreachable = true
	if _, ref := b.call(t, unitAct("stop", "acquire")); ref.GetCode() != "backend.unavailable" {
		t.Errorf("an unreachable backend was answered %v", ref)
	}
}

// std: yoke:the-operations.05
func TestARetentionOverrideIsWrittenToTheLogStore(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	if _, ref := b.call(t, retain("acquire", 0, count(0))); ref.GetCode() != "retention.invalid" {
		t.Errorf("a limit of zero was answered %v", ref)
	}
	week := 7 * 24 * time.Hour
	resp, ref := b.call(t, retain("acquire", week, count(1000)))
	if ref != nil {
		t.Fatal(ref)
	}
	if was := changed(resp).GetPreviously().GetRetention(); was == nil || was.GetAge() != nil || was.Bytes != nil || was.Entries != nil {
		t.Errorf("the first override answered %v, want an unconstrained policy", was)
	}
	if o, ok, _ := b.core.Logs.Override("acquire"); !ok || o.Age != week || o.Entries == nil || *o.Entries != 1000 || o.Bytes != nil {
		t.Errorf("the log store holds %+v (%v), want 7 days and 1000 entries", o, ok)
	}
	resp, ref = b.call(t, retain("acquire", 0, nil))
	if ref != nil {
		t.Fatalf("an override with no limit was refused %v", ref)
	}
	if was := changed(resp).GetPreviously().GetRetention(); was.GetAge().AsDuration() != week || was.GetEntries() != 1000 {
		t.Errorf("the second override answered %v, want 7 days and 1000 entries", was)
	}
	resp, ref = b.call(t, unitAct("retention.clear", "acquire"))
	if ref != nil {
		t.Fatal(ref)
	}
	if was := changed(resp).GetPreviously().GetRetention(); was == nil || was.GetAge() != nil || was.Entries != nil {
		t.Errorf("the clear answered %v, want the unconstrained override it removed", was)
	}
	if _, ok, _ := b.core.Logs.Override("acquire"); ok {
		t.Error("the log store still holds an override")
	}
}

// std: yoke:the-operations.06
func TestAQuestionIsCarriedOpaqueAndRecordedWithoutIt(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 4), "archive": running(station, 1),
		"idle": {plugin: station, state: unit.Stopped, incarnation: 1}, "mute": running(station, 1)},
		map[string]string{"acquire": "reverse", "mute": "silent"})
	b.core.Wait = 100 * time.Millisecond
	resp, ref := b.call(t, ask("acquire", []byte("abc-question")))
	if ref != nil {
		t.Fatal(ref)
	}
	if got := string(resp.GetUnitAsk().GetAnswer()); got != "noitseuq-cba" {
		t.Errorf("the answer is %q, want the bytes reversed", got)
	}
	all := entries(t, b.core.Logs, func(all []logstore.Entry) bool { return len(all) > 0 })
	var asked []logstore.Entry
	for _, e := range all {
		if e.Unit == "acquire" {
			asked = append(asked, e)
		}
	}
	if len(asked) != 1 || asked[0].Incarnation != 4 || asked[0].Actor != "operator:"+me(t).Username {
		t.Errorf("the log store holds %+v, want one entry by the operator about acquire's life 4", asked)
	}
	for _, e := range asked {
		if strings.Contains(e.Message+string(e.Detail), "question") || strings.Contains(e.Message+string(e.Detail), "noitseuq") {
			t.Errorf("the entry holds the question or the answer: %+v", e)
		}
	}
	if _, ref := b.call(t, ask("acquire", bytes.Repeat([]byte{1}, admin.Bound+1))); ref.GetCode() != "operation.malformed" {
		t.Errorf("an oversized question was answered %v", ref)
	}
	for id, want := range map[string]string{"archive": "unit.no_session", "idle": "unit.not_running", "mute": "unit.unanswered"} {
		if _, ref := b.call(t, ask(id, []byte("x"))); ref.GetCode() != want || ref.GetSubject().GetIdentity() != id {
			t.Errorf("asking %s was answered %v, want %s naming it", id, ref, want)
		}
	}
}

// std: yoke:the-operations.07
func TestEveryChangeIsRecordedAgainstItsActor(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	b.core.Registry.Disable(station, "someone")
	for _, r := range []*administrativev1.Request{enable(station), enable(station), unitAct("stop", "acquire")} {
		if _, ref := b.call(t, r); ref != nil {
			t.Fatal(ref)
		}
	}
	who := "operator:" + me(t).Username
	all := entries(t, b.core.Logs, func(all []logstore.Entry) bool { return len(all) >= 2 })
	time.Sleep(300 * time.Millisecond)
	all, _ = b.core.Logs.Entries(0)
	var mine []string
	for _, e := range all {
		if e.Actor == who {
			mine = append(mine, e.SubjectKind+":"+e.SubjectID)
		}
	}
	if !slices.Equal(mine, []string{"plugin:" + station, "unit:acquire"}) {
		t.Errorf("the log store holds %v by %s, want the enablement and the stop", mine, who)
	}
}

// std: yoke:the-operations.08
func TestWhileStoppingAChangeIsRefusedAndAReadAnswered(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{"acquire": "reverse"})
	ops := b.core.Operations()
	ops["read"] = newFakes().operations()["read"]
	_, operator, _ := served(t, admin.Config{Operations: ops, Stopping: func() bool { return true }})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for name, r := range map[string]*administrativev1.Request{"plugin.enable": enable(station), "unit.ask": ask("acquire", []byte("x"))} {
		_, err := operator.Call(ctx, r)
		if ref := refusalOf(err); ref.GetCode() != "instance.stopping" {
			t.Errorf("%s while stopping was answered %v, want instance.stopping", name, err)
		}
	}
	if _, err := operator.Call(ctx, read("acquire")); err != nil {
		t.Errorf("a read while stopping was refused %v", err)
	}
}

// std: yoke:the-operations.09
func TestStreamControlWaitsForItsTransports(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	var served []string
	for name := range b.core.Operations() {
		served = append(served, name)
	}
	slices.Sort(served)
	want := []string{"plugin.disable", "plugin.enable", "plugin.grant", "plugin.withdraw", "unit.ask", "unit.restart",
		"unit.retention.clear", "unit.retention.set", "unit.start", "unit.stop"}
	for _, name := range want {
		if !slices.Contains(served, name) {
			t.Errorf("the Core does not serve %s", name)
		}
	}
	for _, name := range []string{"unit.stream.start", "unit.stream.stop"} {
		if slices.Contains(served, name) {
			t.Errorf("the Core serves %s", name)
		}
	}
	start := v1(&administrativev1.Request{Operation: &administrativev1.Request_UnitStreamStart{UnitStreamStart: &administrativev1.UnitStream{Unit: "acquire", Stream: "station.data"}}})
	if _, ref := b.call(t, start); ref.GetCode() != "operation.unknown" {
		t.Errorf("unit.stream.start was answered %v, want operation.unknown", ref)
	}
}
