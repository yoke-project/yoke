package supervisor_test

import (
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// events keeps every event published, safely across goroutines.
type events struct {
	mu  sync.Mutex
	all []event.Event
}

func (e *events) publish(ev event.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.all = append(e.all, ev)
}

func (e *events) of(id string) []event.Event {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []event.Event
	for _, ev := range e.all {
		if ev.Type == "unit.state.changed" && ev.Subject.ID == id {
			out = append(out, ev)
		}
	}
	return out
}

// std: yoke:the-migration-step.01
func TestASkippedUnitIsCompletedWithoutALaunch(t *testing.T) {
	out, published := &output{}, &events{}
	s := supervisor.New(supervisor.Config{Root: t.TempDir(), Policy: fast(),
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: out, Publish: published.publish})
	t.Cleanup(func() { s.Stop() })
	migrate := declared(t, "migrate", unit.Oneshot, "write")
	migrate.Skipped = &supervisor.Skipped{Range: "<2.2", Version: "2.2.0"}
	s.Start([]supervisor.Unit{migrate, dependsOn(declared(t, "store", unit.Interface, "serve"), "migrate")})

	until(t, "store launched", 2*time.Second, func() bool { return s.Status("store").Incarnation == 1 })
	if st := s.Status("migrate"); st.State != unit.Completed || st.Incarnation != 0 {
		t.Errorf("migrate is %+v", st)
	}
	if slices.ContainsFunc(out.all(), func(l string) bool { return strings.HasPrefix(l, "migrate#") }) {
		t.Errorf("migrate's program ran: %v", out.all())
	}
	seen := published.of("migrate")
	if len(seen) != 1 {
		t.Fatalf("migrate's state events: %+v", seen)
	}
	var detail map[string]any
	json.Unmarshal(seen[0].Detail, &detail)
	skipped, _ := detail["skipped"].(map[string]any)
	if seen[0].Subject.Incarnation != 0 || detail["from"] != nil || detail["to"] != "Completed" ||
		skipped["range"] != "<2.2" || skipped["version"] != "2.2.0" {
		t.Errorf("the event is %+v, detail %s", seen[0].Subject, seen[0].Detail)
	}
}
