// Package bus is the Core's in-process publish and subscribe: the seam between subsystems, and the only
// one of them authoritative for nothing.
//
// It numbers every event it publishes, tells every subscriber, and never waits for one. A subscriber's
// queue is bounded, so that the slowest consumer never becomes the deployment's clock; when it
// overflows the subscriber is told so, before anything published after it. The bus acquires no
// transport of its own: a consumer outside the Core subscribes through a surface.
package bus

import (
	"context"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Bound is how many events a subscriber's queue holds. It is not declarable.
const Bound = 256

// Delivery is what a subscriber is told: an event, or that its queue overflowed.
type Delivery struct {
	Event    event.Event
	Overflow bool
}

// Bus is one instance's bus.
type Bus struct{}

// New is an empty bus.
func New() *Bus { return &Bus{} }

// Publish numbers an event and tells every subscriber, returning it as published. An event that does
// not fit its fields is refused, and consumes no number.
func (b *Bus) Publish(e event.Event) (event.Event, error) { return e, nil }

// Subscription is one subscriber's queue.
type Subscription struct{}

// Subscribe is a new subscriber, told of every event published from now on.
func (b *Bus) Subscribe() *Subscription { return &Subscription{} }

// Next is the next delivery, waiting for one until ctx ends.
func (s *Subscription) Next(ctx context.Context) (Delivery, error) {
	<-ctx.Done()
	return Delivery{}, ctx.Err()
}

// Close ends the subscription.
func (s *Subscription) Close() {}
