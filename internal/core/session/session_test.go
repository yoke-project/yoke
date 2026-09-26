package session_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// harness is a Session service on a socket, with the Sessions admission issued and what the machine is told.
type harness struct {
	svc    *session.Service
	conn   *grpc.ClientConn
	log    *syncBuffer
	mu     sync.Mutex
	issued map[string]session.Admitted
	told   map[string][]string
}

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{log: &syncBuffer{}, issued: map[string]session.Admitted{}, told: map[string][]string{}}
	h.svc = session.New(session.Config{
		Lookup: func(id string) (session.Admitted, bool) {
			h.mu.Lock()
			defer h.mu.Unlock()
			a, ok := h.issued[id]
			return a, ok
		},
		Observe: func(id string, in unit.Input) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.told[id] = append(h.told[id], fmt.Sprintf("%T%+v", in, in))
		},
		Log: slog.New(slog.NewTextHandler(h.log, nil)),
	})
	dir, _ := os.MkdirTemp("", "ys")
	t.Cleanup(func() { os.RemoveAll(dir) })
	listener, err := net.Listen("unix", filepath.Join(dir, "plugin.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pluginv1.RegisterSessionServer(server, h.svc)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	h.conn, err = grpc.NewClient("unix://"+listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.conn.Close() })
	return h
}

// admit issues a Session identity for a unit, with the heartbeat terms given.
func (h *harness) admit(id, unitID string, interval time.Duration, tolerance uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.issued[id] = session.Admitted{Unit: unitID, Interval: interval, Tolerance: tolerance}
}

func (h *harness) toldOf(unitID string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.told[unitID]...)
}

// stream is a unit's side of one stream: what it sends, and what arrives, until the stream closes.
type stream struct {
	s       pluginv1.Session_OpenClient
	cancel  context.CancelFunc
	arrived chan *pluginv1.Envelope
	next    int
	id      string
}

func (h *harness) stream(t *testing.T, id string) *stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s, err := pluginv1.NewSessionClient(h.conn).Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := &stream{s: s, cancel: cancel, arrived: make(chan *pluginv1.Envelope, 16), id: id}
	go func() {
		defer close(st.arrived)
		for {
			e, err := s.Recv()
			if err != nil {
				return
			}
			st.arrived <- e
		}
	}()
	return st
}

// send sends one envelope with a fresh message identity, and returns that identity.
func (st *stream) send(t *testing.T, fill func(e *pluginv1.Envelope)) string {
	t.Helper()
	st.next++
	e := &pluginv1.Envelope{MessageId: fmt.Sprintf("u-%d", st.next), SessionId: st.id, SentAtUnixNano: time.Now().UnixNano()}
	fill(e)
	if err := st.s.Send(e); err != nil {
		t.Fatalf("sending: %v", err)
	}
	return e.MessageId
}

func open(e *pluginv1.Envelope) {
	e.Payload = &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Open_{Open: &pluginv1.SessionMessage_Open{}}}}
}

func heartbeat(e *pluginv1.Envelope) {
	e.Payload = &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: 90}}
}

func closing(e *pluginv1.Envelope) {
	e.Payload = &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Close_{Close: &pluginv1.SessionMessage_Close{}}}}
}

func ack(correlation string) func(e *pluginv1.Envelope) {
	return func(e *pluginv1.Envelope) {
		e.CorrelationId = correlation
		e.Payload = &pluginv1.Envelope_Ack{Ack: &pluginv1.Ack{Outcome: pluginv1.Ack_OUTCOME_DONE}}
	}
}

// receive waits for the next envelope; nil means the stream closed.
func (st *stream) receive(t *testing.T, within time.Duration) (*pluginv1.Envelope, bool) {
	t.Helper()
	select {
	case e, open := <-st.arrived:
		return e, open
	case <-time.After(within):
		t.Fatalf("nothing arrived and the stream did not close within %v", within)
		return nil, false
	}
}

// quiet says nothing arrives, and the stream stays open, for the duration.
func (st *stream) quiet(t *testing.T, d time.Duration) {
	t.Helper()
	select {
	case e, open := <-st.arrived:
		if open {
			t.Fatalf("%v arrived where nothing should", e)
		}
		t.Fatal("the stream closed where it should have stayed open")
	case <-time.After(d):
	}
}

// closedWithNothing says the stream closes with nothing sent to it.
func (st *stream) closedWithNothing(t *testing.T) {
	t.Helper()
	if e, open := st.receive(t, 2*time.Second); open {
		t.Fatalf("%v arrived on a stream that should have closed with nothing", e)
	}
}

func errorFor(t *testing.T, st *stream, code, correlation string) {
	t.Helper()
	e, open := st.receive(t, 2*time.Second)
	if !open {
		t.Fatalf("the stream closed where an error %s was due", code)
	}
	if e.GetError().GetCode() != code || e.CorrelationId != correlation {
		t.Fatalf("%v arrived, want an error %s correlated to %q", e, code, correlation)
	}
}

// std: yoke:the-session.01
func TestTheFirstEnvelopeIsAnOpenCarryingTheIssuedIdentity(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", time.Second, 3)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	st.send(t, heartbeat)
	st.quiet(t, 200*time.Millisecond)
	if got := h.toldOf("acquire"); !slices.Equal(got, []string{"unit.SessionOpened{}"}) {
		t.Errorf("the machine was told %v", got)
	}
}

// std: yoke:the-session.02
func TestAnythingElseFirstClosesTheStream(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", time.Second, 3)
	first := h.stream(t, "sid-1")
	first.send(t, heartbeat)
	first.closedWithNothing(t)

	never := h.stream(t, "never-issued")
	never.send(t, open)
	never.closedWithNothing(t)

	good := h.stream(t, "sid-1")
	good.send(t, open)
	good.quiet(t, 200*time.Millisecond)

	again := h.stream(t, "sid-1")
	again.send(t, open)
	again.closedWithNothing(t)
	if got := h.toldOf("acquire"); !slices.Equal(got, []string{"unit.SessionOpened{}"}) {
		t.Errorf("the machine was told %v", got)
	}
}

// std: yoke:the-session.03
func TestAnIdentityIsUsedOnce(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", time.Second, 3)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	st.send(t, closing)
	st.closedWithNothing(t)
	again := h.stream(t, "sid-1")
	again.send(t, open)
	again.closedWithNothing(t)
}

// std: yoke:the-session.04
func TestAnEnvelopeCarriesFourFieldsAndOnePayload(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", time.Second, 3)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	empty := st.send(t, func(e *pluginv1.Envelope) {})
	errorFor(t, st, "session.message.malformed", empty)
	st.send(t, func(e *pluginv1.Envelope) { e.MessageId = ""; heartbeat(e) })
	errorFor(t, st, "session.message.malformed", "")

	before := time.Now().UnixNano()
	id, err := h.svc.Send("sid-1", &pluginv1.Envelope{Payload: &pluginv1.Envelope_Control{Control: &pluginv1.Control{
		Kind: &pluginv1.Control_Command_{Command: &pluginv1.Control_Command{Type: "calibrate"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	e, _ := st.receive(t, 2*time.Second)
	if e.MessageId != id || id == "" || e.SessionId != "sid-1" || e.SentAtUnixNano < before || e.CorrelationId != "" || e.GetControl().GetCommand().GetType() != "calibrate" {
		t.Errorf("the command arrived as %v", e)
	}
}

// std: yoke:the-session.05
func TestAReceiverValidatesInOrder(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", time.Second, 3)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	st.send(t, func(e *pluginv1.Envelope) { e.SessionId = "sid-other"; heartbeat(e) })
	st.quiet(t, 150*time.Millisecond)
	if !bytes.Contains([]byte(h.log.String()), []byte("dropped")) {
		t.Errorf("the dropped message was not recorded:\n%s", h.log)
	}
	used := st.send(t, heartbeat)
	st.quiet(t, 100*time.Millisecond)
	st.send(t, func(e *pluginv1.Envelope) { heartbeat(e); e.MessageId = used })
	errorFor(t, st, "session.message.duplicate", used)
	control := func(e *pluginv1.Envelope) {
		e.Payload = &pluginv1.Envelope_Control{Control: &pluginv1.Control{Kind: &pluginv1.Control_Stop_{Stop: &pluginv1.Control_Stop{Stream: "x"}}}}
	}
	st.send(t, func(e *pluginv1.Envelope) { control(e); e.MessageId = used })
	errorFor(t, st, "session.message.duplicate", used)
	fresh := st.send(t, control)
	errorFor(t, st, "session.direction", fresh)
}

// std: yoke:the-session.06
func TestAMessageThatAnswersIsCorrelated(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", time.Second, 3)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	// The Core can send on a Session only once it has opened.
	for deadline := time.Now().Add(2 * time.Second); len(h.toldOf("acquire")) == 0 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
	}
	command, err := h.svc.Send("sid-1", &pluginv1.Envelope{Payload: &pluginv1.Envelope_Control{Control: &pluginv1.Control{
		Kind: &pluginv1.Control_Command_{Command: &pluginv1.Control_Command{Type: "calibrate"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	st.receive(t, 2*time.Second)
	missing := st.send(t, ack(""))
	errorFor(t, st, "session.correlation.missing", missing)
	unknown := st.send(t, ack("c-never"))
	errorFor(t, st, "session.correlation.unknown", unknown)
	st.next++
	self := fmt.Sprintf("u-%d", st.next)
	st.next--
	st.send(t, ack(self))
	errorFor(t, st, "session.correlation.unknown", self)
	st.send(t, ack(command))
	st.quiet(t, 200*time.Millisecond)
}

// std: yoke:the-session.07
func TestHeartbeatsKeepItValidAndMissesRevokeIt(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", 100*time.Millisecond, 3)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	for range 12 {
		st.send(t, heartbeat)
		st.quiet(t, 50*time.Millisecond)
	}
	e, open := st.receive(t, 500*time.Millisecond)
	if !open || e.GetSession().GetRevoked().GetCause() != pluginv1.SessionMessage_Revoked_CAUSE_LIVENESS_LOST {
		t.Fatalf("after the heartbeats stopped %v arrived (open %v), want REVOKED for lost liveness", e, open)
	}
	st.closedWithNothing(t)
	if got := h.toldOf("acquire"); !slices.Equal(got, []string{"unit.SessionOpened{}", "unit.SessionEnded{Withdrawn:false}"}) {
		t.Errorf("the machine was told %v", got)
	}
}

// std: yoke:the-session.08
func TestLosingTheStreamIsNotClosingIt(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "acquire", 200*time.Millisecond, 2)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	st.send(t, heartbeat)
	time.Sleep(50 * time.Millisecond)
	st.cancel()
	time.Sleep(100 * time.Millisecond)
	if got := h.toldOf("acquire"); !slices.Equal(got, []string{"unit.SessionOpened{}"}) {
		t.Fatalf("losing the stream concluded %v at once", got)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := h.toldOf("acquire"); len(got) == 2 {
			if got[1] != "unit.SessionEnded{Withdrawn:false}" {
				t.Fatalf("the machine was told %v", got)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("once the window passed the machine was told %v", h.toldOf("acquire"))
}

// std: yoke:the-session.09
func TestAnOrderlyCloseIsTheUnitsAndARevocationIsTheCores(t *testing.T) {
	h := newHarness(t)
	h.admit("sid-1", "first", time.Second, 3)
	h.admit("sid-2", "second", time.Second, 3)
	first, second := h.stream(t, "sid-1"), h.stream(t, "sid-2")
	first.send(t, open)
	second.send(t, open)
	time.Sleep(100 * time.Millisecond)

	first.send(t, closing)
	first.closedWithNothing(t)
	if err := h.svc.Revoke("sid-2", pluginv1.SessionMessage_Revoked_CAUSE_PLUGIN_DISABLED, "an operator disabled the plugin"); err != nil {
		t.Fatal(err)
	}
	e, open := second.receive(t, 2*time.Second)
	if !open || e.GetSession().GetRevoked().GetCause() != pluginv1.SessionMessage_Revoked_CAUSE_PLUGIN_DISABLED || e.GetSession().GetRevoked().GetLine() != "an operator disabled the plugin" {
		t.Fatalf("the second unit received %v (open %v)", e, open)
	}
	second.closedWithNothing(t)
	if got := h.toldOf("first"); !slices.Equal(got, []string{"unit.SessionOpened{}", "unit.SessionEnded{Withdrawn:false}"}) {
		t.Errorf("the first machine was told %v", got)
	}
	if got := h.toldOf("second"); !slices.Equal(got, []string{"unit.SessionOpened{}", "unit.SessionEnded{Withdrawn:true}"}) {
		t.Errorf("the second machine was told %v", got)
	}
	logged := h.log.String()
	if !bytes.Contains([]byte(logged), []byte("unit=first")) || !bytes.Contains([]byte(logged), []byte("ended=closed")) || !bytes.Contains([]byte(logged), []byte("ended=revoked")) {
		t.Errorf("the record does not tell a close from a revocation:\n%s", logged)
	}
}
