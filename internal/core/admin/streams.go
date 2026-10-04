package admin

import (
	"context"
	"errors"
	"fmt"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/streams"
)

// A stream's state, as a change answers what it replaced.
const (
	streamStopped   = "stopped"
	streamActivated = "activated"
)

// stream checks that a unit names a stream its Plugin declared, and that the unit can be instructed, in
// the order a refusal is decided in; it returns what the stream tolerates and the unit's life.
func (c *Core) stream(id, stream string) (streams.Tolerances, uint64, *administrativev1.Refusal) {
	if ref := c.unit(id); ref != nil {
		return streams.Tolerances{}, 0, ref
	}
	plugin, _ := c.Units.Plugin(id)
	var declared *streams.Tolerances
	if m, ok := c.Manifest(plugin); ok && plugin != "" {
		for _, s := range m.Streams {
			if s.ID == stream {
				declared = &streams.Tolerances{Loss: s.ToleratesLoss, Reorder: s.ToleratesReorder}
			}
		}
	}
	if declared == nil {
		return streams.Tolerances{}, 0, &administrativev1.Refusal{Code: "stream.undeclared", Message: fmt.Sprintf("%s's Plugin declares no stream %s", id, stream),
			Detail: &administrativev1.Refusal_Item{Item: stream}}
	}
	st := c.Units.Status(id)
	if !live(st.State) {
		return streams.Tolerances{}, 0, about("unit.not_running", "unit", id, id+" is not running")
	}
	if !c.Sessions.Open(id) {
		return streams.Tolerances{}, 0, about("unit.no_session", "unit", id, id+" is running and holds no Session")
	}
	return *declared, uint64(st.Incarnation), nil
}

// instruct hands a unit a control instruction about one of its streams, and turns what came back into a
// refusal where it is one.
func (c *Core) instruct(ctx context.Context, id, stream string, control *pluginv1.Control) (*pluginv1.Ack, *administrativev1.Refusal) {
	wait := c.Wait
	if wait == 0 {
		wait = Wait
	}
	waiting, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	ack, err := c.Sessions.Instruct(waiting, id, control)
	var refused *session.Refused
	switch {
	case errors.As(err, &refused) && refused.Code == pluginv1.Code_CODE_SCOPE_WITHHELD:
		return nil, &administrativev1.Refusal{Code: "scope.withheld", Message: refused.Error(), Detail: &administrativev1.Refusal_Item{Item: stream}}
	case errors.As(err, &refused):
		return nil, refusal("operation.malformed", refused.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return nil, about("unit.unanswered", "unit", id, fmt.Sprintf("%s did not answer within %v", id, wait))
	case err != nil:
		return nil, about("unit.no_session", "unit", id, err.Error())
	}
	return ack, nil
}

func flowing(previously string) *administrativev1.Changed {
	return immediately(&administrativev1.Previously{Value: &administrativev1.Previously_State{State: previously}})
}

// streamStart creates the stream's transport and listens on it, then activates the stream on the unit's
// Session: the unit is told only once there is somewhere to emit into.
func (c *Core) streamStart(ctx context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
	id, stream := r.GetUnitStreamStart().GetUnit(), r.GetUnitStreamStart().GetStream()
	tolerates, incarnation, ref := c.stream(id, stream)
	if ref != nil {
		return nil, ref
	}
	answer := func(change *administrativev1.Changed) *administrativev1.Response {
		return &administrativev1.Response{Answer: &administrativev1.Response_UnitStreamStart{UnitStreamStart: change}}
	}
	if c.flows(id, stream) {
		return answer(flowing(streamActivated)), nil
	}
	transport, address, err := c.Streams.Open(id, incarnation, stream, tolerates)
	if err != nil {
		return nil, about("unit.no_session", "unit", id, fmt.Sprintf("the transport of %s could not be created: %v", stream, err))
	}
	kind := pluginv1.Control_Activate_TRANSPORT_ORDERED
	if transport == streams.Framed {
		kind = pluginv1.Control_Activate_TRANSPORT_FRAMED
	}
	ack, ref := c.instruct(ctx, id, stream, &pluginv1.Control{Kind: &pluginv1.Control_Activate_{Activate: &pluginv1.Control_Activate{
		Stream: stream, Transport: kind, Address: address}}})
	if ref != nil {
		c.Streams.Discard(id, stream)
		return nil, ref
	}
	change := flowing(streamStopped)
	if ack.GetOutcome() == pluginv1.Ack_OUTCOME_FAILED {
		// The unit declined: nothing flows, and its transport goes with nothing published about a flow
		// that never began.
		c.Streams.Discard(id, stream)
		change.Consequences = []*administrativev1.Consequence{{Unit: id, Incarnation: incarnation, What: "the unit failed the activation of " + stream + ": " + ack.GetLine()}}
		return answer(change), nil
	}
	c.record(actor, event.Unit, id, incarnation, "started the stream "+stream+" of "+id)
	if c.Publish != nil {
		c.Publish(event.StreamActivated(id, incarnation, stream))
	}
	change.Consequences = []*administrativev1.Consequence{{Unit: id, Incarnation: incarnation, What: "activated " + stream}}
	return answer(change), nil
}

// streamStop instructs the unit to stop the stream, then removes the transport whatever it answered:
// the transport was never the unit's to keep.
func (c *Core) streamStop(ctx context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
	id, stream := r.GetUnitStreamStop().GetUnit(), r.GetUnitStreamStop().GetStream()
	_, incarnation, ref := c.stream(id, stream)
	if ref != nil {
		return nil, ref
	}
	answer := func(change *administrativev1.Changed) *administrativev1.Response {
		return &administrativev1.Response{Answer: &administrativev1.Response_UnitStreamStop{UnitStreamStop: change}}
	}
	if !c.flows(id, stream) {
		return answer(flowing(streamStopped)), nil
	}
	_, ref = c.instruct(ctx, id, stream, &pluginv1.Control{Kind: &pluginv1.Control_Stop_{Stop: &pluginv1.Control_Stop{Stream: stream}}})
	c.Streams.Close(id, stream, streams.Asked)
	if ref != nil && ref.GetCode() != "unit.unanswered" {
		return nil, ref
	}
	c.record(actor, event.Unit, id, incarnation, "stopped the stream "+stream+" of "+id)
	change := flowing(streamActivated)
	change.Consequences = []*administrativev1.Consequence{{Unit: id, Incarnation: incarnation, What: "stopped " + stream}}
	return answer(change), nil
}

// flows says whether a unit's stream has its transport open.
func (c *Core) flows(id, stream string) bool {
	if c.Streams == nil {
		return false
	}
	for _, s := range c.Streams.Active(id) {
		if s == stream {
			return true
		}
	}
	return false
}
