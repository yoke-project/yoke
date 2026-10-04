package conformance

import (
	"fmt"
	"slices"
	"time"
)

// The two channels an interface run's Core binds: the harness attaches to Panel, and the suite holds
// Bench when a case needs a channel that prevails over it.
const (
	Panel = "panel"
	Bench = "bench"
)

// interfaceChannels and interfaceArbitration are what an interface run composes: two local channels of
// one client each, Bench prevailing over Panel in the grade a declaration that says nothing gets.
var (
	interfaceChannels = map[string]any{
		Panel: map[string]any{"transport": "local", "clients": "single"},
		Bench: map[string]any{"transport": "local", "clients": "single"},
	}
	interfaceArbitration = []any{map[string]any{"prevails": Bench, "over": []any{Panel}}}
)

// InterfaceCases are the interface contract's cases, in the order a run performs them.
func InterfaceCases() []Case {
	return []Case{
		{ID: "yoke:interface.01", Title: "attaching is given the opening: the version, the standing subscription and the channel's own record", Contract: "interface",
			Cites:        []string{"specs/70.9", "specs/90.8", "arch/70-interface-surface/03 §The opening picture", "arch/70-interface-surface/08 §How this contract states its version"},
			Precondition: "a Core started by the suite with the channels `panel` and `bench`, local and of one client each, `bench` prevailing over `panel`; and the harness launched by the suite with the instance's root in `CONFORMANCE_INSTANCE`",
			Issues:       "`attach` to `panel`",
			Requires:     "the version 1, a standing subscription, and a picture holding `panel`'s record, attached",
			Run:          attachedWithTheOpening},
		{ID: "yoke:interface.02", Title: "a channel of one client refuses another, and keeps the first", Contract: "interface",
			Cites:        []string{"specs/70.21", "arch/70-interface-surface/03 §How many clients"},
			Precondition: "the harness attached to `panel`",
			Issues:       "`attach` to `panel` again; then `read` of the kind `channel`",
			Requires:     "a refusal `channel.in_use`; then the read answered on the first attachment",
			Run:          oneClientKept},
		{ID: "yoke:interface.03", Title: "a refusal travels as its code, with what it names", Contract: "interface",
			Cites:        []string{"specs/90.23", "arch/00-system/05 §How a refusal travels", "arch/70-interface-surface/08 §This surface's error codes"},
			Precondition: "the harness attached to `panel`, and a unit nobody declared",
			Issues:       "`read` of the unit `nobody`",
			Requires:     "`subject.unknown` naming the kind `unit` and the identity `nobody`",
			Run:          interfaceRefusalTravels},
		{ID: "yoke:interface.04", Title: "a displaced channel learns as a subscriber, and what it may not do is refused with its grade", Contract: "interface",
			Cites:        []string{"specs/70.26", "specs/70.27", "specs/70.29", "arch/70-interface-surface/06 §What the declaration says, and what holding means"},
			Precondition: "the harness attached to `panel`; then the suite attaches to `bench`",
			Issues:       "nothing, until the standing subscription carries the suspension; then `command` of `calibrate` to the fixture",
			Requires:     "an event `channel.suspended` about `panel`; then a refusal `channel.suspended`, in the grade `read-only`, in favour of `bench`",
			Run:          displacedAndRefused},
		{ID: "yoke:interface.05", Title: "a channel on a local socket reclaims, and its suspension ends", Contract: "interface",
			Cites:        []string{"specs/70.35", "specs/70.36", "specs/70.37", "arch/70-interface-surface/06 §How a suspension ends, in two halves"},
			Precondition: "the harness attached to `panel`, suspended in favour of `bench`",
			Issues:       "`reclaim`",
			Requires:     "that it changed something, and `panel`'s record no longer suspended",
			Run:          reclaimed},
		{ID: "yoke:interface.06", Title: "a confirmation is answered", Contract: "interface",
			Cites:        []string{"specs/70.32", "specs/70.33", "arch/70-interface-surface/03 §A connection that looks open proves nothing"},
			Precondition: "the harness attached to `panel`, a channel named in an arbitration rule",
			Issues:       "`confirm` of the standing subscription",
			Requires:     "an answer, and no refusal",
			Run:          confirmed},
		{ID: "yoke:interface.07", Title: "a subscription opens with a snapshot of what the channel sees, and a channel sees itself alone", Contract: "interface",
			Cites:        []string{"specs/70.7", "specs/70.8", "specs/90.32", "arch/70-interface-surface/05 §What a subscription promises", "arch/70-interface-surface/05 §Three subject kinds, and three it does not see"},
			Precondition: "the harness attached to `panel`",
			Issues:       "`subscribe` to the subject kind `channel`",
			Requires:     "a snapshot holding `panel`'s record, and not `bench`'s",
			Run:          subscribedToChannels},
	}
}

// client is the harness a case drives, or the outcome of its absence.
func client(r *Run, directive string) (*Harness, *Outcome) {
	h, err := r.Client()
	if err != nil {
		o := Fail(directive, "a harness launched against the instance", err.Error())
		return nil, &o
	}
	return h, nil
}

// attached is the harness, attached to Panel: the first case attaches it, and every later one finds it so.
func attached(r *Run, directive string) (*Harness, Result, *Outcome) {
	h, absent := client(r, directive)
	if absent != nil {
		return nil, Result{}, absent
	}
	if r.opening != nil {
		return h, *r.opening, nil
	}
	res := h.Do("attach", map[string]any{"channel": Panel})
	if res.Unrecognised {
		o := Absent("attach")
		return nil, res, &o
	}
	if res.Refusal != "" {
		o := Fail("`attach` to `panel`", "an attachment", res.Refusal)
		return nil, res, &o
	}
	r.opening = &res
	return h, res, nil
}

// channelRecord is the record of the channel named, among records.
func channelRecord(records any, name string) map[string]any {
	list, _ := records.([]any)
	for _, rec := range list {
		if field(rec, "channel", "declared", "name") == name {
			m, _ := field(rec, "channel").(map[string]any)
			return m
		}
	}
	return nil
}

// std: yoke:interface.01
func attachedWithTheOpening(r *Run) Outcome {
	_, res, absent := attached(r, "`attach`")
	if absent != nil {
		return *absent
	}
	panel := channelRecord(res.Value["records"], Panel)
	if res.Value["version"] != float64(1) || res.Value["subscription"] == "" || res.Value["subscription"] == nil ||
		panel == nil || field(panel, "observed", "attached") != true {
		return Fail("`attach` to `panel`", "the version 1, a standing subscription, and `panel`'s record, attached", fmt.Sprintf("%v", res.Value))
	}
	return Pass()
}

// std: yoke:interface.02
func oneClientKept(r *Run) Outcome {
	h, _, absent := attached(r, "`attach`")
	if absent != nil {
		return *absent
	}
	again := h.Do("attach", map[string]any{"channel": Panel})
	if again.Refusal != "channel.in_use" {
		return Fail("`attach` to `panel` again", "`channel.in_use`", fmt.Sprintf("%q %v", again.Refusal, again.Value))
	}
	read := h.Do("read", map[string]any{"kind": "channel"})
	if read.Unrecognised {
		return Absent("read")
	}
	if read.Refusal != "" || channelRecord(read.Value["records"], Panel) == nil {
		return Fail("`read` of the kind `channel`", "the channels' records, on the first attachment", fmt.Sprintf("%q %v", read.Refusal, read.Value))
	}
	return Pass()
}

// std: yoke:interface.03
func interfaceRefusalTravels(r *Run) Outcome {
	h, _, absent := attached(r, "`read`")
	if absent != nil {
		return *absent
	}
	res := h.Do("read", map[string]any{"kind": "unit", "identity": "nobody"})
	if res.Unrecognised {
		return Absent("read")
	}
	if res.Refusal != "subject.unknown" || field(res.Value, "subject", "kind") != "unit" || field(res.Value, "subject", "identity") != "nobody" {
		return Fail("`read` of the unit `nobody`", "`subject.unknown` naming the unit `nobody`", fmt.Sprintf("%q %v", res.Refusal, res.Value))
	}
	return Pass()
}

// std: yoke:interface.04
func displacedAndRefused(r *Run) Outcome {
	h, _, absent := attached(r, "`command`")
	if absent != nil {
		return *absent
	}
	if err := r.Hold(Bench); err != nil {
		return Fail("the suite attaching to `bench`", "an attachment", err.Error())
	}
	suspended := func(o Observation) bool {
		return o.Kind == "event" && o.Fields["type"] == "channel.suspended" && o.Fields["subject"] == Panel
	}
	seen := false
	for deadline := time.Now().Add(10 * time.Second); !seen && time.Now().Before(deadline); {
		o, ok := h.Observe(time.Until(deadline))
		if !ok {
			break
		}
		seen = suspended(o)
	}
	if !seen {
		return Fail("nothing, while `bench` attaches", "an event `channel.suspended` about `panel`", "none")
	}
	res := h.Do("command", map[string]any{"unit": Fixture, "type": "calibrate", "payload": ""})
	if res.Unrecognised {
		return Absent("command")
	}
	if res.Refusal != "channel.suspended" || field(res.Value, "suspension", "grade") != "read-only" || field(res.Value, "suspension", "by") != Bench {
		return Fail("`command` while suspended", "`channel.suspended`, read-only, in favour of `bench`", fmt.Sprintf("%q %v", res.Refusal, res.Value))
	}
	return Pass()
}

// std: yoke:interface.05
func reclaimed(r *Run) Outcome {
	h, _, absent := attached(r, "`reclaim`")
	if absent != nil {
		return *absent
	}
	res := h.Do("reclaim", nil)
	if res.Unrecognised {
		return Absent("reclaim")
	}
	if res.Refusal != "" || res.Value["changed"] != true || field(res.Value, "channel", "observed", "suspended") == true {
		return Fail("`reclaim`", "a change, and `panel` no longer suspended", fmt.Sprintf("%q %v", res.Refusal, res.Value))
	}
	return Pass()
}

// std: yoke:interface.06
func confirmed(r *Run) Outcome {
	h, opening, absent := attached(r, "`confirm`")
	if absent != nil {
		return *absent
	}
	res := h.Do("confirm", map[string]any{"subscription": opening.Value["subscription"], "sequence": opening.Value["at"]})
	if res.Unrecognised {
		return Absent("confirm")
	}
	if res.Refusal != "" {
		return Fail("`confirm` of the standing subscription", "an answer", res.Refusal)
	}
	return Pass()
}

// std: yoke:interface.07
func subscribedToChannels(r *Run) Outcome {
	h, _, absent := attached(r, "`subscribe`")
	if absent != nil {
		return *absent
	}
	res := h.Do("subscribe", map[string]any{"subject_kind": "channel"})
	if res.Unrecognised {
		return Absent("subscribe")
	}
	if res.Refusal != "" {
		return Fail("`subscribe` to channels", "a subscription", res.Refusal)
	}
	isSnapshot := func(o Observation) bool {
		return o.Kind == "snapshot" && channelRecord(o.Fields["records"], Panel) != nil && channelRecord(o.Fields["records"], Bench) == nil
	}
	if slices.ContainsFunc(res.Before, isSnapshot) {
		return Pass()
	}
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		o, ok := h.Observe(time.Until(deadline))
		if !ok {
			break
		}
		if isSnapshot(o) {
			return Pass()
		}
	}
	return Fail("`subscribe` to channels", "a snapshot holding `panel`'s record, and not `bench`'s", "none")
}
