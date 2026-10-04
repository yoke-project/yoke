package interfaces_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// bench is a deployment of Plugin units in every condition an operation can meet, and a unit that runs to
// completion.
type bench struct{}

var benchUnits = map[string]struct {
	kind  unit.Kind
	state unit.State
}{
	"acquire":        {unit.Plugin, unit.Running},
	"halted":         {unit.Plugin, unit.Stopped},
	"idle":           {unit.Plugin, unit.Running},
	"mute":           {unit.Plugin, unit.Running},
	"broken":         {unit.Plugin, unit.Running},
	"calibrate-once": {unit.Oneshot, unit.Completed},
}

func (bench) IDs() []string {
	var ids []string
	for id := range benchUnits {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
func (bench) Kind(id string) unit.Kind { return benchUnits[id].kind }
func (bench) Status(id string) supervisor.Status {
	return supervisor.Status{State: benchUnits[id].state, Incarnation: 3}
}

// fakeSessions are the Sessions of the bench's units: acquire acknowledges and answers, mute never
// answers, broken fails, idle and halted have none. A type or stream named `zero` is declared and not
// granted.
type fakeSessions struct {
	mu      sync.Mutex
	handed  []*pluginv1.Control
	present []bool
}

func (f *fakeSessions) Open(id string) bool { return id == "acquire" || id == "mute" || id == "broken" }

func (f *fakeSessions) failure(ctx context.Context, id, named string) error {
	switch {
	case !f.Open(id):
		return errors.New("no Session")
	case named == "zero" || named == "station.preview":
		return &session.Refused{Code: pluginv1.Code_CODE_SCOPE_WITHHELD, Reason: "not granted"}
	case id == "mute":
		<-ctx.Done()
		return ctx.Err()
	case id == "broken":
		return &session.UnitFailed{Code: "instrument.busy", Message: "the lamp is warming"}
	}
	return nil
}

func (f *fakeSessions) Instruct(ctx context.Context, id string, c *pluginv1.Control) (*pluginv1.Ack, error) {
	named := c.GetCommand().GetType() + c.GetActivate().GetStream() + c.GetStop().GetStream()
	if err := f.failure(ctx, id, named); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	present := false
	if a := c.GetActivate(); a != nil {
		_, err := os.Stat(a.GetAddress())
		present = err == nil
	}
	f.handed, f.present = append(f.handed, c), append(f.present, present)
	return &pluginv1.Ack{Outcome: pluginv1.Ack_OUTCOME_DONE, Line: "done"}, nil
}

func (f *fakeSessions) Ask(ctx context.Context, id string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error) {
	if err := f.failure(ctx, id, q.GetType()); err != nil {
		return nil, err
	}
	answer := slices.Clone(q.GetPayload())
	slices.Reverse(answer)
	return &pluginv1.Query_Answer{Payload: answer}, nil
}

type acting struct {
	*attached
	w         *world
	sessions  *fakeSessions
	stopping  *bool
	transport *streams.Service
}

func actingOn(t *testing.T) *acting {
	t.Helper()
	w := newWorld()
	stopping := false
	sessions := &fakeSessions{}
	r := root(t)
	transports := streams.New(streams.Config{Root: r, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Publish: w.publish})
	t.Cleanup(func() { transports.CloseAll("acquire", streams.UnitExited) })
	manifest := &gate.Manifest{ID: "com.example.station", Commands: []string{"calibrate", "zero"}, Queries: []string{"head-status"},
		Streams: []gate.Stream{{ID: "station.spectra"}, {ID: "station.preview", ToleratesLoss: true}}}
	b, err := interfaces.BindServing(r, mode, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}}, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{
			Channel: ch, Bus: w.bus, Publish: w.publish, Units: bench{}, Sessions: sessions, Transports: transports, Wait: 200 * time.Millisecond,
			Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
			Stopping: func() bool { return stopping },
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	a, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	a.next(t)
	return &acting{attached: a, w: w, sessions: sessions, stopping: &stopping, transport: transports}
}

func command(unitID, typ string, payload []byte) *interfacev1.Request {
	return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Command{Command: &interfacev1.Command{Unit: unitID, Type: typ, Payload: payload}}}
}

// std: yoke:channel-operations.01
func TestACommandIsCarriedAndItsAcknowledgementComesBack(t *testing.T) {
	a := actingOn(t)
	f := a.answerTo(t, "c", command("acquire", "calibrate", []byte{1, 2, 3}))
	if ack := f.GetAnswer().GetCommand(); ack.GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE || ack.GetLine() != "done" {
		t.Errorf("the command was answered %v", f)
	}
	a.sessions.mu.Lock()
	defer a.sessions.mu.Unlock()
	if len(a.sessions.handed) != 1 || a.sessions.handed[0].GetCommand().GetType() != "calibrate" || !bytes.Equal(a.sessions.handed[0].GetCommand().GetPayload(), []byte{1, 2, 3}) {
		t.Errorf("the Session was handed %v", a.sessions.handed)
	}
}

// std: yoke:channel-operations.02
func TestAQuestionIsCarriedAndItsAnswerComesBackOpaque(t *testing.T) {
	a := actingOn(t)
	query := func(payload []byte) *interfacev1.Request {
		return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Query{Query: &interfacev1.Question{Unit: "acquire", Type: "head-status", Payload: payload}}}
	}
	if f := a.answerTo(t, "q", query([]byte{1, 2, 3})); !bytes.Equal(f.GetAnswer().GetQuery().GetPayload(), []byte{3, 2, 1}) {
		t.Errorf("the question was answered %v", f)
	}
	if f := a.answerTo(t, "big", query(make([]byte, 1<<20+1))); f.GetRefusal().GetCode() != "operation.malformed" {
		t.Errorf("a question over 1 MiB was answered %v", f)
	}
}

// std: yoke:channel-operations.03
func TestARefusalIsDecidedInTheContractsOrder(t *testing.T) {
	a := actingOn(t)
	for i, c := range []struct {
		unit, typ, code, names string
	}{
		{"nobody", "calibrate", "subject.unknown", "nobody"},
		{"calibrate-once", "calibrate", "subject.unknown", "calibrate-once"},
		{"acquire", "dance", "scope.undeclared", "dance"},
		{"acquire", "zero", "scope.withheld", "zero"},
		{"halted", "calibrate", "unit.not_running", "halted"},
		{"idle", "calibrate", "unit.no_session", "idle"},
		{"mute", "calibrate", "unit.unanswered", "mute"},
		{"broken", "calibrate", "unit.failed", "instrument.busy"},
	} {
		ref := a.answerTo(t, fmt.Sprint(i), command(c.unit, c.typ, nil)).GetRefusal()
		if ref.GetCode() != c.code || ref.GetItem()+ref.GetSubject().GetIdentity() != c.names {
			t.Errorf("commanding %s with %s was refused %v, want %s naming %s", c.unit, c.typ, ref, c.code, c.names)
		}
	}
	*a.stopping = true
	if ref := a.answerTo(t, "s", command("acquire", "calibrate", nil)).GetRefusal(); ref.GetCode() != "instance.stopping" {
		t.Errorf("a command while stopping was refused %v", ref)
	}
}

// std: yoke:channel-operations.04
func TestStreamStartAndStopCreateAndRemoveTheTransport(t *testing.T) {
	a := actingOn(t)
	stream := func(start bool, name string) *interfacev1.Request {
		us := &interfacev1.UnitStream{Unit: "acquire", Stream: name}
		if start {
			return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamStart{StreamStart: us}}
		}
		return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamStop{StreamStop: us}}
	}
	if f := a.answerTo(t, "start", stream(true, "station.spectra")); f.GetAnswer().GetStreamStart().GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE {
		t.Fatalf("the start was answered %v", f)
	}
	address := a.transport.Address("acquire", "station.spectra")
	a.sessions.mu.Lock()
	handed := slices.Clone(a.sessions.handed)
	present := slices.Clone(a.sessions.present)
	a.sessions.mu.Unlock()
	if len(handed) != 1 || handed[0].GetActivate().GetStream() != "station.spectra" || !present[0] || address == "" {
		t.Errorf("the unit was handed %v, the transport present %v", handed, present)
	}
	if n := len(a.w.of("unit.stream.activated")); n != 1 {
		t.Errorf("%d activations were published", n)
	}
	if f := a.answerTo(t, "stop", stream(false, "station.spectra")); f.GetAnswer().GetStreamStop().GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE {
		t.Errorf("the stop was answered %v", f)
	}
	if _, err := os.Stat(address); err == nil {
		t.Error("the transport is still there")
	}
	if ref := a.answerTo(t, "undeclared", stream(true, "station.nothing")).GetRefusal(); ref.GetCode() != "scope.undeclared" || ref.GetItem() != "station.nothing" {
		t.Errorf("a stream not declared was refused %v", ref)
	}
}

// std: yoke:channel-operations.05
func TestAuthenticateSaysWhoTheChannelEstablished(t *testing.T) {
	a := actingOn(t)
	f := a.answerTo(t, "auth", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Authenticate{Authenticate: &interfacev1.Authenticate{}}})
	me, _ := user.LookupId(strconv.Itoa(os.Getuid()))
	if got := f.GetAnswer().GetAuthenticate(); got.GetAccount() != me.Username || got.GetToken() != "" {
		t.Errorf("authenticate was answered %v", f)
	}
}
