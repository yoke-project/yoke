package admin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// Units is the supervisor, as the operations on a unit reach it.
type Units interface {
	// Plugin is the plugin a declared unit runs, empty for a unit of another kind; false for a unit
	// nobody declared.
	Plugin(unit string) (string, bool)
	// Of are the units declared to run a plugin.
	Of(plugin string) []string
	// IDs are every unit declared, in the order of their identities.
	IDs() []string
	Kind(unit string) unit.Kind
	Status(unit string) supervisor.Status
	StartUnit(unit string) error
	StopUnit(unit string) error
	RestartUnit(unit string) error
}

// ErrUnreachable is what Units answers when the backend a unit runs on cannot be reached.
var ErrUnreachable = supervisor.ErrUnreachable

// Sessions are the units' Sessions, as the operations reach them.
type Sessions interface {
	// Open says whether a unit holds a Session now.
	Open(unit string) bool
	Ask(ctx context.Context, unit string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error)
	Revoke(unit string, cause pluginv1.SessionMessage_Revoked_Cause, line string) error
}

// Wait is how long the Core waits for a unit's acknowledgement or answer. It is not declarable.
const Wait = 30 * time.Second

// Bound is the most an opaque question or answer may hold.
const Bound = 1 << 20

// Core is what the operations act on: every one terminates here, and none is forwarded to a unit.
type Core struct {
	Registry *registry.Registry
	Manifest func(plugin string) (*gate.Manifest, bool)
	Units    Units
	Sessions Sessions
	Logs     *logstore.Store
	Publish  func(event.Event)
	// Instance is the instance's own record, as it stands.
	Instance func() *administrativev1.InstanceRecord
	// Documents are the documents the Core read, each as it was read.
	Documents func() []*administrativev1.DocumentRecord
	// Composed says whether the composition in force runs a plugin.
	Composed func(plugin string) bool
	// Connections are the administrative connections open now.
	Connections func() []Connection
	// Wait replaces the 30 s wait, for a test.
	Wait time.Duration
}

// Records are the records of a subject kind, or of the one subject named: what a read answers and what a
// subscription's snapshot is made of.
func (c *Core) Records(kind, identity string) ([]*administrativev1.Record, *administrativev1.Refusal) {
	return nil, nil
}

// Operations are the operations the Core serves, by name. Stream control is not among them: a stream
// travels on a transport of its own, and there is none yet to create.
func (c *Core) Operations() map[string]Operation {
	return map[string]Operation{
		"plugin.enable":        {Answer: c.policy(true)},
		"plugin.disable":       {Answer: c.policy(false)},
		"plugin.grant":         {Answer: c.grant(true)},
		"plugin.withdraw":      {Answer: c.grant(false)},
		"unit.start":           {Answer: c.act("start")},
		"unit.stop":            {Answer: c.act("stop")},
		"unit.restart":         {Answer: c.act("restart")},
		"unit.retention.set":   {Answer: c.retention(true)},
		"unit.retention.clear": {Answer: c.retention(false)},
		"unit.ask":             {Answer: c.ask},
	}
}

// Changes says whether an operation changes something, which is what is refused while the instance stops.
func Changes(name string) bool {
	return !slices.Contains([]string{"read", "log.query", "log.follow", "subscribe"}, name)
}

type answer = func(context.Context, event.Actor, *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal)

// about is a refusal naming the subject it is about.
func about(code, kind, identity, message string) *administrativev1.Refusal {
	return &administrativev1.Refusal{Code: code, Message: message,
		Detail: &administrativev1.Refusal_Subject{Subject: &administrativev1.Subject{Kind: kind, Identity: identity}}}
}

func unavailable(err error) *administrativev1.Refusal {
	return refusal("store.unavailable", fmt.Sprintf("the store could not be read or written: %v", err))
}

// plugin checks that id is a plugin.
func (c *Core) plugin(id string) (registry.Plugin, *administrativev1.Refusal) {
	p, ok, err := c.Registry.Plugin(id)
	if err != nil {
		return p, unavailable(err)
	}
	if ok {
		return p, nil
	}
	if _, isUnit := c.Units.Plugin(id); isUnit {
		return p, about("subject.wrong_kind", "unit", id, id+" is a unit, and this operation is about a plugin")
	}
	return p, about("subject.unknown", "plugin", id, "no plugin "+id+" is known")
}

// unit checks that id is a unit.
func (c *Core) unit(id string) *administrativev1.Refusal {
	if _, ok := c.Units.Plugin(id); ok {
		return nil
	}
	if _, isPlugin, _ := c.Registry.Plugin(id); isPlugin {
		return about("subject.wrong_kind", "plugin", id, id+" is a plugin, and this operation is about a unit")
	}
	return about("subject.unknown", "unit", id, "no unit "+id+" is declared")
}

// record writes the act to the log store, against the actor that did it.
func (c *Core) record(actor event.Actor, kind event.Kind, id string, incarnation uint64, message string) {
	e := logstore.Entry{At: time.Now(), Source: logstore.FromCore, Severity: event.Notable, SubjectKind: string(kind),
		SubjectID: id, Actor: logstore.Actor(actor), Message: message}
	if kind == event.Unit {
		e.Unit, e.Incarnation = id, incarnation
	}
	c.Logs.Append(e)
}

// immediately is a change's answer, effective at once, with what it replaced.
func immediately(previously *administrativev1.Previously) *administrativev1.Changed {
	return &administrativev1.Changed{Previously: previously, Effective: administrativev1.Changed_EFFECTIVE_IMMEDIATELY}
}

func (c *Core) policy(enabled bool) answer {
	return func(_ context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
		id := r.GetPluginEnable().GetPlugin()
		if !enabled {
			id = r.GetPluginDisable().GetPlugin()
		}
		p, ref := c.plugin(id)
		if ref != nil {
			return nil, ref
		}
		change := immediately(&administrativev1.Previously{Value: &administrativev1.Previously_Enabled{Enabled: p.Enabled}})
		answered := &administrativev1.Response{Answer: &administrativev1.Response_PluginEnable{PluginEnable: change}}
		do := c.Registry.Enable
		if !enabled {
			answered.Answer = &administrativev1.Response_PluginDisable{PluginDisable: change}
			do = c.Registry.Disable
		}
		moved, err := do(id, actor.Person)
		if err != nil {
			return nil, unavailable(err)
		}
		if !moved {
			return answered, nil
		}
		c.Publish(event.PolicyChanged(id, actor, &enabled, nil, nil))
		c.record(actor, event.Plugin, id, 0, map[bool]string{true: "enabled ", false: "disabled "}[enabled]+id)
		if !enabled {
			// Disabling reaches every copy: each live Session of the plugin is revoked.
			for _, u := range c.Units.Of(id) {
				if !c.Sessions.Open(u) {
					continue
				}
				incarnation := c.Units.Status(u).Incarnation
				if c.Sessions.Revoke(u, pluginv1.SessionMessage_Revoked_CAUSE_PLUGIN_DISABLED, "the plugin was disabled") == nil {
					change.Consequences = append(change.Consequences, &administrativev1.Consequence{Unit: u, Incarnation: uint64(incarnation), What: "its Session was revoked"})
				}
			}
		}
		return answered, nil
	}
}

func (c *Core) grant(grant bool) answer {
	return func(_ context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
		g := r.GetPluginGrant()
		if !grant {
			g = r.GetPluginWithdraw()
		}
		id, capability := g.GetPlugin(), g.GetCapability()
		p, ref := c.plugin(id)
		if ref != nil {
			return nil, ref
		}
		if !c.declares(id, capability) {
			return nil, &administrativev1.Refusal{Code: "capability.undeclared", Message: "the plugin's Manifest does not declare " + capability,
				Detail: &administrativev1.Refusal_Item{Item: capability}}
		}
		change := &administrativev1.Changed{Previously: &administrativev1.Previously{Value: &administrativev1.Previously_Granted{Granted: slices.Contains(p.Grants, capability)}},
			Effective: administrativev1.Changed_EFFECTIVE_AT_NEXT_ADMISSION}
		answered := &administrativev1.Response{Answer: &administrativev1.Response_PluginGrant{PluginGrant: change}}
		do := c.Registry.Grant
		if !grant {
			answered.Answer = &administrativev1.Response_PluginWithdraw{PluginWithdraw: change}
			do = c.Registry.Withdraw
		}
		moved, err := do(id, capability, actor.Person)
		if err != nil {
			return nil, unavailable(err)
		}
		// Each running unit keeps the scope its own admission computed, until it is admitted again.
		for _, u := range c.Units.Of(id) {
			st := c.Units.Status(u)
			if !live(st.State) {
				continue
			}
			change.Consequences = append(change.Consequences, &administrativev1.Consequence{Unit: u, Incarnation: uint64(st.Incarnation),
				What: "still under the scope of its admission, until it is admitted again"})
		}
		if moved {
			if grant {
				c.Publish(event.PolicyChanged(id, actor, nil, []string{capability}, nil))
				c.record(actor, event.Plugin, id, 0, "granted "+capability+" to "+id)
			} else {
				c.Publish(event.PolicyChanged(id, actor, nil, nil, []string{capability}))
				c.record(actor, event.Plugin, id, 0, "withdrew "+capability+" from "+id)
			}
		}
		return answered, nil
	}
}

// declares says whether the plugin's Manifest declares a capability.
func (c *Core) declares(plugin, capability string) bool {
	m, ok := c.Manifest(plugin)
	if !ok {
		return false
	}
	return slices.ContainsFunc(m.Capabilities, func(k gate.Capability) bool { return k.Name == capability })
}

// live says whether a state is a life still going.
func live(s unit.State) bool { return s != "" && !s.Terminal() }

func (c *Core) act(op string) answer {
	return func(_ context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
		answered := &administrativev1.Response{}
		var id string
		var do func(string) error
		change := immediately(nil)
		switch op {
		case "start":
			id, do = r.GetUnitStart().GetUnit(), c.Units.StartUnit
			answered.Answer = &administrativev1.Response_UnitStart{UnitStart: change}
		case "stop":
			id, do = r.GetUnitStop().GetUnit(), c.Units.StopUnit
			answered.Answer = &administrativev1.Response_UnitStop{UnitStop: change}
		default:
			id, do = r.GetUnitRestart().GetUnit(), c.Units.RestartUnit
			answered.Answer = &administrativev1.Response_UnitRestart{UnitRestart: change}
		}
		if ref := c.unit(id); ref != nil {
			return nil, ref
		}
		before := c.Units.Status(id)
		change.Previously = &administrativev1.Previously{Value: &administrativev1.Previously_State{State: string(before.State)}}
		if (op == "stop" && !live(before.State)) || (op == "start" && live(before.State)) {
			return answered, nil
		}
		err := do(id)
		if errors.Is(err, ErrUnreachable) {
			return nil, about("backend.unavailable", "unit", id, err.Error())
		}
		if err != nil {
			return nil, about("subject.unknown", "unit", id, err.Error())
		}
		after := c.Units.Status(id)
		if op != "start" && live(before.State) {
			change.Consequences = append(change.Consequences, &administrativev1.Consequence{Unit: id, Incarnation: uint64(before.Incarnation), What: "ended"})
		}
		if op != "stop" {
			change.Consequences = append(change.Consequences, &administrativev1.Consequence{Unit: id, Incarnation: uint64(after.Incarnation), What: "begun"})
		}
		c.record(actor, event.Unit, id, uint64(after.Incarnation), op+" "+id)
		return answered, nil
	}
}

func retentionOf(r logstore.Retention) *administrativev1.Retention {
	out := &administrativev1.Retention{Bytes: r.Bytes, Entries: r.Entries}
	if r.Age > 0 {
		out.Age = durationpb.New(r.Age)
	}
	return out
}

func (c *Core) retention(set bool) answer {
	return func(_ context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
		id := r.GetUnitRetentionClear().GetUnit()
		if set {
			id = r.GetUnitRetentionSet().GetUnit()
		}
		if ref := c.unit(id); ref != nil {
			return nil, ref
		}
		var next logstore.Retention
		if set {
			o := r.GetUnitRetentionSet()
			if (o.Age != nil && o.GetAge().AsDuration() <= 0) || (o.Bytes != nil && o.GetBytes() == 0) || (o.Entries != nil && o.GetEntries() == 0) {
				return nil, refusal("retention.invalid", "a limit is a positive number; one left out is unconstrained")
			}
			next = logstore.Retention{Age: o.GetAge().AsDuration(), Bytes: o.Bytes, Entries: o.Entries}
		}
		previous, _, err := c.Logs.Override(id)
		if err != nil {
			return nil, unavailable(err)
		}
		change := immediately(&administrativev1.Previously{Value: &administrativev1.Previously_Retention{Retention: retentionOf(previous)}})
		incarnation := uint64(c.Units.Status(id).Incarnation)
		if !set {
			if err := c.Logs.ClearOverride(id); err != nil {
				return nil, unavailable(err)
			}
			c.record(actor, event.Unit, id, incarnation, "cleared the retention of "+id)
			return &administrativev1.Response{Answer: &administrativev1.Response_UnitRetentionClear{UnitRetentionClear: change}}, nil
		}
		if err := c.Logs.SetOverride(id, next); err != nil {
			return nil, unavailable(err)
		}
		c.record(actor, event.Unit, id, incarnation, "set the retention of "+id)
		return &administrativev1.Response{Answer: &administrativev1.Response_UnitRetentionSet{UnitRetentionSet: change}}, nil
	}
}

// ask carries a question to a unit and its answer back. Neither is read, and neither is recorded.
func (c *Core) ask(ctx context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
	q := r.GetUnitAsk()
	id := q.GetUnit()
	if ref := c.unit(id); ref != nil {
		return nil, ref
	}
	if len(q.GetQuestion()) > Bound {
		return nil, refusal("operation.malformed", "a question is bounded at 1 MiB")
	}
	st := c.Units.Status(id)
	if !live(st.State) {
		return nil, about("unit.not_running", "unit", id, id+" is not running")
	}
	if !c.Sessions.Open(id) {
		return nil, about("unit.no_session", "unit", id, id+" is running and holds no Session")
	}
	wait := c.Wait
	if wait == 0 {
		wait = Wait
	}
	waiting, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	c.record(actor, event.Unit, id, uint64(st.Incarnation), "asked "+id+" of the type "+q.GetType())
	a, err := c.Sessions.Ask(waiting, id, &pluginv1.Query_Question{Type: q.GetType(), Payload: q.GetQuestion()})
	var refused *session.Refused
	switch {
	case errors.As(err, &refused) && refused.Code == pluginv1.Code_CODE_SCOPE_WITHHELD:
		return nil, &administrativev1.Refusal{Code: "scope.withheld", Message: refused.Error(), Detail: &administrativev1.Refusal_Item{Item: q.GetType()}}
	case errors.As(err, &refused):
		return nil, refusal("operation.malformed", refused.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return nil, about("unit.unanswered", "unit", id, fmt.Sprintf("%s did not answer within %v", id, wait))
	case err != nil:
		return nil, about("unit.no_session", "unit", id, err.Error())
	}
	if len(a.GetPayload()) > Bound {
		return nil, about("unit.unanswered", "unit", id, "the answer exceeds 1 MiB, and is not carried")
	}
	return &administrativev1.Response{Answer: &administrativev1.Response_UnitAsk{UnitAsk: &administrativev1.Asked{Answer: a.GetPayload()}}}, nil
}
