package supervisor_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// The test binary is also every unit these tests launch: this variable says which part it plays.
const role = "YOKE_TEST_UNIT_ROLE"

func TestMain(m *testing.M) {
	switch os.Getenv(role) {
	case "":
		os.Exit(m.Run())
	case "exit-0":
		os.Exit(0)
	case "exit-1":
		os.Exit(1)
	case "serve":
		time.Sleep(time.Hour)
	case "exit-soon":
		time.Sleep(200 * time.Millisecond)
		if leaving := os.Getenv("YOKE_TEST_LEAVING"); leaving != "" {
			os.WriteFile(leaving, nil, 0o600)
		}
		os.Exit(0)
	case "write":
		fmt.Println("first line")
		fmt.Println("second line")
		fmt.Fprintln(os.Stderr, "on the error stream")
		os.Exit(1)
	case "tell":
		// What the process was handed, as lines its output carries.
		for _, name := range []string{"YOKE_PLUGIN", "YOKE_UNIT", "YOKE_SOCKET", "YOKE_BIND", "YOKE_TOKEN", "DECLARED"} {
			value, set := os.LookupEnv(name)
			fmt.Printf("env %s=%s set=%v\n", name, value, set)
		}
		fmt.Printf("args %d %q\n", len(os.Args)-1, os.Args[1:])
		os.Exit(0)
	case "fork":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), role+"=serve")
		child.Start()
		os.WriteFile(os.Getenv("YOKE_TEST_CHILD_PID"), []byte(fmt.Sprint(child.Process.Pid)), 0o600)
		time.Sleep(time.Hour)
	case "stubborn":
		signal.Ignore(syscall.SIGTERM)
		os.WriteFile(os.Getenv("YOKE_TEST_IGNORING"), nil, 0o600)
		time.Sleep(time.Hour)
	}
	os.Exit(2)
}

// self is the test binary and its digest: the executable every unit here is.
func self(t *testing.T) (string, string) {
	t.Helper()
	data, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return os.Args[0], "sha256:" + hex.EncodeToString(sum[:])
}

// declared is a unit playing part, of kind, with the test binary's own digest.
func declared(t *testing.T, id string, kind unit.Kind, part string) supervisor.Unit {
	exec, digest := self(t)
	return supervisor.Unit{ID: id, Kind: kind, Plugin: "com.yoke.test", Exec: exec, Digest: digest,
		Env: map[string]string{role: part}}
}

// output collects what units write, safely across goroutines.
type output struct {
	mu    sync.Mutex
	lines []string
}

func (o *output) Line(unitID string, incarnation int, line string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.lines = append(o.lines, fmt.Sprintf("%s#%d %s", unitID, incarnation, line))
}

func (o *output) all() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.lines...)
}

// fast is a policy of short windows, so a test waits milliseconds rather than the deployment's figures.
func fast() supervisor.Policy {
	return supervisor.Policy{
		StartupWindow: 5 * time.Second, StopWindow: 300 * time.Millisecond,
		Backoff: 50 * time.Millisecond, Ceiling: 200 * time.Millisecond, StabilityWindow: 10 * time.Second,
	}
}

func started(t *testing.T, policy supervisor.Policy) (*supervisor.Supervisor, *output) {
	t.Helper()
	out := &output{}
	s := supervisor.New(supervisor.Config{
		Root: t.TempDir(), Policy: policy,
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: out,
	})
	t.Cleanup(func() { s.Stop() })
	return s, out
}

// until waits for the condition, or fails saying what was last seen.
func until(t *testing.T, what string, within time.Duration, holds func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if holds() {
			return
		}
	}
	t.Fatalf("%s did not happen within %v", what, within)
}

// std: yoke:the-supervisor.08
func TestTheIdentityIsVerifiedOnTheDescriptorThenExecuted(t *testing.T) {
	s, _ := started(t, fast())
	good := declared(t, "good", unit.Interface, "serve")
	s.Launch(good)
	until(t, "the unit with its digest running", 5*time.Second, func() bool { return s.Status("good").State == unit.Running })

	bad := declared(t, "bad", unit.Interface, "serve")
	bad.Digest = "sha256:" + strings.Repeat("0", 64)
	s.Launch(bad)
	until(t, "the unit whose digest disagrees failing", 5*time.Second, func() bool { return s.Status("bad").State == unit.Failed })
	status := s.Status("bad")
	if status.Incarnation != 0 {
		t.Errorf("a unit whose digest disagreed was started, as incarnation %d", status.Incarnation)
	}
	if !strings.Contains(status.Failure, strings.Repeat("0", 64)) {
		t.Errorf("the failure does not name the digest that disagreed: %q", status.Failure)
	}
}

// std: yoke:the-supervisor.09
func TestTheProcessIsHandedWhatItNeeds(t *testing.T) {
	s, out := started(t, fast())
	for _, kind := range []unit.Kind{unit.Plugin, unit.Oneshot} {
		u := declared(t, "tell-"+string(kind), kind, "tell")
		u.Env["DECLARED"] = "its own"
		u.Args = []string{"one argument, with spaces"}
		s.Launch(u)
	}
	until(t, "both units telling what they received", 5*time.Second, func() bool {
		return strings.Count(strings.Join(out.all(), "\n"), "args ") >= 2
	})
	root := s.Root()
	for _, kind := range []unit.Kind{unit.Plugin, unit.Oneshot} {
		id := "tell-" + string(kind)
		var said []string
		for _, line := range out.all() {
			if strings.HasPrefix(line, id+"#") {
				said = append(said, line[strings.Index(line, " ")+1:])
			}
		}
		told := strings.Join(said, "\n")
		for _, want := range []string{
			"env YOKE_UNIT=" + id + " set=true",
			"env YOKE_SOCKET=" + filepath.Join(root, "plugin.sock") + " set=true",
			"env YOKE_BIND=" + filepath.Join(root, "plugins", id+".sock") + " set=true",
			"env DECLARED=its own set=true",
			`args 1 ["one argument, with spaces"]`,
		} {
			if !strings.Contains(told, want) {
				t.Errorf("%s was not handed %q:\n%s", id, want, told)
			}
		}
		if strings.Contains(told, "env YOKE_TOKEN= set") {
			t.Errorf("%s was handed no token:\n%s", id, told)
		}
		plugin := strings.Contains(told, "env YOKE_PLUGIN=com.yoke.test set=true")
		if plugin != (kind == unit.Plugin) {
			t.Errorf("%s: YOKE_PLUGIN handed is %v, want %v:\n%s", id, plugin, kind == unit.Plugin, told)
		}
	}
}

// std: yoke:the-supervisor.10
func TestAUnitsOutputIsCapturedWithItsIncarnation(t *testing.T) {
	s, out := started(t, fast())
	s.Launch(declared(t, "writer", unit.Interface, "write"))
	until(t, "two lives of the writer", 5*time.Second, func() bool {
		return strings.Count(strings.Join(out.all(), "\n"), "first line") >= 2
	})
	lines := strings.Join(out.all(), "\n")
	for _, want := range []string{"writer#1 first line", "writer#1 second line", "writer#1 on the error stream", "writer#2 first line"} {
		if !strings.Contains(lines, want) {
			t.Errorf("%q was not captured:\n%s", want, lines)
		}
	}
}

// std: yoke:the-supervisor.11
func TestAnExitReachesTheMachineWhenItHappens(t *testing.T) {
	policy := fast()
	policy.Backoff, policy.Ceiling = time.Hour, time.Hour
	s, _ := started(t, policy)
	brief := declared(t, "brief", unit.Interface, "exit-soon")
	leaving := filepath.Join(t.TempDir(), "leaving")
	brief.Env["YOKE_TEST_LEAVING"] = leaving
	s.Launch(brief)
	until(t, "the interface running", 5*time.Second, func() bool { return s.Status("brief").State == unit.Running })
	until(t, "the interface about to exit", 10*time.Second, func() bool { _, err := os.Stat(leaving); return err == nil })
	until(t, "the exit reaching the machine", time.Second, func() bool { return s.Status("brief").State == unit.Failed })
}

// std: yoke:the-supervisor.12
func TestTheWaitDoublesToItsCeilingAndAttemptsNeverRunOut(t *testing.T) {
	s, _ := started(t, fast())
	s.Launch(declared(t, "crashing", unit.Interface, "exit-1"))
	var waits []time.Duration
	var tokens []string
	seen := 0
	until(t, "six attempts", 10*time.Second, func() bool {
		status := s.Status("crashing")
		if status.Waiting && status.Attempt > seen {
			seen = status.Attempt
			waits = append(waits, status.Wait)
			tokens = append(tokens, status.Token)
		}
		return seen >= 6
	})
	want := []time.Duration{50, 100, 200, 200, 200}
	for i, w := range want {
		if i < len(waits) && waits[i] != w*time.Millisecond {
			t.Errorf("wait %d is %v, want %v (all: %v)", i+1, waits[i], w*time.Millisecond, waits)
		}
	}
	distinct := map[string]bool{}
	for _, token := range tokens {
		distinct[token] = true
	}
	if len(distinct) != len(tokens) {
		t.Errorf("two attempts carried one token: %v", tokens)
	}
	if status := s.Status("crashing"); status.Incarnation < 6 {
		t.Errorf("after six attempts the incarnation is %d", status.Incarnation)
	}
}

// std: yoke:the-supervisor.13
func TestWaitingIsReportedAndIsNotAState(t *testing.T) {
	policy := fast()
	policy.Backoff, policy.Ceiling = time.Hour, time.Hour
	s, _ := started(t, policy)
	s.Launch(declared(t, "down", unit.Interface, "exit-1"))
	until(t, "the unit waiting", 5*time.Second, func() bool { return s.Status("down").Waiting })
	status := s.Status("down")
	if status.State != unit.Failed {
		t.Errorf("a unit waiting is %s, want the Failed its last incarnation reached", status.State)
	}
	if status.Attempt != 1 || time.Until(status.NextAt) < 50*time.Minute {
		t.Errorf("the status says attempt %d, next at %v; want attempt 1, about an hour away", status.Attempt, status.NextAt)
	}
}

// std: yoke:the-supervisor.14
func TestAUnitThatStayedReadyStartsCountingAgain(t *testing.T) {
	policy := fast()
	policy.StabilityWindow = 100 * time.Millisecond
	s, _ := started(t, policy)
	u := declared(t, "flaky", unit.Interface, "exit-1")
	s.Launch(u)
	until(t, "a second failure", 5*time.Second, func() bool { return s.Status("flaky").Attempt >= 2 })
	s.Declare(u.ID, func(d *supervisor.Unit) { d.Env[role] = "exit-soon" })
	until(t, "the unit running longer than the window, then failing", 5*time.Second, func() bool {
		status := s.Status("flaky")
		return status.Waiting && status.Attempt == 1
	})
	if wait := s.Status("flaky").Wait; wait != 50*time.Millisecond {
		t.Errorf("after a stable run the wait is %v, want the first wait again", wait)
	}
}

// std: yoke:the-supervisor.15
func TestAUnitThatNeverBecomesReadyIsEndedAtTheWindow(t *testing.T) {
	policy := fast()
	policy.StartupWindow = 300 * time.Millisecond
	policy.Backoff, policy.Ceiling = time.Hour, time.Hour
	s, _ := started(t, policy)
	s.Launch(declared(t, "silent", unit.Plugin, "serve"))
	until(t, "the process up", 5*time.Second, func() bool { return s.Status("silent").PID != 0 })
	pid := s.Status("silent").PID
	until(t, "the window ending it", 3*time.Second, func() bool { return s.Status("silent").State == unit.Failed })
	if syscall.Kill(pid, 0) == nil {
		t.Errorf("process %d is still there after its startup window", pid)
	}
}

// std: yoke:the-supervisor.16
func TestStoppingAsksInReverseWaitsAndEnds(t *testing.T) {
	s, _ := started(t, fast())
	childFile := filepath.Join(t.TempDir(), "child")
	first := declared(t, "first", unit.Interface, "serve")
	second := declared(t, "second", unit.Interface, "fork")
	second.Env["YOKE_TEST_CHILD_PID"] = childFile
	third := declared(t, "third", unit.Interface, "stubborn")
	ignoring := filepath.Join(t.TempDir(), "ignoring")
	third.Env["YOKE_TEST_IGNORING"] = ignoring
	for _, u := range []supervisor.Unit{first, second, third} {
		s.Launch(u)
		until(t, u.ID+" running", 5*time.Second, func() bool { return s.Status(u.ID).State == unit.Running })
	}
	until(t, "the forked child", 5*time.Second, func() bool { _, err := os.Stat(childFile); return err == nil })
	until(t, "the third ignoring the signal", 5*time.Second, func() bool { _, err := os.Stat(ignoring); return err == nil })
	raw, _ := os.ReadFile(childFile)
	var child int
	fmt.Sscan(string(raw), &child)

	began := time.Now()
	if err := s.Stop(); err != nil {
		t.Errorf("stopping failed: %v", err)
	}
	if order := s.StopOrder(); strings.Join(order, ",") != "third,second,first" {
		t.Errorf("the units were asked to stop as %v, want third, second, first", order)
	}
	if time.Since(began) < 300*time.Millisecond {
		t.Error("the stubborn unit was not given its stop window")
	}
	for _, id := range []string{"first", "second", "third"} {
		if state := s.Status(id).State; state != unit.Stopped {
			t.Errorf("%s is %s after the stop, want Stopped", id, state)
		}
	}
	time.Sleep(100 * time.Millisecond)
	if syscall.Kill(child, 0) == nil {
		t.Errorf("the child %d the second unit forked outlived it", child)
	}
}

// std: yoke:the-supervisor.17
func TestASupervisorThatCannotObserveConcludesNothing(t *testing.T) {
	policy := fast()
	policy.Backoff, policy.Ceiling = time.Hour, time.Hour
	s, _ := started(t, policy)
	s.Launch(declared(t, "watched", unit.Interface, "serve"))
	until(t, "the unit running", 5*time.Second, func() bool { return s.Status("watched").State == unit.Running })

	since := time.Date(2026, 9, 25, 4, 0, 0, 0, time.UTC)
	s.Quiet(since)
	status := s.Status("watched")
	if status.State != unit.Running {
		t.Errorf("a unit nothing can observe moved to %s", status.State)
	}
	if !status.Unobservable || !status.UnobservableSince.Equal(since) {
		t.Errorf("the unit does not carry the condition since %v: %+v", since, status)
	}
	if err := s.StopUnit("watched"); err == nil || !strings.Contains(err.Error(), "host") {
		t.Errorf("stopping an unobservable unit gave %v, want a failure naming the backend", err)
	}
	s.Launch(declared(t, "late", unit.Interface, "serve"))
	until(t, "the launch failing as an attempt", 5*time.Second, func() bool { return s.Status("late").State == unit.Failed })

	s.Observable()
	status = s.Status("watched")
	if status.Unobservable || status.State != unit.Running {
		t.Errorf("after the source returned: %+v", status)
	}
}
