package trunk_test

import (
	"testing"

	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/trunk"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// std: yoke:the-migration-step.02
func TestTheRangeIsWeighedAgainstTheVersionThatLastRan(t *testing.T) {
	declared := func() []supervisor.Unit {
		return []supervisor.Unit{{ID: "migrate", Kind: unit.Oneshot, MigratesFrom: "<2.2"}, {ID: "acquire", Kind: unit.Plugin}}
	}
	for _, c := range []struct {
		lastRan string
		want    *supervisor.Skipped
	}{
		{"2.1.0", nil},
		{"2.2.0", &supervisor.Skipped{Range: "<2.2", Version: "2.2.0"}},
		{"", &supervisor.Skipped{Range: "<2.2", Version: "none"}},
	} {
		weighed := trunk.Migrations(declared(), c.lastRan)
		got := weighed[0].Skipped
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("against %q the oneshot is skipped %+v, want %+v", c.lastRan, got, c.want)
		}
		if weighed[1].Skipped != nil {
			t.Errorf("against %q the Plugin unit is skipped %+v", c.lastRan, weighed[1].Skipped)
		}
	}
}
