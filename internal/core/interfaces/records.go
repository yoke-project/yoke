package interfaces

import (
	"slices"

	"google.golang.org/protobuf/types/known/timestamppb"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// records are what the channel observes, as they stand: the instance, every unit, and this channel. A
// read, a snapshot and the opening picture are all made of them.
func (s *Surface) records() []*interfacev1.Record {
	var out []*interfacev1.Record
	if s.cfg.Instance != nil {
		out = append(out, &interfacev1.Record{Subject: &interfacev1.Record_Instance{Instance: s.cfg.Instance()}})
	}
	if s.cfg.Units != nil {
		for _, id := range s.cfg.Units.IDs() {
			out = append(out, &interfacev1.Record{Subject: &interfacev1.Record_Unit{Unit: s.unitRecord(id)}})
		}
	}
	return append(out, &interfacev1.Record{Subject: &interfacev1.Record_Channel{Channel: s.channelRecord()}})
}

// unitRecord is one unit as the channel sees it, and on a Plugin unit what the channel may address: the
// objects its plugin was granted.
func (s *Surface) unitRecord(id string) *interfacev1.UnitRecord {
	kind := s.cfg.Units.Kind(id)
	st := s.cfg.Units.Status(id)
	r := &interfacev1.UnitRecord{
		Declared:  &interfacev1.UnitRecord_Declared{Identity: id, Kind: string(kind)},
		Observed:  &interfacev1.UnitRecord_Observed{State: string(st.State), Incarnation: uint64(st.Incarnation)},
		Addressed: &interfacev1.UnitRecord_Addressed{},
	}
	if !st.Since.IsZero() {
		r.Observed.Since = timestamppb.New(st.Since)
	}
	if st.HasCondition {
		r.Observed.Condition = &interfacev1.Condition{Grade: uint32(st.Condition.Grade), Line: st.Condition.Line, Since: timestamppb.New(st.ConditionSince)}
	}
	if s.cfg.Active != nil {
		r.Observed.Streams = s.cfg.Active(id)
	}
	if kind == unit.Plugin && s.cfg.Granted != nil {
		r.Addressed.Streams, r.Addressed.Commands, r.Addressed.Queries = s.cfg.Granted(id)
	}
	return r
}

// channelRecord is this channel: as declared, and as it stands.
func (s *Surface) channelRecord() *interfacev1.ChannelRecord {
	ch := s.cfg.Channel
	class := "local"
	if ch.Address != nil && ch.Address.Class != "" {
		class = ch.Address.Class
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	client := ""
	if len(s.clients) > 0 {
		client = s.clients[len(s.clients)-1]
	}
	return &interfacev1.ChannelRecord{
		Declared: &interfacev1.ChannelRecord_Declared{Name: ch.Name, Projection: ch.Transport, AddressClass: class, Clients: ch.Clients},
		Observed: &interfacev1.ChannelRecord_Observed{Attached: len(s.clients) > 0, Client: client},
	}
}

// observes says whether the channel sees an event: every unit, the instance, and itself, and nothing
// about plugins, documents, connections or other channels.
func (s *Surface) observes(e event.Event) bool {
	switch e.Subject.Kind {
	case event.Unit, event.Instance:
		return true
	case event.Channel:
		return e.Subject.ID == s.cfg.Channel.Name
	}
	return false
}

// eventOf is an event as a channel receives it.
func eventOf(e event.Event) *interfacev1.Event {
	return &interfacev1.Event{Seq: e.Seq, Type: e.Type, Time: timestamppb.New(e.Time), Severity: uint32(e.Severity), Occurrence: e.Occurrence,
		Cause: e.Cause, Detail: e.Detail, Subject: &interfacev1.Subject{Kind: string(e.Subject.Kind), Identity: e.Subject.ID, Incarnation: e.Subject.Incarnation},
		Actor: &interfacev1.Actor{Class: string(e.Actor.Class), Person: e.Actor.Person}}
}

func (s *Surface) attach(client string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cfg.Channel.Clients == "single" && len(s.clients) > 0 {
		return false
	}
	s.clients = append(s.clients, client)
	return true
}

func (s *Surface) detach(client string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i := slices.Index(s.clients, client); i >= 0 {
		s.clients = slices.Delete(s.clients, i, i+1)
	}
}
