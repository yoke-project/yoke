package admin

import (
	"context"
	"slices"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/logstore"
)

// Page is the most entries a query answers at once, and Behind how many a follow holds before it is
// told it fell behind.
const (
	Page   = 500
	Behind = 256
)

// kinds are the readable subjects: the six kinds of the event model.
var kinds = []string{"instance", "unit", "plugin", "channel", "document", "connection"}

func stamp(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// Records are the records of a subject kind, or of the one subject named: what a read answers and what a
// subscription's snapshot is made of.
func (c *Core) Records(kind, identity string) ([]*administrativev1.Record, *administrativev1.Refusal) {
	if !slices.Contains(kinds, kind) {
		return nil, refusal("operation.malformed", "a read names one of the six subject kinds, and "+kind+" is none")
	}
	var all []*administrativev1.Record
	var ids []string
	switch kind {
	case "instance":
		if c.Instance != nil {
			r := c.Instance()
			all, ids = append(all, &administrativev1.Record{Subject: &administrativev1.Record_Instance{Instance: r}}), append(ids, r.GetIdentity())
		}
	case "unit":
		for _, id := range c.Units.IDs() {
			all, ids = append(all, &administrativev1.Record{Subject: &administrativev1.Record_Unit{Unit: c.unitRecord(id)}}), append(ids, id)
		}
	case "plugin":
		plugins, err := c.Registry.Plugins()
		if err != nil {
			return nil, unavailable(err)
		}
		for _, id := range plugins {
			if p := c.pluginRecord(id); p != nil {
				all, ids = append(all, &administrativev1.Record{Subject: &administrativev1.Record_Plugin{Plugin: p}}), append(ids, id)
			}
		}
	case "document":
		if c.Documents != nil {
			for _, d := range c.Documents() {
				all, ids = append(all, &administrativev1.Record{Subject: &administrativev1.Record_Document{Document: d}}), append(ids, d.GetPath())
			}
		}
	case "connection":
		if c.Connections != nil {
			for _, k := range c.Connections() {
				all, ids = append(all, &administrativev1.Record{Subject: &administrativev1.Record_Connection{Connection: &administrativev1.ConnectionRecord{
					Identity: k.ID, Projection: string(k.Projection), Actor: &administrativev1.Actor{Class: string(k.Actor.Class), Person: k.Actor.Person},
					Opened: stamp(k.Opened)}}}), append(ids, k.ID)
			}
		}
	case "channel":
		// No channel exists before the interface surface does.
	}
	if identity == "" {
		return all, nil
	}
	if i := slices.Index(ids, identity); i >= 0 {
		return all[i : i+1], nil
	}
	return nil, about("subject.unknown", kind, identity, "no "+kind+" "+identity+" is known")
}

func (c *Core) unitRecord(id string) *administrativev1.UnitRecord {
	plugin, _ := c.Units.Plugin(id)
	st := c.Units.Status(id)
	r := &administrativev1.UnitRecord{
		Declared: &administrativev1.UnitRecord_Declared{Identity: id, Kind: string(c.Units.Kind(id)), Backend: "host", Plugin: plugin},
		Observed: &administrativev1.UnitRecord_Observed{State: string(st.State), Incarnation: uint64(st.Incarnation), Since: stamp(st.Since)},
	}
	if st.HasCondition {
		r.Observed.Condition = &administrativev1.Condition{Grade: uint32(st.Condition.Grade), Line: st.Condition.Line, Since: stamp(st.ConditionSince)}
	}
	if plugin != "" {
		r.Plugin = c.pluginRecord(plugin)
	}
	return r
}

func (c *Core) pluginRecord(id string) *administrativev1.PluginRecord {
	p, ok, err := c.Registry.Plugin(id)
	if err != nil || !ok {
		return nil
	}
	_, present := c.Manifest(id)
	var running []string
	for _, u := range c.Units.Of(id) {
		if live(c.Units.Status(u).State) {
			running = append(running, u)
		}
	}
	return &administrativev1.PluginRecord{
		Declared: &administrativev1.PluginRecord_Declared{Identity: id, Protocol: uint32(p.Protocol), ManifestDigest: p.ManifestDigest,
			Version: p.Version, Language: p.Language, SdkLine: p.SDK},
		Authorized: &administrativev1.PluginRecord_Authorized{Enabled: p.Enabled, Granted: p.Grants, CredentialMode: p.Mode},
		Observed:   &administrativev1.PluginRecord_Observed{ManifestPresent: present, Composed: c.Composed != nil && c.Composed(id), Running: running},
	}
}

func (c *Core) read(_ context.Context, _ event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
	records, ref := c.Records(r.GetRead().GetKind(), r.GetRead().GetIdentity())
	if ref != nil {
		return nil, ref
	}
	return &administrativev1.Response{Answer: &administrativev1.Response_Read{Read: &administrativev1.Records{Records: records}}}, nil
}

func entryOf(e logstore.Entry) *administrativev1.LogEntry {
	return &administrativev1.LogEntry{Seq: e.Seq, At: stamp(e.At), Unit: e.Unit, Incarnation: e.Incarnation, Source: string(e.Source),
		Severity: uint32(e.Severity), Type: e.Type, SubjectKind: e.SubjectKind, SubjectId: e.SubjectID, Actor: e.Actor, Cause: e.Cause,
		Message: e.Message, Detail: e.Detail}
}

func (c *Core) logQuery(_ context.Context, _ event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
	q := r.GetLogQuery()
	query := logstore.Query{After: q.GetCursor(), Unit: q.GetUnit(), Incarnation: q.GetIncarnation(), Floor: int(q.GetFloor()), Limit: Page}
	if q.From != nil {
		query.From = q.GetFrom().AsTime()
	}
	if q.Until != nil {
		query.Until = q.GetUntil().AsTime()
	}
	entries, resumed, err := c.Logs.Query(query)
	if err != nil {
		return nil, unavailable(err)
	}
	page := &administrativev1.LogPage{Next: q.GetCursor(), Resumed: resumed}
	for _, e := range entries {
		page.Entries = append(page.Entries, entryOf(e))
		page.Next = e.Seq
	}
	return &administrativev1.Response{Answer: &administrativev1.Response_LogQuery{LogQuery: page}}, nil
}

// logFollow pushes the entries a follow selects as they are written. Its queue holds Behind; when it is
// full, the follower is told the sequence it was current to, and the follow goes on from what is written
// next — the gap is a query away, the entries being durable.
func (c *Core) logFollow(ctx context.Context, _ event.Actor, r *administrativev1.Request, send func(*administrativev1.Response) error) *administrativev1.Refusal {
	f := r.GetLogFollow()
	cursor := f.GetCursor()
	if cursor == 0 {
		last, err := c.Logs.Last()
		if err != nil {
			return unavailable(err)
		}
		cursor = last
	}
	written, stop := c.Logs.Watch()
	defer stop()
	queue := make(chan *administrativev1.Followed, Behind)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer close(queue)
		for {
			entries, _, err := c.Logs.Query(logstore.Query{After: cursor, Unit: f.GetUnit(), Incarnation: f.GetIncarnation(), Floor: int(f.GetFloor())})
			if err != nil {
				return
			}
			for _, e := range entries {
				select {
				case queue <- &administrativev1.Followed{Carries: &administrativev1.Followed_Entry{Entry: entryOf(e)}}:
					cursor = e.Seq
					continue
				default:
				}
				// Full: the follower was current to cursor, and is told so once there is room.
				behind := cursor
				if last, err := c.Logs.Last(); err == nil {
					cursor = last
				}
				select {
				case queue <- &administrativev1.Followed{Carries: &administrativev1.Followed_BehindAt{BehindAt: behind}}:
				case <-ctx.Done():
					return
				}
				break
			}
			select {
			case <-written:
			case <-ctx.Done():
				return
			}
		}
	}()
	for followed := range queue {
		if err := send(&administrativev1.Response{Answer: &administrativev1.Response_LogFollow{LogFollow: followed}}); err != nil {
			return nil
		}
	}
	return nil
}
