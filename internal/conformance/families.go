package conformance

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// FamilyCases are the plugin contract's cases for the families, the capabilities and the grants, which
// run after the next life of case 6 is admitted. The suite drives the deployment through the
// administrative contract, as any client does.
func FamilyCases() []Case {
	return []Case{
		{Contract: "plugin", ID: "yoke:plugin.07", Title: "an occurrence outside the granted scope is refused, and the Session goes on",
			Cites:        []string{"specs/50.30", "specs/50.64", "specs/50.105", "arch/50-plugin-surface/06 §Four families, closed", "arch/50-plugin-surface/04 §Revocation"},
			Precondition: "the harness of case 6, admitted and granted nothing",
			Issues:       "`report` of the occurrence its Manifest declares, at 40",
			Requires:     "an observation `refused` with `scope.withheld`, and no end of the Session",
			Run:          occurrenceWithheld},
		{Contract: "plugin", ID: "yoke:plugin.08", Title: "a grant reaches the unit at its next admission",
			Cites:        []string{"specs/50.30", "specs/50.32", "specs/60.33", "arch/50-plugin-surface/03 §The grant is an intersection, computed once"},
			Precondition: "the harness of case 7",
			Issues:       "every capability the Manifest declares granted and the unit restarted, on the administrative surface; then `start` in the life that follows",
			Requires:     "each grant effective at the next admission; the next life accepted without restriction, granted every capability, stream, command and query declared",
			Run:          grantedAtNextLife},
		{Contract: "plugin", ID: "yoke:plugin.09", Title: "the Core's question reaches the unit, and its answer comes back, opaque",
			Cites:        []string{"specs/50.61", "specs/50.69", "specs/60.46", "arch/50-plugin-surface/05 §The eight", "arch/60-administrative-surface/04 §The one operation whose content the Core does not read"},
			Precondition: "the harness of case 8, granted everything",
			Issues:       "a question of the type its Manifest declares, asked of the unit on the administrative surface with some bytes; then `answer` with other bytes",
			Requires:     "an observation `question` of that type carrying the bytes asked; the administrative answer is the bytes answered",
			Run:          questionAnswered},
		{Contract: "plugin", ID: "yoke:plugin.10", Title: "an occurrence is carried at the author's severity",
			Cites:        []string{"specs/50.64", "specs/50.104", "specs/90.34", "arch/50-plugin-surface/05 §The event family is where a unit declares a severity"},
			Precondition: "the harness of case 8, granted everything, and a subscription to `unit.occurrence.reported` on the administrative surface",
			Issues:       "`report` of the occurrence its Manifest declares, at 70, with a line",
			Requires:     "an event about the unit carrying the occurrence, severity 70 and the unit as its actor",
			Run:          occurrenceCarried},
		{Contract: "plugin", ID: "yoke:plugin.11", Title: "a health report is carried as the unit graded it",
			Cites:        []string{"specs/50.65", "specs/50.66", "specs/90.34", "arch/50-plugin-surface/05 §What a health report carries"},
			Precondition: "the harness of case 8, and a subscription to `unit.condition.changed` on the administrative surface",
			Issues:       "`report-health` at 80, with a line",
			Requires:     "an event about the unit at severity 80, with the unit as its actor",
			Run:          healthCarried},
		{Contract: "plugin", ID: "yoke:plugin.12", Title: "disabling the plugin revokes the Session, and the process ends",
			Cites:        []string{"specs/50.49", "specs/50.51", "specs/60.29", "specs/90.29", "arch/50-plugin-surface/04 §Revocation"},
			Precondition: "the harness of case 8, its Session open",
			Issues:       "the plugin disabled on the administrative surface",
			Requires:     "the end observed as a revocation, the plugin disabled, and then the harness gone",
			Run:          disabledRevoked},
	}
}

// declared is what the described Manifest declares, as lists of names.
type declared struct {
	ID           string
	Capabilities []string
	Streams      []string
	Commands     []string
	Queries      []string
	Occurrences  []string
}

func (r *Run) declared() declared {
	var m struct {
		ID           string                  `yaml:"id"`
		Capabilities []struct{ Name string } `yaml:"capabilities"`
		Streams      []struct{ ID string }   `yaml:"streams"`
		Commands     []any                   `yaml:"commands"`
		Queries      []any                   `yaml:"queries"`
		Occurrences  []any                   `yaml:"occurrences"`
	}
	yaml.Unmarshal([]byte(r.described), &m)
	d := declared{ID: m.ID}
	for _, c := range m.Capabilities {
		d.Capabilities = append(d.Capabilities, c.Name)
	}
	for _, s := range m.Streams {
		d.Streams = append(d.Streams, s.ID)
	}
	names := func(list []any) []string {
		var out []string
		for _, v := range list {
			switch v := v.(type) {
			case string:
				out = append(out, v)
			case map[string]any:
				if id, ok := v["id"].(string); ok {
					out = append(out, id)
				}
			}
		}
		return out
	}
	d.Commands, d.Queries, d.Occurrences = names(m.Commands), names(m.Queries), names(m.Occurrences)
	return d
}

// Operator is the instance's operator projection, which the suite drives the deployment through.
func (r *Run) Operator() (administrativev1.OperatorClient, error) {
	if r.operator == nil {
		conn, err := grpc.NewClient("unix://"+filepath.Join(r.instance, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		r.operator = administrativev1.NewOperatorClient(conn)
	}
	return r.operator, nil
}

func request(r *administrativev1.Request) *administrativev1.Request {
	r.Version = 1
	return r
}

// observed waits for an observation of a kind, and returns it with every observation before it.
func observed(h *Harness, kind string, within time.Duration) (Observation, []Observation, bool) {
	var before []Observation
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		o, ok := h.Observe(time.Until(deadline))
		if !ok {
			break
		}
		if o.Kind == kind {
			return o, before, true
		}
		before = append(before, o)
	}
	return Observation{}, before, false
}

// std: yoke:plugin.07
func occurrenceWithheld(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("`report`", "the harness of case 6", err.Error())
	}
	d := r.declared()
	if len(d.Occurrences) == 0 {
		return Fail("`report`", "a Manifest declaring an occurrence", r.described)
	}
	res := h.Do("report", map[string]any{"occurrence": d.Occurrences[0], "severity": 40, "line": "withheld"})
	if res.Unrecognised {
		return Absent("report")
	}
	o, before, ok := observed(h, "refused", 10*time.Second)
	if !ok || o.Fields["code"] != "scope.withheld" {
		return Fail("`report` outside the granted scope", "an observation `refused` with `scope.withheld`", fmt.Sprintf("%v %v", o, before))
	}
	if h.Gone(time.Second) {
		return Fail("`report` outside the granted scope", "the Session going on", "the harness gone")
	}
	return Pass()
}

// std: yoke:plugin.08
func grantedAtNextLife(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("grants", "the harness of case 7", err.Error())
	}
	op, err := r.Operator()
	if err != nil {
		return Fail("grants", "the operator projection", err.Error())
	}
	d := r.declared()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for _, c := range d.Capabilities {
		resp, err := op.Call(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: d.ID, Capability: c}}}))
		if err != nil || resp.GetPluginGrant().GetEffective() != administrativev1.Changed_EFFECTIVE_AT_NEXT_ADMISSION {
			return Fail("a grant of "+c, "a change effective at the next admission", fmt.Sprintf("%v %v", resp, err))
		}
	}
	unit := h.Hello().Unit
	if _, err := op.Call(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: unit}}})); err != nil {
		return Fail("a restart of "+unit, "the unit restarted", err.Error())
	}
	next, err := r.NextUnit(30 * time.Second)
	if err != nil {
		return Fail("a restart of "+unit, "the next life saying hello", err.Error())
	}
	res := next.Do("start", nil)
	if res.Unrecognised {
		return Absent("start")
	}
	granted, _ := res.Value["granted"].(map[string]any)
	if res.Value["outcome"] != "accepted" ||
		!slices.Equal(sorted(stringList(granted["capabilities"])), sorted(d.Capabilities)) ||
		!slices.Equal(sorted(stringList(granted["streams"])), sorted(d.Streams)) ||
		!slices.Equal(sorted(stringList(granted["commands"])), sorted(d.Commands)) ||
		!slices.Equal(sorted(stringList(granted["queries"])), sorted(d.Queries)) {
		return Fail("`start` in the next life", "an acceptance granting everything declared", fmt.Sprintf("%v %s", res.Value, res.Refusal))
	}
	return Pass()
}

// std: yoke:plugin.09
func questionAnswered(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("a question", "the harness of case 8", err.Error())
	}
	op, err := r.Operator()
	if err != nil {
		return Fail("a question", "the operator projection", err.Error())
	}
	d := r.declared()
	if len(d.Queries) == 0 {
		return Fail("a question", "a Manifest declaring a query", r.described)
	}
	type answered struct {
		resp *administrativev1.Response
		err  error
	}
	got := make(chan answered, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := op.Call(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_UnitAsk{UnitAsk: &administrativev1.UnitAsk{
			Unit: h.Hello().Unit, Type: d.Queries[0], Question: []byte("how are you")}}}))
		got <- answered{resp, err}
	}()
	q, before, ok := observed(h, "question", 10*time.Second)
	if !ok || q.Fields["type"] != d.Queries[0] || q.Fields["payload"] != "how are you" {
		return Fail("a question of "+d.Queries[0], "an observation `question` carrying the bytes asked", fmt.Sprintf("%v %v", q, before))
	}
	if res := h.Do("answer", map[string]any{"question": q.Fields["id"], "payload": "well"}); res.Unrecognised {
		return Absent("answer")
	}
	select {
	case a := <-got:
		if a.err != nil || string(a.resp.GetUnitAsk().GetAnswer()) != "well" {
			return Fail("`answer`", "the administrative answer carrying the bytes answered", fmt.Sprintf("%v %v", a.resp, a.err))
		}
	case <-time.After(30 * time.Second):
		return Fail("`answer`", "the administrative answer", "none within 30 s")
	}
	return Pass()
}

// subscribed subscribes on the operator projection to one type, and returns the stream once its snapshot
// has arrived.
func (r *Run) subscribed(ctx context.Context, typ string) (administrativev1.Operator_WatchClient, error) {
	op, err := r.Operator()
	if err != nil {
		return nil, err
	}
	stream, err := op.Watch(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_Subscribe{Subscribe: &administrativev1.Subscribe{
		Filter: &administrativev1.Filter{Type: typ}}}}))
	if err != nil {
		return nil, err
	}
	if _, err := stream.Recv(); err != nil {
		return nil, err
	}
	return stream, nil
}

// eventAbout reads the stream until an event about the unit arrives.
func eventAbout(stream administrativev1.Operator_WatchClient, unit string) (*administrativev1.Event, error) {
	for {
		r, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		if e := r.GetSubscribe().GetEvent(); e != nil && e.GetSubject().GetIdentity() == unit {
			return e, nil
		}
	}
}

// std: yoke:plugin.10
func occurrenceCarried(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("`report`", "the harness of case 8", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := r.subscribed(ctx, "unit.occurrence.reported")
	if err != nil {
		return Fail("`report`", "a subscription on the administrative surface", err.Error())
	}
	d := r.declared()
	if res := h.Do("report", map[string]any{"occurrence": d.Occurrences[0], "severity": 70, "line": "drifting"}); res.Unrecognised {
		return Absent("report")
	} else if res.Refusal != "" {
		return Fail("`report` at 70", "the report accepted", res.Refusal)
	}
	e, err := eventAbout(stream, h.Hello().Unit)
	if err != nil || e.GetOccurrence() != d.Occurrences[0] || e.GetSeverity() != 70 || e.GetActor().GetClass() != "unit" {
		return Fail("`report` at 70", "an event carrying the occurrence at 70, with the unit as its actor", fmt.Sprintf("%v %v", e, err))
	}
	return Pass()
}

// std: yoke:plugin.11
func healthCarried(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("`report-health`", "the harness of case 8", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := r.subscribed(ctx, "unit.condition.changed")
	if err != nil {
		return Fail("`report-health`", "a subscription on the administrative surface", err.Error())
	}
	if res := h.Do("report-health", map[string]any{"grade": 80, "line": "warm"}); res.Unrecognised {
		return Absent("report-health")
	} else if res.Refusal != "" {
		return Fail("`report-health` at 80", "the report accepted", res.Refusal)
	}
	e, err := eventAbout(stream, h.Hello().Unit)
	if err != nil || e.GetSeverity() != 80 || e.GetActor().GetClass() != "unit" {
		return Fail("`report-health` at 80", "an event at 80, with the unit as its actor", fmt.Sprintf("%v %v", e, err))
	}
	return Pass()
}

// std: yoke:plugin.12
func disabledRevoked(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("a disable", "the harness of case 8", err.Error())
	}
	op, err := r.Operator()
	if err != nil {
		return Fail("a disable", "the operator projection", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	id := r.declared().ID
	if _, err := op.Call(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_PluginDisable{PluginDisable: &administrativev1.PluginPolicy{Plugin: id}}})); err != nil {
		return Fail("a disable of "+id, "the plugin disabled", err.Error())
	}
	o, before, ok := observed(h, "session-ended", 10*time.Second)
	if !ok || o.Fields["closed"] != false || o.Fields["cause"] != "plugin disabled" {
		return Fail("a disable of "+id, "the end observed as a revocation, the plugin disabled", fmt.Sprintf("%v %v", o, before))
	}
	if !h.Gone(10 * time.Second) {
		return Fail("a disable of "+id, "the harness gone", "still there after 10 s")
	}
	return Pass()
}
