package bus_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
)

func into(t *testing.T, e event.Event) string {
	t.Helper()
	var d map[string]any
	json.Unmarshal(e.Detail, &d)
	to, _ := d["to"].(string)
	return to
}

// std: yoke:a-subscription.01
func TestASubscriptionOpensWithASnapshot(t *testing.T) {
	b := bus.New()
	b.Publish(event.StateChanged("acquire", 1, unit.Admitted, unit.Running))
	b.Publish(event.StateChanged("archive", 1, unit.Running, unit.Failed))
	b.Publish(event.OccurrenceReported("acquire", 1, &pluginv1.Event{Occurrence: "calibration.drift", Severity: 40}))
	last, _ := b.Publish(event.InstanceReady("bench"))
	s, snapshot := b.SubscribeTo(event.Filter{SubjectKind: event.Unit})
	defer s.Close()
	if snapshot.At != last.Seq || len(snapshot.Events) != 2 {
		t.Fatalf("the snapshot is at %d with %+v", snapshot.At, snapshot.Events)
	}
	for _, e := range snapshot.Events {
		if e.Type != "unit.state.changed" {
			t.Errorf("the snapshot holds %s", e.Type)
		}
	}
	b.Publish(event.StateChanged("acquire", 1, unit.Running, unit.Stopped))
	if d := next(t, s); d.Overflow || d.Event.Seq != snapshot.At+1 {
		t.Errorf("the first delivery is %+v, want the event numbered %d", d, snapshot.At+1)
	}
}

// std: yoke:a-subscription.02
func TestTheSnapshotHoldsEachSubjectsCurrentValue(t *testing.T) {
	b := bus.New()
	b.Publish(event.StateChanged("acquire", 1, "", unit.Starting))
	b.Publish(event.StateChanged("acquire", 1, unit.Starting, unit.Admitted))
	b.Publish(event.StateChanged("acquire", 1, unit.Admitted, unit.Running))
	b.Publish(event.StateChanged("acquire", 2, "", unit.Starting))
	s, snapshot := b.SubscribeTo(event.Filter{})
	defer s.Close()
	if len(snapshot.Events) != 1 || snapshot.Events[0].Subject.Incarnation != 2 || into(t, snapshot.Events[0]) != "Starting" {
		t.Errorf("the snapshot holds %+v", snapshot.Events)
	}
}

// std: yoke:a-subscription.03
func TestAFilterNarrowsTheStream(t *testing.T) {
	b := bus.New()
	s, _ := b.SubscribeTo(event.Filter{Floor: 50})
	defer s.Close()
	b.Publish(event.StateChanged("acquire", 1, unit.Admitted, unit.Running))
	failed, _ := b.Publish(event.StateChanged("archive", 1, unit.Running, unit.Failed))
	reported, _ := b.Publish(event.OccurrenceReported("acquire", 1, &pluginv1.Event{Occurrence: "calibration.drift", Severity: 90}))
	for _, want := range []uint64{failed.Seq, reported.Seq} {
		if d := next(t, s); d.Event.Seq != want {
			t.Errorf("delivered %+v, want the event numbered %d", d.Event, want)
		}
	}
	quiet(t, s)
}

// std: yoke:a-subscription.04
func TestAnOverflowIsFollowedByAFreshSnapshot(t *testing.T) {
	b := bus.New()
	s, _ := b.SubscribeTo(event.Filter{})
	defer s.Close()
	var last event.Event
	for i := range 300 {
		to := unit.Running
		if i == 299 {
			to = unit.Failed
		}
		last, _ = b.Publish(event.StateChanged("acquire", 1, unit.Admitted, to))
	}
	var announced bus.Delivery
	for {
		d := next(t, s)
		if d.Overflow {
			announced = d
			break
		}
	}
	if announced.Snapshot == nil || announced.Snapshot.At != last.Seq || len(announced.Snapshot.Events) != 1 || into(t, announced.Snapshot.Events[0]) != "Failed" {
		t.Fatalf("the overflow was announced with %+v", announced.Snapshot)
	}
	after, _ := b.Publish(event.StateChanged("acquire", 2, "", unit.Starting))
	if d := next(t, s); d.Overflow || d.Event.Seq != after.Seq || after.Seq != last.Seq+1 {
		t.Errorf("after the snapshot it was told %+v, want the event numbered %d", d, last.Seq+1)
	}
}

// std: yoke:a-subscription.05
func TestTheJoinIsExact(t *testing.T) {
	b := bus.New()
	var wg sync.WaitGroup
	lastPublished := make([]string, 10)
	stop := make(chan struct{})
	for u := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			states := []unit.State{unit.Starting, unit.Admitted, unit.Running}
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				to := states[i%3]
				b.Publish(event.StateChanged(fmt.Sprintf("u%d", u), 1, "", to))
				lastPublished[u] = string(to)
				if i%50 == 0 {
					time.Sleep(time.Millisecond)
				}
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	s, snapshot := b.SubscribeTo(event.Filter{})
	defer s.Close()
	held := map[string]string{}
	for _, e := range snapshot.Events {
		held[e.Subject.ID] = into(t, e)
	}
	done := make(chan struct{})
	var mu sync.Mutex
	previous := snapshot.At
	go func() {
		defer close(done)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			d, err := s.Next(ctx)
			cancel()
			if err != nil {
				return
			}
			mu.Lock()
			if d.Overflow {
				previous = d.Snapshot.At
				for _, e := range d.Snapshot.Events {
					held[e.Subject.ID] = into(t, e)
				}
			} else {
				if d.Event.Seq <= previous {
					t.Errorf("the event numbered %d follows %d", d.Event.Seq, previous)
				}
				previous = d.Event.Seq
				held[d.Event.Subject.ID] = into(t, d.Event)
			}
			mu.Unlock()
		}
	}()
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
	<-done
	for u := range 10 {
		id := fmt.Sprintf("u%d", u)
		if held[id] != lastPublished[u] {
			t.Errorf("the subscriber holds %s for %s, and the last change published was %s", held[id], id, lastPublished[u])
		}
	}
}

// std: yoke:a-subscription.06
func TestAStreamsStateIsOneLevelPerStream(t *testing.T) {
	b := bus.New()
	b.Publish(event.StreamActivated("acquire", 1, "station.spectra"))
	b.Publish(event.StreamActivated("acquire", 1, "station.preview"))
	b.Publish(event.StreamStopped("acquire", 1, "station.preview", "asked", true))
	s, snapshot := b.SubscribeTo(event.Filter{})
	defer s.Close()
	var got []string
	for _, e := range snapshot.Events {
		var d struct{ Stream string }
		json.Unmarshal(e.Detail, &d)
		got = append(got, e.Type+" "+d.Stream)
	}
	want := []string{"unit.stream.activated station.spectra", "unit.stream.stopped station.preview"}
	if !slices.Equal(got, want) {
		t.Errorf("the snapshot holds %q, want %q", got, want)
	}
}
