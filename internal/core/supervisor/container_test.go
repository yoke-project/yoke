package supervisor_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// containers is an engine as each case scripts it: what a container writes, how it ends, and whether it
// heeds SIGTERM — recording every act it is asked for.
type containers struct {
	mu       sync.Mutex
	absent   bool
	says     string   // the line every container writes once started
	ends     []int    // the status each successive container ends with on its own; none keeps running
	deaf     bool     // a container ignores SIGTERM
	asked    []string // the acts, in order: "create <unit>", "attach <id>", "start <id>", "signal <id> <sig>", "remove <id>"
	launched []engine.Launch
	removed  map[string]bool
	events   chan engine.Event
	outputs  map[string][2]io.Writer
	closers  map[string]chan struct{}
	count    int
}

func newContainers() *containers {
	return &containers{removed: map[string]bool{}, outputs: map[string][2]io.Writer{}, closers: map[string]chan struct{}{}}
}

func (c *containers) record(act string) {
	c.mu.Lock()
	c.asked = append(c.asked, act)
	c.mu.Unlock()
}

func (c *containers) acts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.asked...)
}

func (c *containers) Create(_ context.Context, l engine.Launch) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.absent {
		c.asked = append(c.asked, "create "+l.Unit+" absent")
		return "", &engine.Absent{Image: l.Image}
	}
	c.count++
	id := fmt.Sprintf("c%d", c.count)
	c.asked = append(c.asked, "create "+l.Unit)
	c.launched = append(c.launched, l)
	c.closers[id] = make(chan struct{})
	return id, nil
}

func (c *containers) Attach(_ context.Context, id string, stdout, stderr io.Writer) (<-chan struct{}, error) {
	c.record("attach " + id)
	c.mu.Lock()
	c.outputs[id] = [2]io.Writer{stdout, stderr}
	done := c.closers[id]
	c.mu.Unlock()
	return done, nil
}

func (c *containers) Start(_ context.Context, id string) error {
	c.record("start " + id)
	c.mu.Lock()
	out, says := c.outputs[id], c.says
	var end *int
	if len(c.ends) > 0 {
		e := c.ends[0]
		c.ends = c.ends[1:]
		end = &e
	}
	c.mu.Unlock()
	if says != "" {
		io.WriteString(out[0], says+"\n")
	}
	if end != nil {
		go c.die(id, *end)
	}
	return nil
}

// die ends a container: its output closes, and the engine reports its end.
func (c *containers) die(id string, status int) {
	c.mu.Lock()
	done, open := c.closers[id]
	delete(c.closers, id)
	events := c.events
	c.mu.Unlock()
	if !open {
		return
	}
	close(done)
	events <- engine.Event{ID: id, Action: "die", ExitCode: status}
}

func (c *containers) Signal(_ context.Context, id, signal string) error {
	c.record("signal " + id + " " + signal)
	if signal == "SIGKILL" || !c.deaf {
		go c.die(id, 137)
	}
	return nil
}

func (c *containers) Remove(_ context.Context, id string) error {
	c.record("remove " + id)
	c.mu.Lock()
	c.removed[id] = true
	c.mu.Unlock()
	return nil
}

func (c *containers) Events(ctx context.Context, instance string) (<-chan engine.Event, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = make(chan engine.Event, 16)
	return c.events, nil
}

func (c *containers) wasRemoved(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removed[id]
}

// inContainers is a supervisor whose engine is c, or none when c is nil.
func inContainers(t *testing.T, c *containers, policy supervisor.Policy) (*supervisor.Supervisor, *output) {
	t.Helper()
	out := &output{}
	cfg := supervisor.Config{Root: t.TempDir(), Policy: policy, Instance: "bench",
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: out}
	if c != nil {
		cfg.Containers = c
	}
	s := supervisor.New(cfg)
	t.Cleanup(func() { s.Stop() })
	return s, out
}

func imaged(id string, kind unit.Kind) supervisor.Unit {
	return supervisor.Unit{ID: id, Kind: kind, Image: "localhost/fixture@sha256:" + strings.Repeat("cd", 32),
		Args: []string{"--once"}, Env: map[string]string{"DECLARED": "its own"}}
}

// std: yoke:the-container-backend.01
func TestAUnitNamingAnImageIsLaunchedInAContainer(t *testing.T) {
	c := newContainers()
	c.says = "from inside"
	s, out := inContainers(t, c, fast())
	u := imaged("calibrate", unit.Oneshot)
	s.Launch(u)
	until(t, "the container's line", 3*time.Second, func() bool { return slices.Contains(out.all(), "calibrate#1 from inside") })
	if got := c.acts(); len(got) < 3 || got[0] != "create calibrate" || got[1] != "attach c1" || got[2] != "start c1" {
		t.Errorf("the engine was asked %v", got)
	}
	c.mu.Lock()
	l := c.launched[0]
	c.mu.Unlock()
	status := s.Status("calibrate")
	want := s.Environment(u, status.Token)
	if l.Image != u.Image || !slices.Equal(l.Args, u.Args) || !slices.Equal(l.Env, want) || l.Directory != s.Root() ||
		l.Instance != "bench" || l.Unit != "calibrate" || l.Incarnation != 1 || l.UID != os.Getuid() || l.GID != os.Getgid() {
		t.Errorf("created %+v, want the environment %v", l, want)
	}
	if status.State != unit.Running {
		t.Errorf("the unit is %s", status.State)
	}
}

// std: yoke:the-container-backend.02
func TestTheEnginesReportOfAnEndIsTheUnitsExit(t *testing.T) {
	c := newContainers()
	c.ends = []int{0, 1}
	s, _ := inContainers(t, c, fast())
	s.Launch(imaged("first", unit.Oneshot))
	until(t, "the first to complete", 3*time.Second, func() bool { return s.Status("first").State == unit.Completed })
	second := imaged("second", unit.Oneshot)
	second.RestartOnFailure = true
	s.Launch(second)
	until(t, "the second to fail", 3*time.Second, func() bool {
		st := s.Status("second")
		return st.State == unit.Failed && st.Waiting
	})
	until(t, "both containers removed", 3*time.Second, func() bool { return c.wasRemoved("c1") && c.wasRemoved("c2") })
}

// std: yoke:the-container-backend.03
func TestAnAbsentImageIsAFaultTheRestartPolicyWaitsOn(t *testing.T) {
	c := newContainers()
	c.absent = true
	s, _ := inContainers(t, c, fast())
	u := imaged("calibrate", unit.Interface)
	s.Launch(u)
	until(t, "the unit to fail and wait", 3*time.Second, func() bool {
		st := s.Status("calibrate")
		return st.State == unit.Failed && st.Waiting
	})
	if st := s.Status("calibrate"); !strings.Contains(st.Failure, u.Image) {
		t.Errorf("the failure says %q", st.Failure)
	}
	c.mu.Lock()
	c.absent = false
	c.mu.Unlock()
	until(t, "a later attempt to start", 3*time.Second, func() bool { return slices.Contains(c.acts(), "start c1") })
	for _, act := range c.acts() {
		if !strings.HasPrefix(act, "create") && !strings.HasPrefix(act, "attach") && !strings.HasPrefix(act, "start") {
			t.Errorf("the engine was asked %s", act)
		}
	}
}

// std: yoke:the-container-backend.04
func TestAStopSignalsWaitsEndsAndRemoves(t *testing.T) {
	c := newContainers()
	c.deaf = true
	policy := fast()
	s, _ := inContainers(t, c, policy)
	s.Launch(imaged("panel", unit.Interface))
	until(t, "the container to start", 3*time.Second, func() bool { return slices.Contains(c.acts(), "start c1") })
	began := time.Now()
	if err := s.StopUnit("panel"); err != nil {
		t.Fatal(err)
	}
	until(t, "the unit to stop", 3*time.Second, func() bool { return s.Status("panel").State == unit.Stopped })
	acts := c.acts()
	term, kill := slices.Index(acts, "signal c1 SIGTERM"), slices.Index(acts, "signal c1 SIGKILL")
	if term < 0 || kill < term || time.Since(began) < policy.StopWindow {
		t.Errorf("the engine was asked %v within %v", acts, time.Since(began))
	}
	if !c.wasRemoved("c1") {
		t.Error("the container was not removed")
	}
}

// std: yoke:the-container-backend.05
func TestWithNoEngineALaunchInAContainerIsAFault(t *testing.T) {
	s, _ := inContainers(t, nil, fast())
	s.Launch(imaged("calibrate", unit.Interface))
	s.Launch(declared(t, "beside", unit.Oneshot, "exit-0"))
	until(t, "the containerised unit to fail and wait", 3*time.Second, func() bool {
		st := s.Status("calibrate")
		return st.State == unit.Failed && st.Waiting
	})
	if st := s.Status("calibrate"); !strings.Contains(st.Failure, "no container engine") {
		t.Errorf("the failure says %q", st.Failure)
	}
	until(t, "the host unit to complete", 3*time.Second, func() bool { return s.Status("beside").State == unit.Completed })
}
