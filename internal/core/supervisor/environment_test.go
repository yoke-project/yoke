package supervisor_test

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// std: yoke:channels-bound.04
func TestAManagedInterfaceIsToldItsChannelAndNothingAPluginIsTold(t *testing.T) {
	s := supervisor.New(supervisor.Config{Root: "/run/yoke", Policy: supervisor.DefaultPolicy(), Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens()})
	names := func(env []string) []string {
		var out []string
		for _, kv := range env {
			out = append(out, strings.SplitN(kv, "=", 2)[0])
		}
		slices.Sort(out)
		return out
	}
	panel := s.Environment(supervisor.Unit{ID: "panel", Kind: unit.Interface, Channel: "/run/yoke/interfaces/front.sock"}, "token")
	if got := names(panel); !slices.Equal(got, []string{"YOKE_SOCKET", "YOKE_UNIT"}) {
		t.Errorf("the interface was given %v", panel)
	}
	if !slices.Contains(panel, "YOKE_SOCKET=/run/yoke/interfaces/front.sock") || !slices.Contains(panel, "YOKE_UNIT=panel") {
		t.Errorf("the interface was given %v", panel)
	}
	acquire := s.Environment(supervisor.Unit{ID: "acquire", Kind: unit.Plugin, Plugin: "com.example.station"}, "token")
	if !slices.Contains(acquire, "YOKE_SOCKET=/run/yoke/plugin.sock") || !slices.Contains(acquire, "YOKE_TOKEN=token") || !slices.Contains(acquire, "YOKE_PLUGIN=com.example.station") {
		t.Errorf("the Plugin was given %v", acquire)
	}
}

// issued records whom a token was issued for.
type issued struct {
	mu   sync.Mutex
	for_ []string
}

func (i *issued) Issue(unitID string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.for_ = append(i.for_, unitID)
	return "token-" + unitID
}

func (i *issued) units() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return append([]string(nil), i.for_...)
}

// std: yoke:what-a-unit-arrives-with.03
func TestAUnitThatRunsToCompletionIsToldItsIdentityAndNothingElse(t *testing.T) {
	tokens := &issued{}
	out := &output{}
	s := supervisor.New(supervisor.Config{Root: t.TempDir(), Policy: fast(), Incarnations: supervisor.NewCounter(), Tokens: tokens, Output: out})
	t.Cleanup(func() { s.Stop() })
	calibrate := declared(t, "calibrate", unit.Oneshot, "tell")
	calibrate.Env["DECLARED"] = "its own"
	s.Launch(calibrate)
	until(t, "the oneshot to complete", 3*time.Second, func() bool { return s.Status("calibrate").State == unit.Completed })
	lines := out.all()
	for _, want := range []string{"env YOKE_UNIT=calibrate set=true", "env DECLARED=its own set=true", "env YOKE_PLUGIN= set=false",
		"env YOKE_SOCKET= set=false", "env YOKE_BIND= set=false", "env YOKE_TOKEN= set=false"} {
		if !slices.Contains(lines, "calibrate#1 "+want) {
			t.Errorf("the oneshot did not arrive with %q: %v", want, lines)
		}
	}
	if got := tokens.units(); len(got) != 0 {
		t.Errorf("a bootstrap token was issued for %v, which present no admission", got)
	}
	if got := s.Status("calibrate").Token; got != "" {
		t.Errorf("the oneshot's status carries the token %q", got)
	}
}
