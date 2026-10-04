package admin_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// Instruct hands a unit a control instruction, and the unit accepts it at once; a stream that is declared
// and not granted is refused as the Session would refuse it.
func (f *fakeSessions) Instruct(_ context.Context, id string, c *pluginv1.Control) (*pluginv1.Ack, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.open[id]; !ok {
		return nil, fmt.Errorf("no Session")
	}
	stream := c.GetActivate().GetStream() + c.GetStop().GetStream()
	if stream == "station.preview" {
		return nil, &session.Refused{Code: pluginv1.Code_CODE_SCOPE_WITHHELD, Reason: "not granted"}
	}
	present := false
	if a := c.GetActivate(); a != nil {
		_, err := os.Stat(a.GetAddress())
		present = err == nil
	}
	f.instructed = append(f.instructed, instruction{unit: id, control: c, present: present})
	return &pluginv1.Ack{Outcome: pluginv1.Ack_OUTCOME_DONE}, nil
}

func (f *fakeSessions) instructions() []instruction {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.instructed)
}

// streamBench is a bench whose station declares two streams, of which the tests' Sessions grant one,
// with a real transport service over a short root.
func streamBench(t *testing.T) (*bench, *streams.Service) {
	t.Helper()
	b := newBench(t, map[string]*fakeUnit{
		"acquire": {plugin: station, state: unit.Running, incarnation: 2},
		"idle":    {plugin: station, state: unit.Running, incarnation: 1},
	}, map[string]string{"acquire": "reverse"})
	root, _ := os.MkdirTemp("", "ya")
	t.Cleanup(func() { os.RemoveAll(root) })
	svc := streams.New(streams.Config{Root: root, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Publish: b.events.publish})
	t.Cleanup(func() { svc.CloseAll("acquire", streams.UnitExited) })
	manifest := &gate.Manifest{ID: station, Streams: []gate.Stream{{ID: "station.spectra"}, {ID: "station.preview", ToleratesLoss: true}},
		Capabilities: []gate.Capability{{Name: "stream.data.publish"}}}
	b.core.Manifest = func(id string) (*gate.Manifest, bool) { return manifest, id == station }
	b.core.Streams = svc
	return b, svc
}

func streamRequest(start bool, unitID, stream string) *administrativev1.Request {
	us := &administrativev1.UnitStream{Unit: unitID, Stream: stream}
	if start {
		return &administrativev1.Request{Version: 1, Operation: &administrativev1.Request_UnitStreamStart{UnitStreamStart: us}}
	}
	return &administrativev1.Request{Version: 1, Operation: &administrativev1.Request_UnitStreamStop{UnitStreamStop: us}}
}

func streamsOf(t *testing.T, b *bench, unitID string) []string {
	t.Helper()
	resp, ref := b.call(t, &administrativev1.Request{Version: 1, Operation: &administrativev1.Request_Read{Read: &administrativev1.Read{Kind: "unit", Identity: unitID}}})
	if ref != nil || len(resp.GetRead().GetRecords()) != 1 {
		t.Fatalf("the read of %s answered %v %v", unitID, resp, ref)
	}
	return resp.GetRead().GetRecords()[0].GetUnit().GetObserved().GetStreams()
}

// std: yoke:a-streams-transport.06
func TestUnitStreamStartCreatesTheTransportThenActivates(t *testing.T) {
	b, svc := streamBench(t)
	for _, c := range []struct {
		unit, stream, code, names string
	}{
		{"nobody", "station.spectra", "subject.unknown", "nobody"},
		{"acquire", "station.nothing", "stream.undeclared", "station.nothing"},
		{"acquire", "station.preview", "scope.withheld", "station.preview"},
		{"idle", "station.spectra", "unit.no_session", "idle"},
	} {
		_, ref := b.call(t, streamRequest(true, c.unit, c.stream))
		if ref.GetCode() != c.code || ref.GetItem()+ref.GetSubject().GetIdentity() != c.names {
			t.Errorf("starting %s of %s was refused %v, want %s naming %s", c.stream, c.unit, ref, c.code, c.names)
		}
		if active := svc.Active(c.unit); len(active) != 0 {
			t.Errorf("starting %s of %s left %v open", c.stream, c.unit, active)
		}
	}

	resp, ref := b.call(t, streamRequest(true, "acquire", "station.spectra"))
	c := changed(resp)
	if ref != nil || c == nil || c.GetPreviously().GetState() != "stopped" || c.GetEffective() != administrativev1.Changed_EFFECTIVE_IMMEDIATELY ||
		len(c.GetConsequences()) != 1 || c.GetConsequences()[0].GetUnit() != "acquire" || c.GetConsequences()[0].GetIncarnation() != 2 {
		t.Fatalf("the start answered %v %v", resp, ref)
	}
	got := b.sessions.instructions()
	if len(got) != 1 || got[0].unit != "acquire" || got[0].control.GetActivate().GetStream() != "station.spectra" ||
		got[0].control.GetActivate().GetTransport() != pluginv1.Control_Activate_TRANSPORT_ORDERED || !got[0].present {
		t.Errorf("the unit was handed %+v", got)
	}
	activated := b.events.of("unit.stream.activated")
	var detail struct{ Stream string }
	if len(activated) == 1 {
		json.Unmarshal(activated[0].Detail, &detail)
	}
	if len(activated) != 1 || activated[0].Subject.ID != "acquire" || activated[0].Subject.Incarnation != 2 || detail.Stream != "station.spectra" {
		t.Errorf("published %+v", activated)
	}
	if listed := streamsOf(t, b, "acquire"); !slices.Equal(listed, []string{"station.spectra"}) {
		t.Errorf("the unit's record lists %v", listed)
	}

	resp, ref = b.call(t, streamRequest(true, "acquire", "station.spectra"))
	if c := changed(resp); ref != nil || c.GetPreviously().GetState() != "activated" || len(c.GetConsequences()) != 0 {
		t.Errorf("the second start answered %v %v", resp, ref)
	}
	if n := len(b.sessions.instructions()); n != 1 {
		t.Errorf("the second start handed the unit %d instructions in all", n)
	}
}

// std: yoke:a-streams-transport.07
func TestUnitStreamStopInstructsTheUnitThenRemovesTheTransport(t *testing.T) {
	b, svc := streamBench(t)
	if _, ref := b.call(t, streamRequest(true, "acquire", "station.spectra")); ref != nil {
		t.Fatalf("the start was refused: %v", ref)
	}
	address := svc.Address("acquire", "station.spectra")
	resp, ref := b.call(t, streamRequest(false, "acquire", "station.spectra"))
	if c := changed(resp); ref != nil || c.GetPreviously().GetState() != "activated" || len(c.GetConsequences()) != 1 {
		t.Fatalf("the stop answered %v %v", resp, ref)
	}
	got := b.sessions.instructions()
	if len(got) != 2 || got[1].control.GetStop().GetStream() != "station.spectra" {
		t.Errorf("the unit was handed %+v", got)
	}
	if _, err := os.Stat(address); err == nil {
		t.Errorf("%s is still there", address)
	}
	deadline := time.Now().Add(time.Second)
	for len(b.events.of("unit.stream.stopped")) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	stopped := b.events.of("unit.stream.stopped")
	var detail struct{ Stream, Reason string }
	if len(stopped) == 1 {
		json.Unmarshal(stopped[0].Detail, &detail)
	}
	if len(stopped) != 1 || detail.Stream != "station.spectra" || detail.Reason != "asked" {
		t.Errorf("published %+v", stopped)
	}
	if listed := streamsOf(t, b, "acquire"); len(listed) != 0 {
		t.Errorf("the unit's record still lists %v", listed)
	}
	resp, ref = b.call(t, streamRequest(false, "acquire", "station.spectra"))
	if c := changed(resp); ref != nil || c.GetPreviously().GetState() != "stopped" || len(c.GetConsequences()) != 0 {
		t.Errorf("the second stop answered %v %v", resp, ref)
	}
	if n := len(b.sessions.instructions()); n != 2 {
		t.Errorf("the second stop handed the unit %d instructions in all", n)
	}
}
