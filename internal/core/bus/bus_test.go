package bus_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
)

func changed(unitID string) event.Event {
	return event.StateChanged(unitID, 1, unit.Starting, unit.Running)
}

// next is the subscriber's next delivery, failing the test if none arrives within a second.
func next(t *testing.T, s *bus.Subscription) bus.Delivery {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	d, err := s.Next(ctx)
	if err != nil {
		t.Fatalf("nothing was delivered: %v", err)
	}
	return d
}

// quiet says nothing more is delivered for a moment.
func quiet(t *testing.T, s *bus.Subscription) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if d, err := s.Next(ctx); err == nil {
		t.Fatalf("%+v was delivered where nothing should be", d)
	}
}

// std: yoke:the-event-bus.01
func TestEveryEventPublishedIsNumbered(t *testing.T) {
	b := bus.New()
	s := b.Subscribe()
	defer s.Close()
	first, err := b.Publish(changed("a"))
	if err != nil || first.Seq != 1 {
		t.Fatalf("the first was published as %d: %v", first.Seq, err)
	}
	undeclared := changed("a")
	undeclared.Type = "unit.lamp.changed"
	if _, err := b.Publish(undeclared); err == nil {
		t.Error("an event of an undeclared type was published")
	}
	second, err := b.Publish(changed("b"))
	if err != nil || second.Seq != 2 {
		t.Fatalf("the second was published as %d: %v", second.Seq, err)
	}
	for _, want := range []uint64{1, 2} {
		if d := next(t, s); d.Overflow || d.Event.Seq != want {
			t.Errorf("delivered %+v, want the event numbered %d", d, want)
		}
	}
	quiet(t, s)
}

// std: yoke:the-event-bus.02
func TestASubscriberIsToldInOrderAndPublishingNeverWaits(t *testing.T) {
	b := bus.New()
	reader, stalled := b.Subscribe(), b.Subscribe()
	defer reader.Close()
	defer stalled.Close()
	for i := range 200 {
		b.Publish(changed([]string{"a", "b"}[i%2]))
	}
	last := map[string]uint64{}
	for range 200 {
		d := next(t, reader)
		if d.Overflow || d.Event.Seq <= last[d.Event.Subject.ID] {
			t.Fatalf("delivered %+v after %d for its subject", d, last[d.Event.Subject.ID])
		}
		last[d.Event.Subject.ID] = d.Event.Seq
	}
	start := time.Now()
	for i := range 10000 {
		if _, err := b.Publish(changed([]string{"a", "b"}[i%2])); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("publishing ten thousand took %v", elapsed)
	}
}

// std: yoke:the-event-bus.03
func TestAnOverflowIsAnnouncedNeverSilentAndNeverAClose(t *testing.T) {
	b := bus.New()
	slow, reader := b.Subscribe(), b.Subscribe()
	defer slow.Close()
	defer reader.Close()
	// The reading subscriber reads after every hundred published, so it never falls behind.
	told := 0
	publish := func(n int) {
		t.Helper()
		for i := range n {
			b.Publish(changed("a"))
			if (i+1)%100 == 0 || i == n-1 {
				for {
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
					d, err := reader.Next(ctx)
					cancel()
					if err != nil {
						break
					}
					if d.Overflow {
						t.Fatal("the reading subscriber was told of an overflow")
					}
					told++
				}
			}
		}
	}
	publish(bus.Bound)
	if d := next(t, slow); d.Overflow || d.Event.Seq != 1 {
		t.Fatalf("the slow subscriber was first told %+v", d)
	}
	publish(300)
	var seqs []uint64
	announced := false
	for {
		d := next(t, slow)
		if d.Overflow {
			announced = true
			break
		}
		seqs = append(seqs, d.Event.Seq)
	}
	if !announced || len(seqs) != bus.Bound || seqs[0] != 2 || seqs[len(seqs)-1] != bus.Bound+1 {
		t.Fatalf("before the announcement the slow subscriber was told %d events, %v…%v", len(seqs), seqs[:1], seqs[len(seqs)-1:])
	}
	quiet(t, slow)
	publish(1)
	if d := next(t, slow); d.Overflow || d.Event.Seq != 557 {
		t.Errorf("after the overflow the slow subscriber was told %+v, want the event numbered 557", d)
	}
	if told != 557 {
		t.Errorf("the reading subscriber was told %d events, want 557", told)
	}
}

// std: yoke:the-event-bus.04
func TestThePluginSurfaceCarriesNoSubscription(t *testing.T) {
	protos, _ := filepath.Glob("../../../proto/yoke/plugin/v1/*.proto")
	if len(protos) == 0 {
		t.Fatal("the plugin contract's definitions were not found")
	}
	declaration := regexp.MustCompile(`^\s*(service|rpc|message)\s+(\w+)`)
	for _, p := range protos {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if m := declaration.FindStringSubmatch(line); m != nil {
				name := strings.ToLower(m[2])
				if strings.Contains(name, "subscri") || strings.Contains(name, "watch") || strings.Contains(name, "notif") {
					t.Errorf("%s: the %s %s", filepath.Base(p), m[1], m[2])
				}
			}
		}
	}
}
