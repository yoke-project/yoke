package session_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/scope"
	"github.com/yoke-project/yoke/internal/core/session"
)

// opened is an open Session, the Core having taken its OPEN.
func opened(t *testing.T, h *harness) *stream {
	t.Helper()
	return openedWith(t, h, everything())
}

// openedWith is an open Session with the scope given.
func openedWith(t *testing.T, h *harness, sc *scope.Scope) *stream {
	t.Helper()
	h.admitScoped("sid-1", "acquire", time.Second, 3, sc)
	st := h.stream(t, "sid-1")
	st.send(t, open)
	for deadline := time.Now().Add(2 * time.Second); len(h.toldOf("acquire")) == 0; {
		if time.Now().After(deadline) {
			t.Fatal("the Session did not open")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return st
}

func command(kind string) *pluginv1.Envelope {
	return &pluginv1.Envelope{Payload: &pluginv1.Envelope_Control{Control: &pluginv1.Control{
		Kind: &pluginv1.Control_Command_{Command: &pluginv1.Control_Command{Type: kind}}}}}
}

func question(kind string) *pluginv1.Envelope {
	return &pluginv1.Envelope{Payload: &pluginv1.Envelope_Query{Query: &pluginv1.Query{
		Kind: &pluginv1.Query_Question_{Question: &pluginv1.Query_Question{Type: kind}}}}}
}

// sent has the Core send an envelope, and the unit receive it; it returns the envelope's identity.
func sent(t *testing.T, h *harness, st *stream, e *pluginv1.Envelope) string {
	t.Helper()
	id, err := h.svc.Send("sid-1", e)
	if err != nil {
		t.Fatalf("the Core could not send %v: %v", e, err)
	}
	if got, open := st.receive(t, 2*time.Second); !open || got.MessageId != id {
		t.Fatalf("%v arrived, want %s", got, id)
	}
	return id
}

func acknowledge(correlation string, outcome pluginv1.Ack_Outcome) func(e *pluginv1.Envelope) {
	return func(e *pluginv1.Envelope) {
		e.CorrelationId = correlation
		e.Payload = &pluginv1.Envelope_Ack{Ack: &pluginv1.Ack{Outcome: outcome, Line: "a line"}}
	}
}

func answer(correlation string, payload string) func(e *pluginv1.Envelope) {
	return func(e *pluginv1.Envelope) {
		e.CorrelationId = correlation
		e.Payload = &pluginv1.Envelope_Query{Query: &pluginv1.Query{Kind: &pluginv1.Query_Answer_{Answer: &pluginv1.Query_Answer{Payload: []byte(payload)}}}}
	}
}

func failure(correlation string) func(e *pluginv1.Envelope) {
	return func(e *pluginv1.Envelope) {
		e.CorrelationId = correlation
		e.Payload = &pluginv1.Envelope_Error{Error: &pluginv1.Error{Code: "command.unknown", Message: "a line"}}
	}
}

func occurrence(e *pluginv1.Envelope) {
	e.Payload = &pluginv1.Envelope_Event{Event: &pluginv1.Event{Occurrence: "calibration.drift", Severity: 40, Line: "a line"}}
}

// std: yoke:the-families.01
func TestEveryFamilyAUnitMayOriginateIsReceived(t *testing.T) {
	h := newHarness(t)
	st := opened(t, h)
	c := sent(t, h, st, command("calibrate"))
	q := sent(t, h, st, question("range"))
	st.send(t, heartbeat)
	st.send(t, occurrence)
	st.send(t, failure(""))
	st.send(t, acknowledge(c, pluginv1.Ack_OUTCOME_DONE))
	st.send(t, answer(q, "42"))
	st.send(t, failure(c))
	st.quiet(t, 200*time.Millisecond)
	st.send(t, closing)
	st.closedWithNothing(t)
	if got := h.sessionOf("acquire"); !slices.Equal(got, []string{"unit.SessionOpened{}", "unit.SessionEnded{Withdrawn:false}"}) {
		t.Errorf("the machine was told %v", got)
	}
}

// std: yoke:the-families.02
func TestAFamilyTheCoreOriginatesIsRefusedFromAUnit(t *testing.T) {
	h := newHarness(t)
	st := opened(t, h)
	for _, fill := range []func(e *pluginv1.Envelope){
		func(e *pluginv1.Envelope) { e.Payload = command("calibrate").Payload },
		func(e *pluginv1.Envelope) {
			e.Payload = &pluginv1.Envelope_Control{Control: &pluginv1.Control{Kind: &pluginv1.Control_Activate_{Activate: &pluginv1.Control_Activate{Stream: "station.data"}}}}
		},
		func(e *pluginv1.Envelope) { e.Payload = question("range").Payload },
		func(e *pluginv1.Envelope) {
			e.Payload = &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Revoked_{Revoked: &pluginv1.SessionMessage_Revoked{}}}}
		},
	} {
		id := st.send(t, fill)
		errorFor(t, st, "session.direction", id)
	}
	st.send(t, heartbeat)
	st.quiet(t, 150*time.Millisecond)
}

// std: yoke:the-families.03
func TestTheCoreOriginatesItsOwnFamiliesAndNoneOfTheUnits(t *testing.T) {
	h := newHarness(t)
	st := opened(t, h)
	session := func(m *pluginv1.SessionMessage) *pluginv1.Envelope {
		return &pluginv1.Envelope{Payload: &pluginv1.Envelope_Session{Session: m}}
	}
	for _, e := range []*pluginv1.Envelope{
		{Payload: &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: 90}}},
		{Payload: &pluginv1.Envelope_Ack{Ack: &pluginv1.Ack{Outcome: pluginv1.Ack_OUTCOME_DONE}}},
		{Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: 1}}},
		{Payload: &pluginv1.Envelope_Event{Event: &pluginv1.Event{Occurrence: "calibration.drift"}}},
		{Payload: &pluginv1.Envelope_Query{Query: &pluginv1.Query{Kind: &pluginv1.Query_Answer_{Answer: &pluginv1.Query_Answer{}}}}},
		session(&pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Open_{Open: &pluginv1.SessionMessage_Open{}}}),
		session(&pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Close_{Close: &pluginv1.SessionMessage_Close{}}}),
		session(&pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Revoked_{Revoked: &pluginv1.SessionMessage_Revoked{}}}),
	} {
		if _, err := h.svc.Send("sid-1", e); err == nil {
			t.Errorf("the Core sent %v", e)
		}
	}
	st.quiet(t, 150*time.Millisecond)
	c := sent(t, h, st, command("calibrate"))
	q := sent(t, h, st, question("range"))
	sent(t, h, st, &pluginv1.Envelope{Payload: &pluginv1.Envelope_Error{Error: &pluginv1.Error{Code: "command.unknown"}}})
	if c == q {
		t.Errorf("two messages share the identity %s", c)
	}
}

// std: yoke:the-families.04
func TestAnAcknowledgementAnswersACommandAndAnAnswerAQuestion(t *testing.T) {
	h := newHarness(t)
	st := opened(t, h)
	c := sent(t, h, st, command("calibrate"))
	q := sent(t, h, st, question("range"))
	wrong := st.send(t, acknowledge(q, pluginv1.Ack_OUTCOME_DONE))
	errorFor(t, st, "session.correlation.unknown", wrong)
	wrong = st.send(t, answer(c, "42"))
	errorFor(t, st, "session.correlation.unknown", wrong)
	st.send(t, acknowledge(c, pluginv1.Ack_OUTCOME_DONE))
	st.send(t, answer(q, "42"))
	st.quiet(t, 200*time.Millisecond)
}

// std: yoke:the-families.05
func TestAnAcceptanceIsFollowedByAtMostOneFinalAnswer(t *testing.T) {
	h := newHarness(t)
	st := opened(t, h)
	first := sent(t, h, st, command("calibrate"))
	second := sent(t, h, st, command("calibrate"))
	third := sent(t, h, st, command("calibrate"))
	fourth := sent(t, h, st, command("calibrate"))

	st.send(t, acknowledge(first, pluginv1.Ack_OUTCOME_ACCEPTED))
	st.send(t, acknowledge(first, pluginv1.Ack_OUTCOME_DONE))
	st.quiet(t, 150*time.Millisecond)
	again := st.send(t, acknowledge(first, pluginv1.Ack_OUTCOME_DONE))
	errorFor(t, st, "session.correlation.unknown", again)

	st.send(t, acknowledge(second, pluginv1.Ack_OUTCOME_ACCEPTED))
	twice := st.send(t, acknowledge(second, pluginv1.Ack_OUTCOME_ACCEPTED))
	errorFor(t, st, "session.message.malformed", twice)

	st.send(t, acknowledge(third, pluginv1.Ack_OUTCOME_FAILED))
	after := st.send(t, acknowledge(third, pluginv1.Ack_OUTCOME_ACCEPTED))
	errorFor(t, st, "session.correlation.unknown", after)

	none := st.send(t, acknowledge(fourth, pluginv1.Ack_OUTCOME_UNSPECIFIED))
	errorFor(t, st, "session.message.malformed", none)
}

// std: yoke:the-families.06
func TestWhoeverAskedIsHandedTheFirstAnswer(t *testing.T) {
	h := newHarness(t)
	st := opened(t, h)
	type result struct {
		ack    *pluginv1.Ack
		answer *pluginv1.Query_Answer
		err    error
	}
	issue := func(wait time.Duration, e *pluginv1.Envelope) (<-chan result, string) {
		t.Helper()
		done := make(chan result, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), wait)
			defer cancel()
			if c := e.GetControl().GetCommand(); c != nil {
				ack, err := h.svc.Command(ctx, "sid-1", c)
				done <- result{ack: ack, err: err}
				return
			}
			a, err := h.svc.Ask(ctx, "sid-1", e.GetQuery().GetQuestion())
			done <- result{answer: a, err: err}
		}()
		got, open := st.receive(t, 2*time.Second)
		if !open {
			t.Fatal("the stream closed")
		}
		return done, got.MessageId
	}
	handed := func(done <-chan result) result {
		t.Helper()
		select {
		case r := <-done:
			return r
		case <-time.After(3 * time.Second):
			t.Fatal("the caller was handed nothing")
			return result{}
		}
	}

	done, first := issue(2*time.Second, command("calibrate"))
	st.send(t, acknowledge(first, pluginv1.Ack_OUTCOME_ACCEPTED))
	if r := handed(done); r.err != nil || r.ack.GetOutcome() != pluginv1.Ack_OUTCOME_ACCEPTED {
		t.Errorf("the first caller was handed %v, %v", r.ack, r.err)
	}
	st.send(t, acknowledge(first, pluginv1.Ack_OUTCOME_DONE))
	st.quiet(t, 150*time.Millisecond)
	if log := h.log.String(); !strings.Contains(log, "correlation="+first) || !strings.Contains(log, "no caller") {
		t.Errorf("the final answer that reached no caller was not recorded:\n%s", log)
	}

	done, second := issue(2*time.Second, command("zero"))
	st.send(t, acknowledge(second, pluginv1.Ack_OUTCOME_DONE))
	if r := handed(done); r.err != nil || r.ack.GetOutcome() != pluginv1.Ack_OUTCOME_DONE {
		t.Errorf("the second caller was handed %v, %v", r.ack, r.err)
	}

	done, q := issue(2*time.Second, question("range"))
	st.send(t, answer(q, "42"))
	if r := handed(done); r.err != nil || string(r.answer.GetPayload()) != "42" {
		t.Errorf("the question's caller was handed %v, %v", r.answer, r.err)
	}

	done, third := issue(200*time.Millisecond, command("calibrate"))
	if r := handed(done); !errors.Is(r.err, context.DeadlineExceeded) {
		t.Errorf("the third caller was handed %v, %v", r.ack, r.err)
	}
	st.send(t, acknowledge(third, pluginv1.Ack_OUTCOME_DONE))
	st.quiet(t, 150*time.Millisecond)
}

// std: yoke:the-families.08
func TestAnErrorAnsweringTheCoresMessageClosesTheExchange(t *testing.T) {
	h := newHarness(t)
	st := opened(t, h)
	done := make(chan error, 1)
	started := time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := h.svc.Ask(ctx, "sid-1", &pluginv1.Query_Question{Type: "range"})
		done <- err
	}()
	got, open := st.receive(t, 2*time.Second)
	if !open {
		t.Fatal("the stream closed")
	}
	fail := func(e *pluginv1.Envelope) {
		e.CorrelationId = got.MessageId
		e.Payload = &pluginv1.Envelope_Error{Error: &pluginv1.Error{Code: "instrument.busy", Message: "the lamp is warming"}}
	}
	st.send(t, fail)
	var err error
	select {
	case err = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the caller was handed nothing")
	}
	var failed *session.UnitFailed
	if !errors.As(err, &failed) || failed.Code != "instrument.busy" || failed.Message != "the lamp is warming" || time.Since(started) > time.Second {
		t.Errorf("after %v the caller was handed %v", time.Since(started), err)
	}
	late := st.send(t, answer(got.MessageId, "late"))
	refusal, open := st.receive(t, 2*time.Second)
	if !open || refusal.GetError().GetCode() != "session.correlation.unknown" || refusal.CorrelationId != late {
		t.Errorf("the answer after the error was answered %v", refusal)
	}
}
