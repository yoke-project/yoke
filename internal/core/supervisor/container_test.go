package supervisor_test

import (
	"context"
	"errors"
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
	refuses  map[string]bool // containers the engine will not remove
	events   chan engine.Event
	outputs  map[string][2]io.Writer
	closers  map[string]chan struct{}
	count    int

	away     bool                     // the engine cannot be followed, nor asked what it holds
	watch    []string                 // "events" for each time it is followed, "list" for each time it is asked what it holds
	of       map[string]engine.Launch // what each container was created with
	exited   map[string]int           // the status each container ended with
	gone     map[string]bool          // containers the engine no longer holds, though nobody removed them
	foreign  []engine.Found           // containers under the label that nobody here launched
	returned chan struct{}            // a notice that the engine returned
}

func newContainers() *containers {
	return &containers{removed: map[string]bool{}, outputs: map[string][2]io.Writer{}, closers: map[string]chan struct{}{},
		of: map[string]engine.Launch{}, exited: map[string]int{}, gone: map[string]bool{}, returned: make(chan struct{}, 1)}
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
	c.of[id] = l
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

// die ends a container: its output closes, and the engine reports its end while it is followed.
func (c *containers) die(id string, status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	done, open := c.closers[id]
	delete(c.closers, id)
	if !open {
		return
	}
	close(done)
	c.exited[id] = status
	if c.events != nil {
		c.events <- engine.Event{ID: id, Action: "die", ExitCode: status}
	}
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
	defer c.mu.Unlock()
	if c.refuses[id] {
		return fmt.Errorf("the engine would not remove %s: 500 removal of container %s is already in progress", id, id)
	}
	c.removed[id] = true
	return nil
}

func (c *containers) Events(ctx context.Context, instance string) (<-chan engine.Event, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watch = append(c.watch, "events")
	if c.away {
		return nil, &engine.Unreachable{Address: "unix:///engine.sock", Err: errors.New("connection refused")}
	}
	c.events = make(chan engine.Event, 16)
	return c.events, nil
}

func (c *containers) List(_ context.Context, instance string) ([]engine.Found, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watch = append(c.watch, "list")
	if c.away {
		return nil, &engine.Unreachable{Address: "unix:///engine.sock", Err: errors.New("connection refused")}
	}
	var found []engine.Found
	for id, l := range c.of {
		if c.removed[id] || c.gone[id] {
			continue
		}
		f := engine.Found{ID: id, Unit: l.Unit, Incarnation: l.Incarnation, Running: true}
		if status, ended := c.exited[id]; ended {
			f.Running, f.ExitCode = false, status
		}
		found = append(found, f)
	}
	for _, f := range c.foreign {
		if !c.removed[f.ID] {
			found = append(found, f)
		}
	}
	return found, nil
}

func (c *containers) Returned(context.Context) (<-chan struct{}, error) { return c.returned, nil }

// goAway is the engine going away: its event stream ends, and it cannot be followed again until it comes
// back.
func (c *containers) goAway() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.away = true
	if c.events != nil {
		close(c.events)
		c.events = nil
	}
}

// comeBack lets the engine be followed again; nothing is told.
func (c *containers) comeBack() {
	c.mu.Lock()
	c.away = false
	c.mu.Unlock()
}

// notice is the engine's return, as a notice.
func (c *containers) notice() { c.returned <- struct{}{} }

// endQuietly ends a container while nobody follows the engine: it is held, ended with its status.
func (c *containers) endQuietly(id string, status int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if done, open := c.closers[id]; open {
		close(done)
		delete(c.closers, id)
	}
	c.exited[id] = status
}

// vanish is a container the engine no longer holds.
func (c *containers) vanish(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if done, open := c.closers[id]; open {
		close(done)
		delete(c.closers, id)
	}
	c.gone[id] = true
}

// write is a line a running container writes on its output, to whoever is attached last.
func (c *containers) write(id, line string) {
	c.mu.Lock()
	out := c.outputs[id]
	c.mu.Unlock()
	io.WriteString(out[0], line+"\n")
}

func (c *containers) watched() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.watch...)
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
