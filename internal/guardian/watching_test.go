package guardian_test

import (
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

const heartbeat = `, heartbeat: { interval: 1s, tolerance: "1" }, restart: { backoff: 1s, ceiling: 2s }`

// std: yoke:watching-a-core.01
func TestASilentCoreIsStoppedTerminatedAndBroughtBack(t *testing.T) {
	h := newHost(t, heartbeat)
	h.declare("bench-a", instancerecord.Running, "mute", nil)
	h.start(false)
	c := h.client()
	h.until(c, "bench-a", "bench-a observable", in("observable"))
	h.until(c, "bench-a", "bench-a being brought back", func(w *channel.Watched) bool {
		return w.State == "stopping" && w.Reason == "silent"
	})
	h.until(c, "bench-a", "bench-a waiting, attempt 1", func(w *channel.Watched) bool {
		return w.State == "waiting" && w.Attempt == 1 && w.Reason == "silent"
	})
	for _, said := range []string{"the instance is silent", "did not stop within its stop window"} {
		if !strings.Contains(h.said(), said) {
			t.Errorf("the guardian did not say %q:\n%s", said, h.said())
		}
	}
	for deadline := time.Now().Add(10 * time.Second); h.executed("bench-a") < 2; time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the instance was not brought back:\n%s", h.said())
		}
	}
}

// std: yoke:watching-a-core.02
func TestAnAnnouncedStopIsAnExpectedSilence(t *testing.T) {
	h := newHost(t, heartbeat)
	h.declare("bench-a", instancerecord.Running, "retire", nil)
	h.start(false)
	c := h.client()
	h.until(c, "bench-a", "bench-a stopping", in("stopping"))
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(h.said(), "the Core has stopped"); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the Core never ended:\n%s", h.said())
		}
	}
	for _, never := range []string{"silent", "did not stop within its stop window"} {
		if strings.Contains(h.said(), never) {
			t.Errorf("the guardian said %q of an announced stop:\n%s", never, h.said())
		}
	}
}
