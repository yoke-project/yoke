package supervisor_test

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// sequence is what reaches the log, events and lines alike, in the order they arrive.
type sequence struct {
	mu   sync.Mutex
	seen []string
}

func (q *sequence) Line(unitID string, incarnation int, _, line string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seen = append(q.seen, fmt.Sprintf("line %s#%d %s", unitID, incarnation, line))
}

func (q *sequence) publish(e event.Event) {
	if e.Type != "unit.state.changed" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.seen = append(q.seen, fmt.Sprintf("event %s#%d %s", e.Subject.ID, e.Subject.Incarnation, e.Detail))
}

func (q *sequence) all() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]string(nil), q.seen...)
}

// opens reports whether the incarnation's Starting comes before every line it wrote.
func opens(seen []string, life string) error {
	starting := slices.IndexFunc(seen, func(s string) bool {
		return strings.HasPrefix(s, "event "+life+" ") && strings.Contains(s, `"to":"Starting"`)
	})
	first := slices.IndexFunc(seen, func(s string) bool { return strings.HasPrefix(s, "line "+life+" ") })
	switch {
	case first < 0:
		return fmt.Errorf("%s wrote nothing: %v", life, seen)
	case starting < 0 || starting > first:
		return fmt.Errorf("%s spoke before its Starting: %v", life, seen)
	}
	return nil
}

// std: yoke:the-supervisor.20
func TestAnIncarnationsOutputFollowsTheStateChangeThatStartsIt(t *testing.T) {
	c := newContainers()
	c.says = "at once"
	c.ends = []int{0}
	q := &sequence{}
	s := supervisor.New(supervisor.Config{Root: t.TempDir(), Policy: fast(), Instance: "bench", Containers: c,
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: q, Publish: q.publish})
	t.Cleanup(func() { s.Stop() })
	s.Launch(imaged("calibrate", unit.Oneshot))
	s.Launch(declared(t, "beside", unit.Oneshot, "write"))
	until(t, "both units to end", 5*time.Second, func() bool {
		return s.Status("calibrate").State == unit.Completed && s.Status("beside").State == unit.Failed
	})
	for _, life := range []string{"calibrate#1", "beside#1"} {
		if err := opens(q.all(), life); err != nil {
			t.Error(err)
		}
	}
}
