package interfaces

import (
	"slices"
	"sync"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/gate"
)

// The reasons a channel is suspended: a channel prevailing over it holds, or a local channel reclaimed
// control from it.
const (
	Displaced = "displaced"
	Reclaimed = "reclaimed"
)

// Arbiter decides which channels are suspended, from the declared rules and which channels hold. It is
// computed on every change to what holds or what reclaimed, never on a clock, and it is total and
// stateless: the same inputs give the same suspensions however often it runs.
type Arbiter struct {
	mu       sync.Mutex
	rules    []gate.Rule
	channels map[string]*arbitrated
	order    []string
	publish  func(event.Event)
}

type arbitrated struct {
	local     bool   // bound as a local socket: the one class that may reclaim
	grade     string // what this channel keeps while suspended
	holding   bool
	reclaimed bool
	suspended bool
	by        string
	reason    string
}

// NewArbiter is an arbiter over the channels and rules declared, with nothing holding.
func NewArbiter(channels []gate.Channel, rules []gate.Rule, publish func(event.Event)) *Arbiter {
	a := &Arbiter{rules: rules, channels: map[string]*arbitrated{}, publish: publish}
	for _, ch := range channels {
		grade := ch.OnSuspend
		if grade == "" {
			grade = "read-only"
		}
		a.channels[ch.Name] = &arbitrated{local: ch.Address == nil || ch.Address.Class == "local", grade: grade}
		a.order = append(a.order, ch.Name)
	}
	slices.Sort(a.order)
	return a
}

// Hold says whether a channel holds: attached, and its picture current. A channel that stops holding
// also stops reclaiming.
func (a *Arbiter) Hold(name string, holds bool) {
	a.mu.Lock()
	c, ok := a.channels[name]
	if !ok {
		a.mu.Unlock()
		return
	}
	c.holding = holds
	if !holds {
		c.reclaimed = false
	}
	events := a.recompute()
	a.mu.Unlock()
	a.tell(events)
}

// Reclaim takes control back for a channel bound as a local socket, and says whether anything changed.
func (a *Arbiter) Reclaim(name string) bool {
	a.mu.Lock()
	c, ok := a.channels[name]
	if !ok || !c.local {
		a.mu.Unlock()
		return false
	}
	c.reclaimed = true
	events := a.recompute()
	a.mu.Unlock()
	a.tell(events)
	return len(events) > 0
}

// State is a channel's suspension: whether, in which grade, by which channel, and why.
func (a *Arbiter) State(name string) (suspended bool, grade, by, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.channels[name]
	if !ok || !c.suspended {
		return false, "", "", ""
	}
	return true, c.grade, c.by, c.reason
}

// recompute derives the suspensions from the rules, inverting a rule wherever a local channel it would
// suspend has reclaimed and holds, and returns the events for what changed. Lock held.
func (a *Arbiter) recompute() []event.Event {
	type edge struct{ prevails, over, reason string }
	var edges []edge
	for _, r := range a.rules {
		for _, d := range r.Over {
			if c := a.channels[d]; c != nil && c.reclaimed && c.local && c.holding {
				edges = append(edges, edge{d, r.Prevails, Reclaimed})
			} else {
				edges = append(edges, edge{r.Prevails, d, Displaced})
			}
		}
	}
	var events []event.Event
	for _, name := range a.order {
		c := a.channels[name]
		suspended, by, reason := false, "", ""
		for _, e := range edges {
			if p := a.channels[e.prevails]; e.over == name && p != nil && p.holding {
				suspended, by, reason = true, e.prevails, e.reason
				break
			}
		}
		switch {
		case suspended && (!c.suspended || c.by != by || c.reason != reason):
			events = append(events, event.ChannelSuspended(name, reason, by, c.grade))
		case !suspended && c.suspended:
			events = append(events, event.ChannelResumed(name))
		}
		c.suspended, c.by, c.reason = suspended, by, reason
	}
	return events
}

func (a *Arbiter) tell(events []event.Event) {
	if a.publish == nil {
		return
	}
	for _, e := range events {
		a.publish(e)
	}
}

// withdrawn says whether a grade withdraws an operation. reclaim is withdrawn by none, and neither are
// read, subscribe, confirm and authenticate.
func withdrawn(grade, operation string) bool {
	switch operation {
	case "command", "stream.start", "stream.stop":
		return true
	case "query", "stream.subscribe", "stream.unsubscribe":
		return grade == "dark"
	}
	return false
}
