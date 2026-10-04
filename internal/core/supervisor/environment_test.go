package supervisor_test

import (
	"slices"
	"strings"
	"testing"

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
