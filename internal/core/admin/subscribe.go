package admin

import (
	"context"
	"slices"
	"strings"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Subscriptions is the most one shell connection holds, the standing one included.
const Subscriptions = 8

// filterOf is a filter as the bus reads it.
func filterOf(f *administrativev1.Filter) event.Filter {
	return event.Filter{SubjectKind: event.Kind(f.GetSubjectKind()), SubjectID: f.GetSubjectIdentity(), Floor: int(f.GetFloor()),
		Type: f.GetType(), TypePrefix: f.GetTypePrefix(), Occurrence: f.GetOccurrence(), OccurrencePrefix: f.GetOccurrencePrefix()}
}

// eventOf is an event as it travels.
func eventOf(e event.Event) *administrativev1.Event {
	return &administrativev1.Event{Seq: e.Seq, Type: e.Type, Time: stamp(e.Time), Severity: uint32(e.Severity), Occurrence: e.Occurrence,
		Cause: e.Cause, Detail: e.Detail, Subject: &administrativev1.Subject{Kind: string(e.Subject.Kind), Identity: e.Subject.ID, Incarnation: e.Subject.Incarnation},
		Actor: &administrativev1.Actor{Class: string(e.Actor.Class), Person: e.Actor.Person}}
}

// selected are the records of every subject a filter's subject axis selects: the kind it names, or the
// kind a type names by its first segment, or every kind. Severity, type and occurrence select events, and
// a record is none.
func (c *Core) selected(f *administrativev1.Filter) []*administrativev1.Record {
	kind := f.GetSubjectKind()
	if kind == "" && f.GetType() != "" {
		kind, _, _ = strings.Cut(f.GetType(), ".")
	}
	from := kinds
	if kind != "" {
		from = []string{kind}
	}
	var out []*administrativev1.Record
	for _, k := range from {
		records, _ := c.Records(k, f.GetSubjectIdentity())
		out = append(out, records...)
	}
	return out
}

// subscribe projects the bus outwards: a snapshot of records at a sequence, then the events the filter
// selects from the next, and an overflow announced with a fresh snapshot.
func (c *Core) subscribe(ctx context.Context, _ event.Actor, r *administrativev1.Request, send func(*administrativev1.Response) error) *administrativev1.Refusal {
	f := r.GetSubscribe().GetFilter()
	if k := f.GetSubjectKind(); k != "" && !slices.Contains(kinds, k) {
		return refusal("operation.malformed", "a filter names one of the six subject kinds, and "+k+" is none")
	}
	answer := func(s *administrativev1.Subscribed) error {
		return send(&administrativev1.Response{Answer: &administrativev1.Response_Subscribe{Subscribe: s}})
	}
	// The subscription opens first, so that nothing concluded while the records are assembled is missed.
	sub, snap := c.Bus.SubscribeTo(filterOf(f))
	defer sub.Close()
	if answer(&administrativev1.Subscribed{Carries: &administrativev1.Subscribed_Snapshot{Snapshot: &administrativev1.Snapshot{At: snap.At, Records: c.selected(f)}}}) != nil {
		return nil
	}
	for {
		d, err := sub.Next(ctx)
		if err != nil {
			return nil
		}
		carries := &administrativev1.Subscribed{Carries: &administrativev1.Subscribed_Event{Event: eventOf(d.Event)}}
		if d.Overflow {
			carries.Carries = &administrativev1.Subscribed_Overflow{Overflow: &administrativev1.Snapshot{At: d.Snapshot.At, Records: c.selected(f)}}
		}
		if answer(carries) != nil {
			return nil
		}
	}
}
