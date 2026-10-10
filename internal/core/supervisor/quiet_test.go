package supervisor_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// published collects what the supervisor publishes.
type published struct {
	mu     sync.Mutex
	events []event.Event
}

func (p *published) publish(e event.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
}

// conditions are the unit.condition.changed events published about a unit.
func (p *published) conditions(unitID string) []event.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []event.Event
	for _, e := range p.events {
		if e.Type == "unit.condition.changed" && e.Subject.ID == unitID {
			out = append(out, e)
		}
	}
	return out
}

// beside is a supervisor whose engine is c, which publishes what it concludes.
func beside(t *testing.T, c *containers, policy supervisor.Policy) (*supervisor.Supervisor, *output, *published) {
	t.Helper()
	out, pub := &output{}, &published{}
	s := supervisor.New(supervisor.Config{Root: t.TempDir(), Policy: policy, Instance: "bench", Containers: c,
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: out, Publish: pub.publish})
	t.Cleanup(func() {
		c.comeBack()
		stopped := make(chan struct{})
		go func() { s.Stop(); close(stopped) }()
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Error("the supervisor did not stop")
		}
	})
	return s, out, pub
}

func allRunning(s *supervisor.Supervisor, ids ...string) func() bool {
	return func() bool {
		for _, id := range ids {
			if s.Status(id).State != unit.Running {
				return false
			}
		}
		return true
	}
}

func unobservable(s *supervisor.Supervisor, id string) func() bool {
	return func() bool { return s.Status(id).Unobservable }
}

func patient() supervisor.Policy {
	policy := fast()
	policy.Backoff, policy.Ceiling = time.Hour, time.Hour
	return policy
}

// std: yoke:the-quiet-engine.01
func TestTheEnginesEventsEndingIsTheEngineGoingQuiet(t *testing.T) {
	c := newContainers()
	s, _, pub := beside(t, c, patient())
	s.Launch(imaged("panel", unit.Interface))
	s.Launch(declared(t, "acquire", unit.Interface, "serve"))
	until(t, "both units running", 5*time.Second, allRunning(s, "panel", "acquire"))

	before := time.Now()
	c.goAway()
	until(t, "the containerised unit unobservable", 3*time.Second, unobservable(s, "panel"))
	st := s.Status("panel")
	if st.State != unit.Running || st.Incarnation != 1 {
		t.Errorf("a unit nothing can observe is %s on incarnation %d", st.State, st.Incarnation)
	}
	if !st.HasCondition || st.Condition.Grade != event.Notable || !strings.Contains(st.Condition.Line, "container engine") {
		t.Errorf("the unit carries the condition %+v (%v)", st.Condition, st.HasCondition)
	}
	if st.ConditionSince.Before(before) || st.ConditionSince.After(time.Now()) || !st.UnobservableSince.Equal(st.ConditionSince) {
		t.Errorf("the condition began at %v, and unobservable since %v; the stream ended after %v", st.ConditionSince, st.UnobservableSince, before)
	}
	got := pub.conditions("panel")
	if len(got) != 1 || got[0].Actor.Class != event.ByCore || got[0].Severity != event.Notable || got[0].Subject.Incarnation != 1 {
		t.Errorf("published about the containerised unit: %+v", got)
	}
	host := s.Status("acquire")
	if host.HasCondition || host.Unobservable || len(pub.conditions("acquire")) != 0 {
		t.Errorf("the host unit is %+v", host)
	}
	if st.Backend != "container" || host.Backend != "host" {
		t.Errorf("the backends are %q and %q", st.Backend, host.Backend)
	}
}

// std: yoke:the-quiet-engine.02
func TestWhileTheEngineIsQuietAnOperationNeedingItIsAFault(t *testing.T) {
	c := newContainers()
	s, _, _ := beside(t, c, patient())
	s.Launch(imaged("panel", unit.Interface))
	s.Launch(declared(t, "acquire", unit.Interface, "serve"))
	until(t, "both units running", 5*time.Second, allRunning(s, "panel", "acquire"))
	c.goAway()
	until(t, "the containerised unit unobservable", 3*time.Second, unobservable(s, "panel"))

	asked := len(c.acts())
	for name, op := range map[string]func(string) error{"stop": s.StopUnit, "start": s.StartUnit, "restart": s.RestartUnit} {
		err := op("panel")
		if !errors.Is(err, supervisor.ErrUnreachable) || !strings.Contains(err.Error(), "container") {
			t.Errorf("%s gave %v, want a fault naming the container backend", name, err)
		}
	}
	if got := c.acts(); len(got) != asked {
		t.Errorf("the engine was asked %v", got[asked:])
	}
	if st := s.Status("panel"); st.State != unit.Running {
		t.Errorf("the containerised unit is %s", st.State)
	}
	if err := s.StopUnit("acquire"); err != nil {
		t.Fatal(err)
	}
	until(t, "the host unit stopped", 3*time.Second, func() bool { return s.Status("acquire").State == unit.Stopped })
	err := s.Stop()
	if !errors.Is(err, supervisor.ErrUnreachable) || !strings.Contains(err.Error(), "panel") {
		t.Errorf("the supervisor's stop gave %v", err)
	}
}

// std: yoke:the-quiet-engine.03
func TestALaunchWhileTheEngineIsQuietIsAnOrdinaryFailedAttempt(t *testing.T) {
	c := newContainers()
	s, _, _ := beside(t, c, fast())
	s.Launch(imaged("panel", unit.Interface))
	until(t, "the first unit running", 3*time.Second, allRunning(s, "panel"))
	c.goAway()
	until(t, "the containerised unit unobservable", 3*time.Second, unobservable(s, "panel"))

	s.Launch(imaged("late", unit.Interface))
	until(t, "the launch failing and waiting", 3*time.Second, func() bool {
		st := s.Status("late")
		return st.State == unit.Failed && st.Waiting
	})
	if st := s.Status("late"); !strings.Contains(st.Failure, "container engine") {
		t.Errorf("the failure says %q", st.Failure)
	}
	if slices.Contains(c.acts(), "create late") {
		t.Error("a container was created while the engine was away")
	}
	c.comeBack()
	until(t, "a later attempt running", 3*time.Second, allRunning(s, "late"))
	acts := c.acts()
	create := slices.Index(acts, "create late")
	if create < 0 || create+2 >= len(acts) || !strings.HasPrefix(acts[create+1], "attach ") || !strings.HasPrefix(acts[create+2], "start ") {
		t.Errorf("the engine was asked %v", acts)
	}
	if watched := c.watched(); !slices.Contains(watched, "list") {
		t.Errorf("the engine's return was not reconciled: %v", watched)
	}
	for _, id := range []string{"panel", "late"} {
		if st := s.Status(id); st.Unobservable || st.HasCondition {
			t.Errorf("%s still carries the condition: %+v", id, st)
		}
	}
}

// std: yoke:the-quiet-engine.04
func TestTheEnginesReturnIsAnsweredByReconciling(t *testing.T) {
	c := newContainers()
	s, out, pub := beside(t, c, patient())
	s.Launch(imaged("calibrate", unit.Oneshot))
	s.Launch(imaged("panel", unit.Interface))
	s.Launch(imaged("spare", unit.Interface))
	until(t, "the three running", 3*time.Second, allRunning(s, "calibrate", "panel", "spare"))
	ids := map[string]string{}
	c.mu.Lock()
	for id, l := range c.of {
		ids[l.Unit] = id
	}
	c.mu.Unlock()
	c.goAway()
	until(t, "the units unobservable", 3*time.Second, unobservable(s, "spare"))
	c.endQuietly(ids["calibrate"], 0)
	c.vanish(ids["spare"])
	c.mu.Lock()
	c.foreign = []engine.Found{{ID: "x9", Unit: "stray", Incarnation: 7, Running: true}}
	c.mu.Unlock()
	followed := len(c.watched())

	c.comeBack()
	c.notice()
	until(t, "the oneshot concluded", 3*time.Second, func() bool { return s.Status("calibrate").State == unit.Completed })
	until(t, "the vanished one concluded", 3*time.Second, func() bool { return s.Status("spare").State == unit.Failed })
	watched := c.watched()[followed:]
	events, list := slices.Index(watched, "events"), slices.Index(watched, "list")
	if events < 0 || list < events {
		t.Errorf("on its return the engine was %v", watched)
	}
	until(t, "the ended and the foreign containers removed", 3*time.Second, func() bool {
		return c.wasRemoved(ids["calibrate"]) && c.wasRemoved("x9")
	})
	if st := s.Status("panel"); st.State != unit.Running || st.Incarnation != 1 {
		t.Errorf("the running one is %s on incarnation %d", st.State, st.Incarnation)
	}
	for _, act := range c.acts() {
		if strings.HasPrefix(act, "signal "+ids["panel"]) {
			t.Errorf("the running one was asked to %s", act)
		}
	}
	c.write(ids["panel"], "after the return")
	until(t, "the running one's later line", 3*time.Second, func() bool { return slices.Contains(out.all(), "panel#1 after the return") })
	for _, id := range []string{"calibrate", "panel", "spare"} {
		if st := s.Status(id); st.Unobservable || st.HasCondition {
			t.Errorf("%s still carries the condition: %+v", id, st)
		}
		got := pub.conditions(id)
		if len(got) != 2 || got[1].Actor.Class != event.ByCore {
			t.Errorf("published about %s: %+v", id, got)
			continue
		}
		var detail map[string]any
		json.Unmarshal(got[1].Detail, &detail)
		if _, has := detail["to"]; has || detail["from"] != float64(event.Notable) {
			t.Errorf("the condition cleared from %s says %v", id, detail)
		}
	}
}

// std: yoke:the-quiet-engine.05
func TestTheReturnIsLearntFromANoticeAndNeverOnATimer(t *testing.T) {
	c := newContainers()
	s, _, _ := beside(t, c, fast())
	s.Launch(imaged("panel", unit.Interface))
	until(t, "the unit running", 3*time.Second, allRunning(s, "panel"))
	followed := len(c.watched())
	c.goAway()
	until(t, "the unit unobservable", 3*time.Second, unobservable(s, "panel"))
	since := s.Status("panel").ConditionSince

	time.Sleep(20 * fast().Backoff)
	if away := c.watched()[followed:]; !slices.Equal(away, []string{"events"}) {
		t.Errorf("while the engine was away it was asked %v, want the one attempt to follow it again", away)
	}
	if st := s.Status("panel"); !st.Unobservable || !st.ConditionSince.Equal(since) {
		t.Errorf("the condition moved while nothing was known: %+v, began %v", st, since)
	}

	c.comeBack()
	c.notice()
	until(t, "the condition gone", 3*time.Second, func() bool { return !s.Status("panel").Unobservable })
	back := c.watched()[followed+1:]
	if events, list := slices.Index(back, "events"), slices.Index(back, "list"); events < 0 || list < events {
		t.Errorf("after the notice the engine was %v", back)
	}
}

// std: yoke:the-quiet-engine.08
func TestOnlyTheUnitsRunningOnAQuietEngineCarryTheCondition(t *testing.T) {
	c := newContainers()
	c.ends = []int{0}
	s, _, pub := beside(t, c, patient())
	s.Launch(imaged("calibrate", unit.Oneshot))
	until(t, "the oneshot completed", 3*time.Second, func() bool { return s.Status("calibrate").State == unit.Completed })
	s.Launch(imaged("panel", unit.Interface))
	until(t, "the interface running", 3*time.Second, allRunning(s, "panel"))

	c.goAway()
	until(t, "the running unit unobservable", 3*time.Second, unobservable(s, "panel"))
	if st := s.Status("calibrate"); st.Unobservable || st.HasCondition || st.State != unit.Completed {
		t.Errorf("the concluded unit is %+v", st)
	}
	if got := pub.conditions("calibrate"); len(got) != 0 {
		t.Errorf("published about the concluded unit while the engine was away: %+v", got)
	}
	c.comeBack()
	c.notice()
	until(t, "the running unit observable again", 3*time.Second, func() bool { return !s.Status("panel").Unobservable })
	if got := pub.conditions("calibrate"); len(got) != 0 {
		t.Errorf("published about the concluded unit on the engine's return: %+v", got)
	}
	if got := pub.conditions("panel"); len(got) != 2 {
		t.Errorf("published about the running unit: %+v", got)
	}
}
