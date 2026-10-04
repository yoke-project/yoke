package interfaces

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// Payload is the most an opaque command, question or answer may hold.
const Payload = 1 << 20

// Wait is how long a unit's acknowledgement or answer is waited for. It is not declarable.
const Wait = 30 * time.Second

// acting are the operations a channel issues against a unit, when the surface has Sessions to carry them.
func (s *Surface) acting() map[string]Operation {
	if s.cfg.Sessions == nil || s.cfg.Declared == nil || s.cfg.Units == nil {
		return nil
	}
	ops := map[string]Operation{"command": {Answer: s.command}, "query": {Answer: s.query}}
	if s.cfg.Transports != nil {
		ops["stream.start"] = Operation{Answer: s.streamStart}
		ops["stream.stop"] = Operation{Answer: s.streamStop}
	}
	return ops
}

func aboutUnit(code, id, message string) *interfacev1.Refusal {
	return &interfacev1.Refusal{Code: code, Message: message, Detail: &interfacev1.Refusal_Subject{Subject: &interfacev1.Subject{Kind: "unit", Identity: id}}}
}

func aboutItem(code, item, message string) *interfacev1.Refusal {
	return &interfacev1.Refusal{Code: code, Message: message, Detail: &interfacev1.Refusal_Item{Item: item}}
}

// target is the unit an operation acts on, checked in the contract's order up to the object: the instance
// not stopping, then the subject — a Plugin unit, the only kind holding a Session — and its Manifest.
func (s *Surface) target(id string) (*gate.Manifest, *interfacev1.Refusal) {
	if s.cfg.Stopping != nil && s.cfg.Stopping() {
		return nil, refusal("instance.stopping", "the instance is stopping, and this operation would act")
	}
	if !slices.Contains(s.cfg.Units.IDs(), id) || s.cfg.Units.Kind(id) != unit.Plugin {
		return nil, aboutUnit("subject.unknown", id, "nothing this channel may address is the unit "+id)
	}
	m, ok := s.cfg.Declared(id)
	if !ok {
		return nil, aboutUnit("subject.unknown", id, "nothing this channel may address is the unit "+id)
	}
	return m, nil
}

// reachable checks that the unit can be issued anything: running, and holding a Session.
func (s *Surface) reachable(id string) (uint64, *interfacev1.Refusal) {
	st := s.cfg.Units.Status(id)
	if st.State == "" || st.State.Terminal() {
		return 0, aboutUnit("unit.not_running", id, id+" is not running")
	}
	if !s.cfg.Sessions.Open(id) {
		return 0, aboutUnit("unit.no_session", id, id+" is running and holds no Session")
	}
	return uint64(st.Incarnation), nil
}

// waiting is the bounded wait for a unit's answer.
func (s *Surface) waiting(ctx context.Context) (context.Context, context.CancelFunc) {
	wait := s.cfg.Wait
	if wait == 0 {
		wait = Wait
	}
	return context.WithTimeout(ctx, wait)
}

// failed turns what came back from a Session into the refusal it is, naming the object where scope was
// the reason.
func failed(err error, id, object string) *interfacev1.Refusal {
	var refused *session.Refused
	var unitFailed *session.UnitFailed
	switch {
	case errors.As(err, &refused) && refused.Code == pluginv1.Code_CODE_SCOPE_WITHHELD:
		return aboutItem("scope.withheld", object, object+" is declared and not granted to that plugin")
	case errors.As(err, &refused) && refused.Code == pluginv1.Code_CODE_SCOPE_UNDECLARED:
		return aboutItem("scope.undeclared", object, object+" was never declared by the unit's Plugin")
	case errors.As(err, &unitFailed):
		return aboutItem("unit.failed", unitFailed.Code, unitFailed.Message)
	case errors.As(err, &refused):
		return refusal("operation.malformed", refused.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return aboutUnit("unit.unanswered", id, id+" did not answer in time")
	}
	return aboutUnit("unit.no_session", id, err.Error())
}

func acknowledged(a *pluginv1.Ack) *interfacev1.Acknowledged {
	return &interfacev1.Acknowledged{Outcome: interfacev1.Acknowledged_Outcome(a.GetOutcome()), Line: a.GetLine()}
}

// command issues a control message on the unit's Session and answers with its acknowledgement. The
// payload is carried and never read; the type is read, because the capability governing it is named
// against it.
func (s *Surface) command(ctx context.Context, r *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal) {
	c := r.GetCommand()
	if len(c.GetPayload()) > Payload {
		return nil, refusal("operation.malformed", "a command is bounded at 1 MiB")
	}
	m, ref := s.target(c.GetUnit())
	if ref != nil {
		return nil, ref
	}
	if !slices.Contains(m.Commands, c.GetType()) {
		return nil, aboutItem("scope.undeclared", c.GetType(), c.GetType()+" was never declared by the unit's Plugin")
	}
	if _, ref := s.reachable(c.GetUnit()); ref != nil {
		return nil, ref
	}
	wait, cancel := s.waiting(ctx)
	defer cancel()
	ack, err := s.cfg.Sessions.Instruct(wait, c.GetUnit(), &pluginv1.Control{Kind: &pluginv1.Control_Command_{Command: &pluginv1.Control_Command{Type: c.GetType(), Payload: c.GetPayload()}}})
	if err != nil {
		return nil, failed(err, c.GetUnit(), c.GetType())
	}
	return &interfacev1.Response{Answer: &interfacev1.Response_Command{Command: acknowledged(ack)}}, nil
}

// query issues a question on the unit's Session and answers with the unit's bytes, unread.
func (s *Surface) query(ctx context.Context, r *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal) {
	q := r.GetQuery()
	if len(q.GetPayload()) > Payload {
		return nil, refusal("operation.malformed", "a question is bounded at 1 MiB")
	}
	m, ref := s.target(q.GetUnit())
	if ref != nil {
		return nil, ref
	}
	if !slices.Contains(m.Queries, q.GetType()) {
		return nil, aboutItem("scope.undeclared", q.GetType(), q.GetType()+" was never declared by the unit's Plugin")
	}
	if _, ref := s.reachable(q.GetUnit()); ref != nil {
		return nil, ref
	}
	wait, cancel := s.waiting(ctx)
	defer cancel()
	a, err := s.cfg.Sessions.Ask(wait, q.GetUnit(), &pluginv1.Query_Question{Type: q.GetType(), Payload: q.GetPayload()})
	if err != nil {
		return nil, failed(err, q.GetUnit(), q.GetType())
	}
	if len(a.GetPayload()) > Payload {
		return nil, aboutUnit("unit.unanswered", q.GetUnit(), "the answer exceeds 1 MiB, and is not carried")
	}
	return &interfacev1.Response{Answer: &interfacev1.Response_Query{Query: &interfacev1.Answered{Payload: a.GetPayload()}}}, nil
}

// stream checks a stream of a unit up to its Session, and returns what it tolerates and the unit's life.
func (s *Surface) stream(id, stream string) (streams.Tolerances, uint64, *interfacev1.Refusal) {
	m, ref := s.target(id)
	if ref != nil {
		return streams.Tolerances{}, 0, ref
	}
	i := slices.IndexFunc(m.Streams, func(d gate.Stream) bool { return d.ID == stream })
	if i < 0 {
		return streams.Tolerances{}, 0, aboutItem("scope.undeclared", stream, stream+" was never declared by the unit's Plugin")
	}
	incarnation, ref := s.reachable(id)
	if ref != nil {
		return streams.Tolerances{}, 0, ref
	}
	return streams.Tolerances{Loss: m.Streams[i].ToleratesLoss, Reorder: m.Streams[i].ToleratesReorder}, incarnation, nil
}

func done() *interfacev1.Acknowledged {
	return &interfacev1.Acknowledged{Outcome: interfacev1.Acknowledged_OUTCOME_DONE}
}

// streamStart creates the transport and listens on it, then activates the stream on the unit's Session.
// A stream already flowing is answered as done, and nothing is issued.
func (s *Surface) streamStart(ctx context.Context, r *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal) {
	id, stream := r.GetStreamStart().GetUnit(), r.GetStreamStart().GetStream()
	answer := func(a *interfacev1.Acknowledged) *interfacev1.Response {
		return &interfacev1.Response{Answer: &interfacev1.Response_StreamStart{StreamStart: a}}
	}
	tolerates, incarnation, ref := s.stream(id, stream)
	if ref != nil {
		return nil, ref
	}
	if slices.Contains(s.cfg.Transports.Active(id), stream) {
		return answer(done()), nil
	}
	transport, address, err := s.cfg.Transports.Open(id, incarnation, stream, tolerates)
	if err != nil {
		return nil, aboutUnit("unit.no_session", id, fmt.Sprintf("the transport of %s could not be created: %v", stream, err))
	}
	kind := pluginv1.Control_Activate_TRANSPORT_ORDERED
	if transport == streams.Framed {
		kind = pluginv1.Control_Activate_TRANSPORT_FRAMED
	}
	wait, cancel := s.waiting(ctx)
	defer cancel()
	ack, err := s.cfg.Sessions.Instruct(wait, id, &pluginv1.Control{Kind: &pluginv1.Control_Activate_{Activate: &pluginv1.Control_Activate{Stream: stream, Transport: kind, Address: address}}})
	if err != nil {
		s.cfg.Transports.Discard(id, stream)
		return nil, failed(err, id, stream)
	}
	if ack.GetOutcome() == pluginv1.Ack_OUTCOME_FAILED {
		s.cfg.Transports.Discard(id, stream)
		return answer(acknowledged(ack)), nil
	}
	s.publish(event.StreamActivated(id, incarnation, stream))
	return answer(acknowledged(ack)), nil
}

// streamStop instructs the unit, then removes the transport whatever it answered. A stream not flowing is
// answered as done, and nothing is issued.
func (s *Surface) streamStop(ctx context.Context, r *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal) {
	id, stream := r.GetStreamStop().GetUnit(), r.GetStreamStop().GetStream()
	answer := func(a *interfacev1.Acknowledged) *interfacev1.Response {
		return &interfacev1.Response{Answer: &interfacev1.Response_StreamStop{StreamStop: a}}
	}
	if _, _, ref := s.stream(id, stream); ref != nil {
		return nil, ref
	}
	if !slices.Contains(s.cfg.Transports.Active(id), stream) {
		return answer(done()), nil
	}
	wait, cancel := s.waiting(ctx)
	defer cancel()
	ack, err := s.cfg.Sessions.Instruct(wait, id, &pluginv1.Control{Kind: &pluginv1.Control_Stop_{Stop: &pluginv1.Control_Stop{Stream: stream}}})
	s.cfg.Transports.Close(id, stream, streams.Asked)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, failed(err, id, stream)
	}
	if ack == nil {
		return answer(done()), nil
	}
	return answer(acknowledged(ack)), nil
}
