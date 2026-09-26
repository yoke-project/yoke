package supervisor_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// told collects what the supervisor reports as not started.
type told struct {
	mu     sync.Mutex
	causes map[string][]string
}

func (n *told) notStarted(unitID, cause string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.causes[unitID] = append(n.causes[unitID], cause)
}

func (n *told) of(unitID string) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.causes[unitID]...)
}

// reporting is a supervisor that tells n of every unit not started.
func reporting(t *testing.T, n *told) *supervisor.Supervisor {
	t.Helper()
	n.causes = map[string][]string{}
	s := supervisor.New(supervisor.Config{
		Root: t.TempDir(), Policy: fast(),
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: &output{},
		NotStarted: n.notStarted,
	})
	t.Cleanup(func() { s.Stop() })
	return s
}

// window gives a unit its own startup window.
func window(u supervisor.Unit, d time.Duration) supervisor.Unit {
	p := fast()
	p.StartupWindow = d
	u.Policy = &p
	return u
}

func dependsOn(u supervisor.Unit, on ...string) supervisor.Unit {
	u.DependsOn = on
	return u
}

// held says a unit is waiting on exactly these dependencies, with no incarnation and no state.
func held(t *testing.T, s *supervisor.Supervisor, id string, on ...string) {
	t.Helper()
	status := s.Status(id)
	if status.Incarnation != 0 || status.State != "" || !slices.Equal(status.Awaiting, on) {
		t.Errorf("%s is %+v, want no incarnation, no state, awaiting %v", id, status, on)
	}
}

// std: yoke:the-launch-order.01
func TestWhatDependsOnNothingUnmetIsLaunchedAtOnce(t *testing.T) {
	s, _ := started(t, fast())
	s.Start([]supervisor.Unit{
		declared(t, "a", unit.Oneshot, "serve"),
		declared(t, "b", unit.Interface, "serve"),
		dependsOn(declared(t, "c", unit.Oneshot, "exit-0"), "a"),
	})
	for _, id := range []string{"a", "b"} {
		if status := s.Status(id); status.Incarnation == 0 {
			t.Errorf("%s had no incarnation when starting returned: %+v", id, status)
		}
	}
	held(t, s, "c", "a")
}

// std: yoke:the-launch-order.02
func TestADependentOfAPluginIsLaunchedWhenItsSessionOpens(t *testing.T) {
	s, _ := started(t, fast())
	s.Start([]supervisor.Unit{
		declared(t, "acquire", unit.Plugin, "serve"),
		dependsOn(declared(t, "archive", unit.Interface, "serve"), "acquire"),
	})
	s.Input("acquire", unit.AdmissionAccepted{})
	time.Sleep(50 * time.Millisecond)
	held(t, s, "archive", "acquire")
	s.Input("acquire", unit.SessionOpened{})
	until(t, "archive launched", 2*time.Second, func() bool { return s.Status("archive").Incarnation == 1 })
}

// std: yoke:the-launch-order.03
func TestADependentOfARunToCompletionIsLaunchedAfterItExitsZero(t *testing.T) {
	s, _ := started(t, fast())
	s.Start([]supervisor.Unit{
		declared(t, "provision", unit.Oneshot, "exit-soon"),
		dependsOn(declared(t, "store", unit.Interface, "serve"), "provision"),
	})
	until(t, "provision running", 2*time.Second, func() bool { return s.Status("provision").State == unit.Running })
	held(t, s, "store", "provision")
	until(t, "provision completed", 2*time.Second, func() bool { return s.Status("provision").State == unit.Completed })
	until(t, "store launched", 2*time.Second, func() bool { return s.Status("store").Incarnation == 1 })
}

// std: yoke:the-launch-order.04
func TestADependencyThatNeverArrivesIsReportedWithItsChain(t *testing.T) {
	n := &told{}
	s := reporting(t, n)
	s.Start([]supervisor.Unit{
		declared(t, "provision", unit.Oneshot, "exit-1"),
		window(dependsOn(declared(t, "store", unit.Interface, "serve"), "provision"), 300*time.Millisecond),
		window(dependsOn(declared(t, "archive", unit.Interface, "serve"), "store"), 300*time.Millisecond),
		declared(t, "other", unit.Interface, "serve"),
	})
	time.Sleep(time.Second)
	if s.Status("other").State != unit.Running {
		t.Errorf("other is %+v, want running", s.Status("other"))
	}
	want := map[string]string{
		"store":   "store did not start because provision exited 1",
		"archive": "archive did not start because store did not start because provision exited 1",
	}
	for id, cause := range want {
		status := s.Status(id)
		if status.Incarnation != 0 || status.State != "" {
			t.Errorf("%s is %+v, want no incarnation and no state", id, status)
		}
		if got := n.of(id); len(got) != 1 || got[0] != cause {
			t.Errorf("the supervisor was told of %s %q, want once %q", id, got, cause)
		}
		if status.NotStarted != cause {
			t.Errorf("%s's status says %q, want %q", id, status.NotStarted, cause)
		}
	}
}

// std: yoke:the-launch-order.05
func TestADependencyTheInstanceDoesNotStartNeverArrives(t *testing.T) {
	n := &told{}
	s := reporting(t, n)
	s.Start([]supervisor.Unit{window(dependsOn(declared(t, "store", unit.Interface, "serve"), "later"), 300*time.Millisecond)})
	time.Sleep(time.Second)
	want := "store did not start because later does not start with the instance"
	if got := n.of("store"); len(got) != 1 || got[0] != want {
		t.Errorf("the supervisor was told of store %q, want once %q", got, want)
	}
}

// std: yoke:the-launch-order.06
func TestUnitsStopInTheReverseOfTheirLaunching(t *testing.T) {
	n := &told{}
	s := reporting(t, n)
	s.Start([]supervisor.Unit{
		dependsOn(declared(t, "c", unit.Plugin, "serve"), "a"),
		declared(t, "a", unit.Oneshot, "exit-0"),
		declared(t, "b", unit.Interface, "serve"),
		window(dependsOn(declared(t, "d", unit.Interface, "serve"), "c"), 500*time.Millisecond),
	})
	until(t, "c launched", 2*time.Second, func() bool { return s.Status("c").Incarnation == 1 })
	s.Stop()
	time.Sleep(time.Second)
	order := s.StopOrder()
	if i, j := slices.Index(order, "c"), slices.Index(order, "b"); i < 0 || j < 0 || i > j {
		t.Errorf("the stop asked in the order %v, want c before b", order)
	}
	if status := s.Status("d"); status.Incarnation != 0 {
		t.Errorf("d was launched: %+v", status)
	}
	if got := n.of("d"); len(got) != 0 {
		t.Errorf("d was reported not started after the instance stopped: %q", got)
	}
}
