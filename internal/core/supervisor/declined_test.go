package supervisor_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// withDisabled is a supervisor whose plugins are disabled while disabled holds.
func withDisabled(t *testing.T, policy supervisor.Policy, disabled *atomic.Bool) (*supervisor.Supervisor, *published) {
	t.Helper()
	p := &published{}
	s := supervisor.New(supervisor.Config{Root: t.TempDir(), Policy: policy, Incarnations: supervisor.NewCounter(),
		Tokens: supervisor.NewTokens(), Output: &output{}, Publish: p.publish,
		Disabled: func(string) bool { return disabled.Load() }})
	t.Cleanup(func() { s.Stop() })
	return s, p
}

// std: yoke:the-supervisor.21
func TestAUnitOfADisabledPluginIsNotLaunchedAndIsRefused(t *testing.T) {
	var disabled atomic.Bool
	disabled.Store(true)
	s, p := withDisabled(t, fast(), &disabled)
	ran := filepath.Join(t.TempDir(), "ran")
	u := declared(t, "acquire", unit.Plugin, "exit-soon")
	u.Env["YOKE_TEST_LEAVING"] = ran
	s.Launch(u)
	until(t, "acquire to be refused", 2*time.Second, func() bool { return s.Status("acquire").State == unit.Refused })
	time.Sleep(4 * fast().Backoff)
	if st := s.Status("acquire"); st.State != unit.Refused || st.Incarnation != 0 || st.Waiting {
		t.Errorf("acquire is %+v, want Refused with no incarnation and nothing waiting", st)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Error("the unit's process ran")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var changes int
	for _, e := range p.events {
		if e.Type != "unit.state.changed" || e.Subject.ID != "acquire" {
			continue
		}
		changes++
		var detail map[string]any
		json.Unmarshal(e.Detail, &detail)
		declined, _ := detail["declined"].(map[string]any)
		if e.Subject.Incarnation != 0 || detail["to"] != string(unit.Refused) || detail["plugin"] != "com.yoke.test" ||
			declined["reason"] != "plugin.disabled" {
			t.Errorf("the change is %s %s, want Refused with no incarnation, the plugin and declined plugin.disabled", e.Subject.ID, e.Detail)
		}
	}
	if changes != 1 {
		t.Errorf("%d changes were published about acquire, want one", changes)
	}
}

// std: yoke:the-supervisor.22
func TestAUnitWithdrawnByADisableEndsRefused(t *testing.T) {
	var disabled atomic.Bool
	policy := fast()
	policy.Backoff, policy.Ceiling = 2*time.Second, 2*time.Second
	s, _ := withDisabled(t, policy, &disabled)
	asked := filepath.Join(t.TempDir(), "asked")
	slow := declared(t, "slow", unit.Plugin, "noted")
	slow.Env["YOKE_TEST_LEAVING"] = asked
	s.Launch(slow)
	s.Launch(declared(t, "fallen", unit.Plugin, "exit-1"))
	until(t, "slow to be starting", 2*time.Second, func() bool { return s.Status("slow").State == unit.Starting })
	until(t, "fallen to wait for its relaunch", 2*time.Second, func() bool { return s.Status("fallen").Waiting })
	time.Sleep(100 * time.Millisecond) // the process of slow has settled into waiting for its signal
	fell := s.Status("fallen").Incarnation

	disabled.Store(true)
	for _, id := range []string{"slow", "fallen"} {
		if st, moved := s.Withdraw(id); !moved || st.State != unit.Refused {
			t.Errorf("%s was withdrawn to %+v (moved %v), want Refused", id, st, moved)
		}
	}
	if st := s.Status("slow"); st.State != unit.Refused || st.Incarnation != 1 {
		t.Errorf("slow is %+v, want Refused at its incarnation", st)
	}
	until(t, "slow's process to be asked to end", 2*time.Second, func() bool { _, err := os.Stat(asked); return err == nil })
	time.Sleep(policy.Backoff + 500*time.Millisecond)
	if st := s.Status("fallen"); st.State != unit.Refused || st.Waiting || st.Incarnation != fell {
		t.Errorf("fallen is %+v, want Refused, not waiting, and not relaunched past incarnation %d", st, fell)
	}
}
