package interfaces

import (
	"context"
	"fmt"
	"sync"
	"time"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
)

// attachment is one client attached to a channel, whatever projection carries it: the client the channel
// established, the live shape it is told things on, the calls in flight, its confirmed position and the
// deliveries it holds. Nothing about it outlives it.
type attachment struct {
	s      *Surface
	client string
	ctx    context.Context
	end    context.CancelFunc
	live   func(*interfacev1.CoreFrame) error // the live shape: events, deliveries, a stream's later answers

	mu       sync.Mutex
	inFlight map[string]context.CancelFunc
	calls    sync.WaitGroup

	confirmed sync.Mutex
	last      time.Time
	stale     bool

	held    *deliveries
	opening *interfacev1.Opening
}

// begin attaches a client: a single channel already held refuses it; otherwise the attachment is
// announced, counted as current, and its standing subscription opened before its picture is assembled.
func (s *Surface) begin(parent context.Context, client string, live func(*interfacev1.CoreFrame) error) (*attachment, *interfacev1.Refusal) {
	if !s.attach(client) {
		return nil, refusal("channel.in_use", "the channel "+s.cfg.Channel.Name+" takes one client, and one is attached")
	}
	s.publish(event.ChannelAttached(s.cfg.Channel.Name, client))
	ctx, end := context.WithCancel(parent)
	var sending sync.Mutex
	a := &attachment{s: s, client: client, ctx: ctx, end: end, inFlight: map[string]context.CancelFunc{}, last: time.Now(), held: &deliveries{},
		live: func(f *interfacev1.CoreFrame) error {
			sending.Lock()
			defer sending.Unlock()
			return live(f)
		}}
	s.hold(1)

	a.opening = &interfacev1.Opening{Picture: &interfacev1.Snapshot{}, Version: Version}
	if s.cfg.Bus != nil {
		sub, snap := s.cfg.Bus.SubscribeTo(event.Filter{})
		a.opening.Picture = &interfacev1.Snapshot{At: snap.At, Records: s.records()}
		a.opening.Subscription = Standing
		a.calls.Add(1)
		go a.standing(sub)
	} else if s.cfg.Picture != nil {
		a.opening.Picture = s.cfg.Picture()
	}
	if c := s.cfg.Confirm; c != nil && c.Required[s.cfg.Channel.Name] {
		a.calls.Add(1)
		go a.watch(c)
	}
	return a, nil
}

// standing carries what the channel observes on the subscription that stands from the attachment on.
func (a *attachment) standing(sub *bus.Subscription) {
	defer a.calls.Done()
	defer sub.Close()
	for {
		d, err := sub.Next(a.ctx)
		if err != nil {
			return
		}
		if d.Overflow {
			a.live(&interfacev1.CoreFrame{Call: Standing, Carries: &interfacev1.CoreFrame_Answer{Answer: &interfacev1.Response{Answer: &interfacev1.Response_Subscribe{
				Subscribe: &interfacev1.Subscribed{Carries: &interfacev1.Subscribed_Overflow{Overflow: &interfacev1.Snapshot{At: d.Snapshot.At, Records: a.s.records()}}}}}}})
			continue
		}
		if a.s.observes(d.Event) {
			a.live(&interfacev1.CoreFrame{Call: Standing, Carries: &interfacev1.CoreFrame_Event{Event: eventOf(d.Event)}})
		}
	}
}

// watch holds a confirmed subscription to its figures: past the tolerance's worth of intervals with no
// confirmation it is stale, and a stale attachment stops counting toward the channel holding.
func (a *attachment) watch(c *Confirmation) {
	defer a.calls.Done()
	ticker := time.NewTicker(c.Every)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
		a.confirmed.Lock()
		due := !a.stale && time.Since(a.last) > c.Every*time.Duration(c.Tolerance)
		since := a.last
		if due {
			a.stale = true
		}
		a.confirmed.Unlock()
		if due {
			a.s.publish(event.SubscriptionStale(a.s.cfg.Channel.Name, since))
			a.s.hold(-1)
		}
	}
}

// finish ends the attachment: its calls, its deliveries, its place in the channel, and then the
// announcement of why.
func (a *attachment) finish(reason string) {
	a.end()
	a.calls.Wait()
	a.held.all()
	a.s.detach(a.client)
	a.confirmed.Lock()
	current := !a.stale
	a.stale = true
	a.confirmed.Unlock()
	if current {
		a.s.hold(-1)
	}
	a.s.publish(event.ChannelDetached(a.s.cfg.Channel.Name, a.client, reason))
}

// cancel ends a call in flight; the call completes, ended by the caller.
func (a *attachment) cancel(call string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if stop, busy := a.inFlight[call]; busy {
		stop()
	}
}

// handle runs one call. Its first frame goes to reply; a stream's later frames travel on the live shape,
// which on the typed projection is the same stream.
func (a *attachment) handle(call string, r *interfacev1.Request, reply func(*interfacev1.CoreFrame) error) {
	s := a.s
	answer := func(resp *interfacev1.Response) {
		reply(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Answer{Answer: resp}})
	}
	refuse := func(ref *interfacev1.Refusal) {
		reply(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Refusal{Refusal: ref}})
	}
	if r.GetVersion() != Version {
		refuse(refusal("compat.unsupported", fmt.Sprintf("this Core speaks the contract's version %d, and the request states %d", Version, r.GetVersion())))
		return
	}
	name := Name(r)
	if name == "" {
		refuse(refusal("operation.malformed", "the request names no operation"))
		return
	}
	a.mu.Lock()
	_, busy := a.inFlight[call]
	a.mu.Unlock()
	if busy {
		refuse(refusal("operation.malformed", "the call "+call+" is already in flight on this attachment"))
		return
	}
	switch name {
	case "authenticate":
		// On a channel whose class establishes the caller, the answer is who it established; a routable
		// channel, which would take a credential here, is not bound by this Core.
		answer(&interfacev1.Response{Answer: &interfacev1.Response_Authenticate{Authenticate: &interfacev1.Authenticated{Account: a.client}}})
		return
	case "confirm":
		// The position is the attachment's own, so confirming is answered here.
		a.confirmed.Lock()
		wasStale := a.stale
		a.last, a.stale = time.Now(), false
		a.confirmed.Unlock()
		if wasStale {
			s.hold(1)
		}
		answer(&interfacev1.Response{Answer: &interfacev1.Response_Confirm{Confirm: &interfacev1.Confirmed{}}})
		return
	case "reclaim":
		// Never withdrawn by any grade: it is the floor that ends a suspension a person at the machine
		// cannot otherwise end.
		if s.cfg.Channel.Address != nil && s.cfg.Channel.Address.Class != "local" {
			refuse(refusal("channel.not_local", "the channel "+s.cfg.Channel.Name+" is not bound as a local socket, and may not reclaim"))
			return
		}
		changed := s.cfg.Arbiter != nil && s.cfg.Arbiter.Reclaim(s.cfg.Channel.Name)
		answer(&interfacev1.Response{Answer: &interfacev1.Response_Reclaim{Reclaim: &interfacev1.Reclaimed{Channel: s.channelRecord(), Changed: changed}}})
		return
	}
	if withdrawn("", name) || withdrawn("dark", name) {
		if s.cfg.Stopping != nil && s.cfg.Stopping() {
			refuse(refusal("instance.stopping", "the instance is stopping, and "+name+" would act"))
			return
		}
		if s.cfg.Arbiter != nil {
			if suspended, grade, by, _ := s.cfg.Arbiter.State(s.cfg.Channel.Name); suspended && withdrawn(grade, name) {
				refuse(&interfacev1.Refusal{Code: "channel.suspended", Message: "the channel " + s.cfg.Channel.Name + " is suspended " + grade + " in favour of " + by,
					Detail: &interfacev1.Refusal_Suspension{Suspension: &interfacev1.Suspension{Grade: grade, By: by}}})
				return
			}
		}
	}
	switch name {
	case "stream.subscribe":
		// A delivery is the attachment's, and ends with it.
		resp, ref := s.subscribeStream(r, a)
		if ref != nil {
			refuse(ref)
			return
		}
		answer(resp)
		return
	case "stream.unsubscribe":
		if !a.held.drop(r.GetStreamUnsubscribe().GetDelivery()) {
			refuse(refusal("operation.malformed", "this attachment holds no delivery "+r.GetStreamUnsubscribe().GetDelivery()))
			return
		}
		answer(&interfacev1.Response{Answer: &interfacev1.Response_StreamUnsubscribe{StreamUnsubscribe: &interfacev1.Released{}}})
		return
	}
	op, served := s.cfg.Operations[name]
	if !served || (op.Answer == nil && op.Stream == nil) {
		refuse(refusal("operation.unknown", "this Core does not serve "+name))
		return
	}
	callCtx, stop := context.WithCancel(a.ctx)
	a.mu.Lock()
	a.inFlight[call] = stop
	a.mu.Unlock()
	a.calls.Add(1)
	go func() {
		defer a.calls.Done()
		defer func() {
			a.mu.Lock()
			delete(a.inFlight, call)
			a.mu.Unlock()
			stop()
		}()
		if op.Answer != nil {
			resp, ref := op.Answer(callCtx, r)
			if ref != nil {
				refuse(ref)
				return
			}
			answer(resp)
			return
		}
		first := true
		ref := op.Stream(callCtx, r, func(resp *interfacev1.Response) error {
			if callCtx.Err() != nil {
				return callCtx.Err()
			}
			f := &interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Answer{Answer: resp}}
			if first {
				first = false
				return reply(f)
			}
			return a.live(f)
		})
		if ref != nil {
			if first {
				refuse(ref)
			} else {
				a.live(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Refusal{Refusal: ref}})
			}
			return
		}
		by := interfacev1.Completion_BY_CORE
		if callCtx.Err() != nil && a.ctx.Err() == nil {
			by = interfacev1.Completion_BY_CALLER
		}
		a.live(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Completion{Completion: &interfacev1.Completion{By: by}}})
	}()
}
