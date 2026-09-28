// Package bus is the Core's in-process publish and subscribe: the seam between subsystems, and the only
// one of them authoritative for nothing.
//
// It numbers every event it publishes, tells every subscriber, and never waits for one. A subscriber's
// queue is bounded, so that the slowest consumer never becomes the deployment's clock; when it
// overflows the subscriber is told so, before anything published after it. The bus acquires no
// transport of its own: a consumer outside the Core subscribes through a surface.
package bus

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Bound is how many events a subscriber's queue holds. It is not declarable.
const Bound = 256

// Delivery is what a subscriber is told: an event, or that its queue overflowed and the picture as it
// now is.
type Delivery struct {
	Event    event.Event
	Overflow bool
	Snapshot *Snapshot
}

// Snapshot is the current value of every level a filter selects, taken at a sequence: the stream that
// follows it begins at the next.
type Snapshot struct {
	At     uint64
	Events []event.Event
}

// SubscribeTo is a new subscriber, told of what the filter selects from now on, and the snapshot it
// opens with: both are taken at one point, so every event is in the snapshot or after it, and never in
// both.
func (b *Bus) SubscribeTo(f event.Filter) (*Subscription, Snapshot) {
	s := &Subscription{bus: b, filter: f, wake: make(chan struct{}, 1)}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[s] = true
	return s, b.snapshot(f)
}

// snapshot is the current value of every level f selects, at the last sequence published. Lock held.
func (b *Bus) snapshot(f event.Filter) Snapshot {
	snap := Snapshot{At: b.seq}
	for _, e := range b.current {
		if f.Selects(e) {
			snap.Events = append(snap.Events, e)
		}
	}
	slices.SortFunc(snap.Events, func(a, b event.Event) int { return cmp.Compare(a.Seq, b.Seq) })
	return snap
}

// level is what makes an event the current value of something: its type and what it is about. A unit's
// value is its current life's, so the life is not part of it.
type level struct {
	typ  string
	kind event.Kind
	id   string
}

// Bus is one instance's bus.
type Bus struct {
	mu      sync.Mutex
	seq     uint64
	subs    map[*Subscription]bool
	current map[level]event.Event // the last event of every level, which a snapshot is made of
}

// New is an empty bus.
func New() *Bus { return &Bus{subs: map[*Subscription]bool{}, current: map[level]event.Event{}} }

// Publish numbers an event and tells every subscriber, returning it as published. An event that does
// not fit its fields is refused, and consumes no number. Publication is one point, so the numbers are in
// the order the Core concluded.
func (b *Bus) Publish(e event.Event) (event.Event, error) {
	if err := e.Check(); err != nil {
		return e, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	e.Seq = b.seq
	if class, _ := event.ClassOf(e.Type); class == event.Level {
		b.current[level{e.Type, e.Subject.Kind, e.Subject.ID}] = e
	}
	for s := range b.subs {
		if s.filter.Selects(e) {
			s.offer(e)
		}
	}
	return e, nil
}

// Subscription is one subscriber's queue.
type Subscription struct {
	bus    *Bus
	filter event.Filter
	mu     sync.Mutex
	queue  []event.Event
	lost   bool // the queue overflowed, and the subscriber has not been told
	closed bool
	wake   chan struct{}
}

// Subscribe is a new subscriber, told of every event published from now on.
func (b *Bus) Subscribe() *Subscription {
	s, _ := b.SubscribeTo(event.Filter{})
	return s
}

// offer queues an event, or loses it when the queue is full. After a loss nothing more is queued until
// the subscriber has been told of it, so the announcement comes before anything published later.
func (s *Subscription) offer(e event.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case len(s.queue) >= Bound:
		s.lost = true
	case s.lost:
		return
	default:
		s.queue = append(s.queue, e)
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// ErrClosed is what Next answers once the subscription ended.
var ErrClosed = errors.New("the subscription is closed")

// Next is the next delivery, waiting for one until ctx ends. What was queued before an overflow is
// delivered first, then the announcement, then what is published after it.
func (s *Subscription) Next(ctx context.Context) (Delivery, error) {
	for {
		s.mu.Lock()
		switch {
		case s.closed:
			s.mu.Unlock()
			return Delivery{}, ErrClosed
		case len(s.queue) > 0:
			e := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return Delivery{Event: e}, nil
		case s.lost:
			// The fresh snapshot is taken where no event can be published, and the stream resumes after
			// it: the bus is locked before this subscription, as publishing locks them.
			s.mu.Unlock()
			s.bus.mu.Lock()
			s.mu.Lock()
			if !s.lost || len(s.queue) > 0 {
				s.mu.Unlock()
				s.bus.mu.Unlock()
				continue
			}
			s.lost = false
			snap := s.bus.snapshot(s.filter)
			s.mu.Unlock()
			s.bus.mu.Unlock()
			return Delivery{Overflow: true, Snapshot: &snap}, nil
		}
		s.mu.Unlock()
		select {
		case <-s.wake:
		case <-ctx.Done():
			return Delivery{}, ctx.Err()
		}
	}
}

// Close ends the subscription.
func (s *Subscription) Close() {
	s.bus.mu.Lock()
	delete(s.bus.subs, s)
	s.bus.mu.Unlock()
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
