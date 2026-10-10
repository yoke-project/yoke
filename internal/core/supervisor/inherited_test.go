package supervisor_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/supervisor"
)

// std: yoke:the-inherited-containers.01
func TestEveryContainerUnderTheLabelIsRemovedAndNamed(t *testing.T) {
	c := newContainers()
	c.foreign = []engine.Found{
		{ID: "c-running", Unit: "calibrate", Incarnation: 3, Running: true},
		{ID: "c-ended", Unit: "probe", Incarnation: 1, ExitCode: 1},
	}
	cleared, err := supervisor.Inherited(context.Background(), c, "bench")
	if err != nil {
		t.Fatal(err)
	}
	if !c.wasRemoved("c-running") || !c.wasRemoved("c-ended") {
		t.Errorf("removed: %v", c.acts())
	}
	if got := c.watched(); !slices.Equal(got, []string{"list"}) {
		t.Errorf("the engine was asked %v, want it asked once what runs under the label", got)
	}
	for _, act := range c.acts() {
		if !strings.HasPrefix(act, "remove ") {
			t.Errorf("the engine was asked to %s", act)
		}
	}
	slices.SortFunc(cleared, func(a, b engine.Found) int { return strings.Compare(a.ID, b.ID) })
	if want := []engine.Found{c.foreign[1], c.foreign[0]}; !slices.Equal(cleared, want) {
		t.Errorf("what was cleared is %+v, want %+v", cleared, want)
	}
}

// std: yoke:the-inherited-containers.02
func TestAnEngineNotAskedOrAContainerNotRemovedIsAnErrorNamingIt(t *testing.T) {
	away := newContainers()
	away.away = true
	if _, err := supervisor.Inherited(context.Background(), away, "bench"); err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Errorf("against an engine that cannot be reached: %v", err)
	}

	c := newContainers()
	c.foreign = []engine.Found{{ID: "c-stuck", Unit: "calibrate", Incarnation: 1, Running: true}, {ID: "c-other", Unit: "probe", Incarnation: 2}}
	c.refuses = map[string]bool{"c-stuck": true}
	_, err := supervisor.Inherited(context.Background(), c, "bench")
	if err == nil || !strings.Contains(err.Error(), "c-stuck") {
		t.Errorf("against a refused removal: %v", err)
	}
	if !c.wasRemoved("c-other") {
		t.Errorf("the other container was not asked to be removed: %v", c.acts())
	}
}
