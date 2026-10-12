package guardian_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

// std: yoke:the-guardian.10
func TestAGuardianStartsACoreBuiltFromThisTree(t *testing.T) {
	h := newHost(t, "")
	core := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", core, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	s := enginetest.Serve(t)
	h.o.Config.Core, h.o.Config.Engine = core, "unix://"+s.Socket
	// A unit naming an image, not started automatically: the Core reaches the engine for it at its start.
	image := "localhost/yoke-l3-guardian@sha256:" + strings.Repeat("ab", 32)
	h.descriptor = append(h.descriptor, []byte("  keeper: { kind: oneshot, image: \""+image+"\", autostart: false }\n")...)
	os.WriteFile(filepath.Join(h.o.Config.InstallRoot, "com.yoke.station", "2.1.0", "bundle", "yoke.yaml"), h.descriptor, 0o644)
	h.declare("bench-a", instancerecord.Running, "", nil)

	done := h.start(false)
	c := h.client()
	h.until(c, "bench-a", "bench-a observable", in("observable"))
	if !strings.Contains(h.said(), "the container engine is reached") {
		t.Errorf("the Core did not reach the engine it was handed:\n%s", h.said())
	}
	stamp, err := os.ReadFile(filepath.Join(instancerecord.StateDir(h.instances, "bench-a"), "version"))
	if err != nil || strings.TrimSpace(string(stamp)) != "2.1.0" {
		t.Errorf("the stamp says %q: %v", stamp, err)
	}
	h.setDesired("bench-a", instancerecord.Stopped)
	if _, err := c.Reconcile(context.Background(), &channel.ReconcileRequest{Name: "bench-a"}); err != nil {
		t.Fatal(err)
	}
	if err := ended(t, done, 30*time.Second); err != nil {
		t.Errorf("the guardian exited %v:\n%s", err, h.said())
	}
}
