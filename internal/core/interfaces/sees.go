package interfaces

import (
	"context"
	"slices"
	"strings"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
)

// kinds are the six subject kinds; a channel observes the first three.
var (
	kinds    = []string{"instance", "unit", "channel", "plugin", "document", "connection"}
	observed = []string{"instance", "unit", "channel"}
)

// recordsOf are the records of a kind the channel observes, or of the one subject named; a kind it does
// not observe matches nothing.
func (s *Surface) recordsOf(kind, identity string) ([]*interfacev1.Record, *interfacev1.Refusal) {
	if !slices.Contains(kinds, kind) {
		return nil, refusal("operation.malformed", "a read names one of the six subject kinds, and "+kind+" is none")
	}
	if !slices.Contains(observed, kind) {
		return nil, nil
	}
	var all []*interfacev1.Record
	var ids []string
	switch kind {
	case "instance":
		if s.cfg.Instance != nil {
			all, ids = append(all, &interfacev1.Record{Subject: &interfacev1.Record_Instance{Instance: s.cfg.Instance()}}), append(ids, identity)
		}
	case "unit":
		if s.cfg.Units != nil {
			for _, id := range s.cfg.Units.IDs() {
				all, ids = append(all, &interfacev1.Record{Subject: &interfacev1.Record_Unit{Unit: s.unitRecord(id)}}), append(ids, id)
			}
		}
	case "channel":
		all, ids = append(all, &interfacev1.Record{Subject: &interfacev1.Record_Channel{Channel: s.channelRecord()}}), append(ids, s.cfg.Channel.Name)
	}
	if identity == "" {
		return all, nil
	}
	if i := slices.Index(ids, identity); i >= 0 {
		return all[i : i+1], nil
	}
	return nil, &interfacev1.Refusal{Code: "subject.unknown", Message: "nothing this channel may address is the " + kind + " " + identity,
		Detail: &interfacev1.Refusal_Subject{Subject: &interfacev1.Subject{Kind: kind, Identity: identity}}}
}

func (s *Surface) read(_ context.Context, r *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal) {
	records, ref := s.recordsOf(r.GetRead().GetKind(), r.GetRead().GetIdentity())
	if ref != nil {
		return nil, ref
	}
	return &interfacev1.Response{Answer: &interfacev1.Response_Read{Read: &interfacev1.Records{Records: records}}}, nil
}

// selected are the records a filter's subject axis selects: the kind it names, or the kind its type
// names by its first segment, or every kind the channel observes.
func (s *Surface) selected(f *interfacev1.Filter) []*interfacev1.Record {
	kind := f.GetSubjectKind()
	if kind == "" && f.GetType() != "" {
		kind, _, _ = strings.Cut(f.GetType(), ".")
	}
	from := observed
	if kind != "" {
		from = []string{kind}
	}
	var out []*interfacev1.Record
	for _, k := range from {
		if !slices.Contains(kinds, k) {
			continue
		}
		records, _ := s.recordsOf(k, f.GetSubjectIdentity())
		out = append(out, records...)
	}
	return out
}

// subscribe opens on the bus first and assembles the records after, so nothing is missed; then it carries
// the events the filter selects that the channel observes, and an overflow with a fresh snapshot.
func (s *Surface) subscribe(ctx context.Context, r *interfacev1.Request, send func(*interfacev1.Response) error) *interfacev1.Refusal {
	f := r.GetSubscribe().GetFilter()
	if k := f.GetSubjectKind(); k != "" && !slices.Contains(kinds, k) {
		return refusal("operation.malformed", "a filter names one of the six subject kinds, and "+k+" is none")
	}
	answer := func(c *interfacev1.Subscribed) error {
		return send(&interfacev1.Response{Answer: &interfacev1.Response_Subscribe{Subscribe: c}})
	}
	sub, snap := s.cfg.Bus.SubscribeTo(event.Filter{SubjectKind: event.Kind(f.GetSubjectKind()), SubjectID: f.GetSubjectIdentity(), Floor: int(f.GetFloor()),
		Type: f.GetType(), TypePrefix: f.GetTypePrefix(), Occurrence: f.GetOccurrence(), OccurrencePrefix: f.GetOccurrencePrefix()})
	defer sub.Close()
	if answer(&interfacev1.Subscribed{Carries: &interfacev1.Subscribed_Snapshot{Snapshot: &interfacev1.Snapshot{At: snap.At, Records: s.selected(f)}}}) != nil {
		return nil
	}
	for {
		d, err := sub.Next(ctx)
		if err != nil {
			return nil
		}
		if d.Overflow {
			if answer(&interfacev1.Subscribed{Carries: &interfacev1.Subscribed_Overflow{Overflow: &interfacev1.Snapshot{At: d.Snapshot.At, Records: s.selected(f)}}}) != nil {
				return nil
			}
			continue
		}
		if s.observes(d.Event) && answer(&interfacev1.Subscribed{Carries: &interfacev1.Subscribed_Event{Event: eventOf(d.Event)}}) != nil {
			return nil
		}
	}
}
