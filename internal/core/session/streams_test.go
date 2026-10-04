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

// std: yoke:a-streams-transport.05
func TestTheSessionCarriesTheActivationAndTheStop(t *testing.T) {
	h := newHarness(t)
	st := openedWith(t, h, scopeOf(scoped(scope.Stream, "station.spectra+", "station.preview")))
	instruct := func(c *pluginv1.Control) <-chan error {
		done := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ack, err := h.svc.Instruct(ctx, "sid-1", c)
			if err == nil && ack.GetOutcome() == pluginv1.Ack_OUTCOME_UNSPECIFIED {
				err = errors.New("no acknowledgement was handed back")
			}
			done <- err
		}()
		return done
	}
	handed := func(done <-chan error) error {
		t.Helper()
		select {
		case err := <-done:
			return err
		case <-time.After(3 * time.Second):
			t.Fatal("the caller was handed nothing")
			return nil
		}
	}

	activate := &pluginv1.Control{Kind: &pluginv1.Control_Activate_{Activate: &pluginv1.Control_Activate{
		Stream: "station.spectra", Transport: pluginv1.Control_Activate_TRANSPORT_ORDERED, Address: "/run/x/plugins/acquire/streams/station.spectra.sock"}}}
	done := instruct(activate)
	got, open := st.receive(t, 2*time.Second)
	if a := got.GetControl().GetActivate(); !open || a.GetStream() != "station.spectra" || a.GetTransport() != pluginv1.Control_Activate_TRANSPORT_ORDERED ||
		a.GetAddress() != "/run/x/plugins/acquire/streams/station.spectra.sock" {
		t.Fatalf("the unit received %v", got)
	}
	st.send(t, acknowledge(got.MessageId, pluginv1.Ack_OUTCOME_ACCEPTED))
	if err := handed(done); err != nil {
		t.Errorf("the activation's caller was handed %v", err)
	}

	done = instruct(&pluginv1.Control{Kind: &pluginv1.Control_Stop_{Stop: &pluginv1.Control_Stop{Stream: "station.spectra"}}})
	got, open = st.receive(t, 2*time.Second)
	if !open || got.GetControl().GetStop().GetStream() != "station.spectra" {
		t.Fatalf("the unit received %v", got)
	}
	st.send(t, acknowledge(got.MessageId, pluginv1.Ack_OUTCOME_DONE))
	if err := handed(done); err != nil {
		t.Errorf("the stop's caller was handed %v", err)
	}

	err := handed(instruct(&pluginv1.Control{Kind: &pluginv1.Control_Activate_{Activate: &pluginv1.Control_Activate{
		Stream: "station.preview", Transport: pluginv1.Control_Activate_TRANSPORT_FRAMED, Address: "/x"}}}))
	var refused *session.Refused
	if !errors.As(err, &refused) || refused.Code != pluginv1.Code_CODE_SCOPE_WITHHELD {
		t.Errorf("activating a stream not granted was answered %v", err)
	}
	st.quiet(t, 200*time.Millisecond)
}
