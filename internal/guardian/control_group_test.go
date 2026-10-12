package guardian_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

// buildCore builds yoke-core from this tree, while the PATH still holds the toolchain.
func buildCore(t *testing.T) string {
	t.Helper()
	core := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", core, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	return core
}

// describe replaces the installed descriptor's units with those given, each program written under
// components/ with its digest, so that the record pins the descriptor as it now is.
func (h *host) describe(units string, programs map[string]string) {
	h.t.Helper()
	bundle := filepath.Join(h.o.Config.InstallRoot, "com.yoke.station", "2.1.0", "bundle")
	for name, body := range programs {
		os.WriteFile(filepath.Join(bundle, "components", name), []byte(body), 0o755)
		units = strings.ReplaceAll(units, "{"+name+"}", fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(body))))
	}
	i := strings.Index(string(h.descriptor), "units:\n")
	h.descriptor = append(h.descriptor[:i:i], []byte("units:\n"+units)...)
	os.WriteFile(filepath.Join(bundle, "yoke.yaml"), h.descriptor, 0o644)
}

func inSlice(pid int, name string) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cgroup")
	return err == nil && strings.Contains(string(data), "/"+engine.Slice(name)+"/")
}

func gone(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return true
	}
	// A zombie has ended; only its parent has not read that yet.
	fields := strings.Fields(string(data))
	return len(fields) > 2 && fields[2] == "Z"
}

func eventually(t *testing.T, what string, within time.Duration, holds func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); !holds(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s never held within %v", what, within)
		}
	}
}

// std: yoke:the-control-group.01
func TestTheCoreRunsInItsSliceAndWhatEscapedEndsWithIt(t *testing.T) {
	core := buildCore(t)
	h := newHost(t, "")
	h.delegate()
	h.o.Config.Core = core
	name := "cg-" + strconv.Itoa(os.Getpid())
	escaped := filepath.Join(h.witness, "escaped")
	h.describe("  escape: { kind: oneshot, exec: components/escape, digest: \"{escape}\" }\n", map[string]string{
		"escape": "#!/bin/sh\n/usr/bin/setsid /bin/sh -c 'trap \"\" TERM; echo $$ > " + escaped + "; while :; do /bin/sleep 1; done' &\n/bin/sleep 300\n",
	})
	h.declare(name, instancerecord.Running, "", nil)
	h.start(false)
	c := h.client()
	w := h.until(c, name, name+" observable", in("observable"))
	var pid int
	eventually(t, "the escaped process to start", 20*time.Second, func() bool {
		data, err := os.ReadFile(escaped)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil && pid > 0
	})
	t.Cleanup(func() { syscall.Kill(pid, syscall.SIGKILL) })
	if !inSlice(int(w.Pid), name) || !inSlice(pid, name) {
		t.Errorf("the Core %d and the escaped process %d are not both in %s", w.Pid, pid, engine.Slice(name))
	}
	if strings.Contains(h.said(), "no_cgroup_delegation") {
		t.Errorf("a host with a slice was reported without delegation:\n%s", h.said())
	}
	syscall.Kill(int(w.Pid), syscall.SIGKILL)
	eventually(t, "the escaped process to end with the Core", 20*time.Second, func() bool { return gone(pid) })
}

// std: yoke:the-control-group.02
func TestAContainerTheCoreLaunchedIsInTheSliceAndEndsWithIt(t *testing.T) {
	enginetest.Not(t, enginetest.Docker)
	core := buildCore(t)
	dir := t.TempDir()
	build := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "unit"), ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the fixture does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY unit /unit\nENV "+serve+"=1\nENTRYPOINT [\"/unit\"]\n"), 0o644)
	image := enginetest.Image(t, dir, "yoke-l3-slice")
	s := enginetest.Serve(t)
	h := newHost(t, "")
	h.delegate()
	h.o.Config.Core, h.o.Config.Engine = core, "unix://"+s.Socket
	name := "cg-" + strconv.Itoa(os.Getpid()) + "-c"
	t.Cleanup(func() { enginetest.Remove(enginetest.Labelled("dev.yoke-project.instance=" + name)...) })
	h.descriptor = []byte(strings.Replace(string(h.descriptor), "startup_window: 5s", "startup_window: 30s", 1))
	h.describe("  keeper: { kind: oneshot, image: \""+image+"\" }\n", nil)
	h.declare(name, instancerecord.Running, "", nil)
	h.start(false)
	c := h.client()
	w := h.until(c, name, name+" observable", in("observable"))
	var container string
	t.Cleanup(func() {
		if t.Failed() {
			t.Log(h.said())
		}
	})
	eventually(t, "the container to run", time.Minute, func() bool {
		found := enginetest.Labelled("dev.yoke-project.instance=" + name)
		if len(found) == 1 && enginetest.Running(found[0]) {
			container = found[0]
			return true
		}
		return false
	})
	out, _ := enginetest.CLI("inspect", "--format", "{{.State.Pid}}", container).Output()
	if pid, _ := strconv.Atoi(strings.TrimSpace(string(out))); !inSlice(pid, name) {
		t.Errorf("the container's process %d is not in %s", pid, engine.Slice(name))
	}
	syscall.Kill(int(w.Pid), syscall.SIGKILL)
	eventually(t, "the container to end with the Core", 30*time.Second, func() bool { return !enginetest.Running(container) })
}

// std: yoke:the-control-group.03
func TestWithoutDelegationTheFallbackIsWeakerAndSignalsTheProcessGroup(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Running, "parent", nil)
	done := h.start(false)
	c := h.client()
	w := h.until(c, "bench-a", "bench-a observable", in("observable"))
	said := h.said()
	if i, j := strings.Index(said, "confinement.no_cgroup_delegation"), strings.Index(said, "the Core is executed"); i < 0 || i > j {
		t.Errorf("the weaker arrangement was not said before the Core was executed:\n%s", said)
	}
	if pgid, err := syscall.Getpgid(int(w.Pid)); err != nil || pgid != int(w.Pid) {
		t.Errorf("the Core %d does not lead its process group: %d %v", w.Pid, pgid, err)
	}
	data, _ := os.ReadFile(filepath.Join(h.witness, "bench-a.child"))
	child, _ := strconv.Atoi(string(data))
	t.Cleanup(func() { syscall.Kill(child, syscall.SIGKILL) })
	h.setDesired("bench-a", instancerecord.Stopped)
	if _, err := c.Reconcile(t.Context(), &channel.ReconcileRequest{Name: "bench-a"}); err != nil {
		t.Fatal(err)
	}
	ended(t, done, 20*time.Second)
	eventually(t, "the Core's child to end with it", 10*time.Second, func() bool { return gone(child) })
}
