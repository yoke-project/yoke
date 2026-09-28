package session_test

import (
	"context"
	"errors"
	"testing"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/scope"
	"github.com/yoke-project/yoke/internal/core/session"
)

// scoped declares every object named and grants those marked with a trailing "+".
func scoped(kind scope.Kind, ids ...string) func(*scope.Scope) {
	return func(sc *scope.Scope) {
		for _, id := range ids {
			granted := id[len(id)-1] == '+'
			if granted {
				id = id[:len(id)-1]
			}
			sc.Declare(kind, id)
			if granted {
				sc.Grant(kind, id)
			}
		}
	}
}

func scopeOf(parts ...func(*scope.Scope)) *scope.Scope {
	sc := &scope.Scope{}
	for _, p := range parts {
		p(sc)
	}
	return sc
}

func report(class string, severity uint32) func(e *pluginv1.Envelope) {
	return func(e *pluginv1.Envelope) {
		e.Payload = &pluginv1.Envelope_Event{Event: &pluginv1.Event{Occurrence: class, Severity: severity, Line: "a line"}}
	}
}

func activate(stream string) *pluginv1.Envelope {
	return &pluginv1.Envelope{Payload: &pluginv1.Envelope_Control{Control: &pluginv1.Control{
		Kind: &pluginv1.Control_Activate_{Activate: &pluginv1.Control_Activate{Stream: stream}}}}}
}

// refusedWith says the Core declined a sending with the code given.
func refusedWith(t *testing.T, err error, code pluginv1.Code, what string) {
	t.Helper()
	var r *session.Refused
	if !errors.As(err, &r) || r.Code != code {
		t.Errorf("%s: the Core answered %v, want a refusal %v", what, err, code)
	}
}

// std: yoke:the-granted-scope.01
func TestAnOccurrenceIsReceivedOnlyIfGranted(t *testing.T) {
	h := newHarness(t)
	st := openedWith(t, h, scopeOf(scoped(scope.Occurrence, "calibration.drift+", "head.fault")))
	st.send(t, report("calibration.drift", 90))
	st.quiet(t, 150*time.Millisecond)
	withheld := st.send(t, report("head.fault", 99))
	errorFor(t, st, "scope.withheld", withheld)
	undeclared := st.send(t, report("lamp.failure", 10))
	errorFor(t, st, "scope.undeclared", undeclared)
	st.send(t, heartbeat)
	st.quiet(t, 150*time.Millisecond)
}

// std: yoke:the-granted-scope.02
func TestTheCoreSendsNothingTheGrantDoesNotCover(t *testing.T) {
	h := newHarness(t)
	st := openedWith(t, h, scopeOf(
		scoped(scope.Command, "calibrate+", "zero"),
		scoped(scope.Query, "range+", "status"),
		scoped(scope.Stream, "station.spectra+", "station.diagnostics")))
	for _, c := range []struct {
		e    *pluginv1.Envelope
		code pluginv1.Code
	}{
		{command("zero"), pluginv1.Code_CODE_SCOPE_WITHHELD},
		{command("purge"), pluginv1.Code_CODE_SCOPE_UNDECLARED},
		{question("status"), pluginv1.Code_CODE_SCOPE_WITHHELD},
		{question("uptime"), pluginv1.Code_CODE_SCOPE_UNDECLARED},
		{activate("station.diagnostics"), pluginv1.Code_CODE_SCOPE_WITHHELD},
	} {
		_, err := h.svc.Send("sid-1", c.e)
		refusedWith(t, err, c.code, c.e.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := h.svc.Command(ctx, "sid-1", &pluginv1.Control_Command{Type: "zero"})
	refusedWith(t, err, pluginv1.Code_CODE_SCOPE_WITHHELD, "Command zero")
	_, err = h.svc.Ask(ctx, "sid-1", &pluginv1.Query_Question{Type: "uptime"})
	refusedWith(t, err, pluginv1.Code_CODE_SCOPE_UNDECLARED, "Ask uptime")
	st.quiet(t, 150*time.Millisecond)
	sent(t, h, st, command("calibrate"))
	sent(t, h, st, question("range"))
	sent(t, h, st, activate("station.spectra"))
}

// std: yoke:the-granted-scope.03
func TestEnforcementIsExactMembership(t *testing.T) {
	h := newHarness(t)
	st := openedWith(t, h, scopeOf(scoped(scope.Command, "calibrate+"), scoped(scope.Occurrence, "calibration.drift+")))
	for _, kind := range []string{"calibrate.full", "Calibrate"} {
		_, err := h.svc.Send("sid-1", command(kind))
		refusedWith(t, err, pluginv1.Code_CODE_SCOPE_UNDECLARED, kind)
	}
	for _, class := range []string{"calibration", "calibration.drift.fast"} {
		id := st.send(t, report(class, 50))
		errorFor(t, st, "scope.undeclared", id)
	}
}
