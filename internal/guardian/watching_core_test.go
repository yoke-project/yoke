package guardian_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

// std: yoke:watching-a-core.03
func TestAWedgedCoreIsBroughtBackAsANewIncarnation(t *testing.T) {
	h := newHost(t, `, heartbeat: { interval: 1s, tolerance: "2" }, restart: { backoff: 1s, ceiling: 2s }`)
	core := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", core, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	h.o.Config.Core = core
	h.declare("bench-a", instancerecord.Running, "", nil)
	h.start(false)
	c := h.client()
	first := h.until(c, "bench-a", "bench-a observable", in("observable")).Pid
	time.Sleep(3500 * time.Millisecond)
	if w := h.until(c, "bench-a", "bench-a still observable", in("observable")); w.Pid != first {
		t.Fatalf("a Core that kept saying it is alive was replaced: %d, then %d", first, w.Pid)
	}
	syscall.Kill(int(first), syscall.SIGSTOP)
	t.Cleanup(func() { syscall.Kill(int(first), syscall.SIGKILL) })
	h.until(c, "bench-a", "a new Core observable", func(w *channel.Watched) bool {
		return w.State == "observable" && w.Pid != 0 && w.Pid != first
	})
	for _, said := range []string{"the instance is silent", "brought back as a new incarnation"} {
		if !strings.Contains(h.said(), said) {
			t.Errorf("the guardian did not say %q:\n%s", said, h.said())
		}
	}
}
