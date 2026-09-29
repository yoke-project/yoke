package conformance

import (
	"fmt"
	"time"
)

// Fixture is the plugin an administrative run's Core declares, and nothing composes: what the harness
// administers.
const Fixture = "com.yoke.conformance.fixture"

// fixtureManifest declares the fixture with one capability, so that a grant has something to name.
const fixtureManifest = "manifest: 1\nid: " + Fixture + "\nprotocol: 1\nstreams: [ { id: fixture.data } ]\n" +
	"capabilities: [ { name: stream.data.publish, governs: { stream: fixture.data } } ]\n"

// AdministrativeCases are the administrative contract's cases, in the order a run performs them.
func AdministrativeCases() []Case {
	return []Case{
		{ID: "yoke:administrative.01", Title: "the library reaches the instance by its addresses, and reads it", Contract: "administrative",
			Cites:        []string{"specs/60.7", "specs/60.36", "specs/90.4", "arch/60-administrative-surface/01 §One pair per instance", "arch/60-administrative-surface/05 §Which subjects are readable"},
			Precondition: "a Core started by the suite in the service form, and the harness launched by the suite with the instance's root in `CONFORMANCE_INSTANCE`",
			Issues:       "`read` of the kind `instance`",
			Requires:     "one record, the instance's, ready and in the service form",
			Run:          readTheInstance},
		{ID: "yoke:administrative.02", Title: "a change answers what it replaced, and an effect already true succeeds", Contract: "administrative",
			Cites:        []string{"specs/60.31", "specs/60.34", "arch/60-administrative-surface/04 §What every answer carries"},
			Precondition: "the fixture plugin declared, and enabled",
			Issues:       "`disable` of the fixture, twice",
			Requires:     "the first answers that it was enabled, effective immediately; the second that it was not, effective immediately",
			Run:          disableTwice},
		{ID: "yoke:administrative.03", Title: "a refusal travels as its code, with what it names", Contract: "administrative",
			Cites:        []string{"specs/60.57", "specs/90.23", "arch/60-administrative-surface/07 §This surface's codes", "arch/90-sdks/05 §The observable model may not differ"},
			Precondition: "a unit nobody declared, and a capability the fixture's Manifest does not declare",
			Issues:       "`stop-unit` of `nobody`; `grant` of `head.move` to the fixture",
			Requires:     "`subject.unknown` naming the kind `unit` and the identity `nobody`; `capability.undeclared` naming `head.move`",
			Run:          refusalsTravel},
		{ID: "yoke:administrative.04", Title: "a grant takes effect at the next admission", Contract: "administrative",
			Cites:        []string{"specs/60.33", "arch/60-administrative-surface/04 §What every answer carries"},
			Precondition: "the fixture, granted nothing",
			Issues:       "`grant` of `stream.data.publish` to the fixture",
			Requires:     "that it was not granted, effective at the next admission",
			Run:          grantAtNextAdmission},
		{ID: "yoke:administrative.05", Title: "a subscription opens with a snapshot, and continues with what happens", Contract: "administrative",
			Cites:        []string{"specs/60.36", "specs/60.49", "specs/90.32", "arch/60-administrative-surface/06 §What a subscription promises"},
			Precondition: "the fixture, disabled",
			Issues:       "`subscribe` to the subject kind `plugin`; then `enable` of the fixture",
			Requires:     "a snapshot observed holding the fixture's record, then an event `plugin.policy.changed` about the fixture",
			Run:          subscriptionContinues},
		{ID: "yoke:administrative.06", Title: "the log is queried", Contract: "administrative",
			Cites:        []string{"specs/60.43", "arch/60-administrative-surface/05 §The log store, queried and followed"},
			Precondition: "the Core, ready",
			Issues:       "`query-log`, from the beginning",
			Requires:     "entries, among them `instance.ready`",
			Run:          logQueried},
	}
}

// administrator is the harness a case drives, or the outcome of its absence.
func administrator(r *Run, directive string) (*Harness, *Outcome) {
	h, err := r.Administrator()
	if err != nil {
		o := Fail(directive, "a harness launched against the instance", err.Error())
		return nil, &o
	}
	return h, nil
}

// field reads a path of keys out of a result's value.
func field(v any, path ...string) any {
	for _, k := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	return v
}

// std: yoke:administrative.01
func readTheInstance(r *Run) Outcome {
	h, absent := administrator(r, "`read`")
	if absent != nil {
		return *absent
	}
	res := h.Do("read", map[string]any{"kind": "instance"})
	if res.Unrecognised {
		return Absent("read")
	}
	records, _ := res.Value["records"].([]any)
	if len(records) != 1 || field(records[0], "instance", "ready") != true || field(records[0], "instance", "form") != "service" {
		return Fail("`read` of the kind `instance`", "one record, the instance's, ready and in the service form", fmt.Sprintf("%v %s", res.Value, res.Refusal))
	}
	return Pass()
}

// changed says whether a change's answer replaced previously and took effect when.
func changed(res Result, key string, previously bool, effective string) bool {
	return res.Refusal == "" && field(res.Value, "previously", key) == previously && res.Value["effective"] == effective
}

// std: yoke:administrative.02
func disableTwice(r *Run) Outcome {
	h, absent := administrator(r, "`disable`")
	if absent != nil {
		return *absent
	}
	first := h.Do("disable", map[string]any{"plugin": Fixture})
	if first.Unrecognised {
		return Absent("disable")
	}
	if !changed(first, "enabled", true, "immediately") {
		return Fail("`disable` of the fixture", "that it was enabled, effective immediately", fmt.Sprintf("%v %s", first.Value, first.Refusal))
	}
	second := h.Do("disable", map[string]any{"plugin": Fixture})
	if !changed(second, "enabled", false, "immediately") {
		return Fail("`disable` of the fixture, again", "that it was not enabled, effective immediately", fmt.Sprintf("%v %s", second.Value, second.Refusal))
	}
	return Pass()
}

// std: yoke:administrative.03
func refusalsTravel(r *Run) Outcome {
	h, absent := administrator(r, "`stop-unit`")
	if absent != nil {
		return *absent
	}
	stop := h.Do("stop-unit", map[string]any{"unit": "nobody"})
	if stop.Unrecognised {
		return Absent("stop-unit")
	}
	if stop.Refusal != "subject.unknown" || field(stop.Value, "subject", "kind") != "unit" || field(stop.Value, "subject", "identity") != "nobody" {
		return Fail("`stop-unit` of `nobody`", "`subject.unknown` naming the unit `nobody`", fmt.Sprintf("%s %v", stop.Refusal, stop.Value))
	}
	grant := h.Do("grant", map[string]any{"plugin": Fixture, "capability": "head.move"})
	if grant.Unrecognised {
		return Absent("grant")
	}
	if grant.Refusal != "capability.undeclared" || grant.Value["item"] != "head.move" {
		return Fail("`grant` of `head.move`", "`capability.undeclared` naming `head.move`", fmt.Sprintf("%s %v", grant.Refusal, grant.Value))
	}
	return Pass()
}

// std: yoke:administrative.04
func grantAtNextAdmission(r *Run) Outcome {
	h, absent := administrator(r, "`grant`")
	if absent != nil {
		return *absent
	}
	res := h.Do("grant", map[string]any{"plugin": Fixture, "capability": "stream.data.publish"})
	if res.Unrecognised {
		return Absent("grant")
	}
	if !changed(res, "granted", false, "at next admission") {
		return Fail("`grant` of `stream.data.publish`", "that it was not granted, effective at the next admission", fmt.Sprintf("%v %s", res.Value, res.Refusal))
	}
	return Pass()
}

// std: yoke:administrative.05
func subscriptionContinues(r *Run) Outcome {
	h, absent := administrator(r, "`subscribe`")
	if absent != nil {
		return *absent
	}
	res := h.Do("subscribe", map[string]any{"subject_kind": "plugin"})
	if res.Unrecognised {
		return Absent("subscribe")
	}
	if res.Refusal != "" {
		return Fail("`subscribe` to plugins", "a subscription", res.Refusal)
	}
	snapshot, ok := h.Observe(10 * time.Second)
	records, _ := snapshot.Fields["records"].([]any)
	held := false
	for _, rec := range records {
		held = held || field(rec, "plugin", "declared", "identity") == Fixture
	}
	if !ok || snapshot.Kind != "snapshot" || !held {
		return Fail("`subscribe` to plugins", "a snapshot holding the fixture's record", fmt.Sprintf("%v", snapshot))
	}
	if enabled := h.Do("enable", map[string]any{"plugin": Fixture}); enabled.Unrecognised {
		return Absent("enable")
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		o, ok := h.Observe(time.Until(deadline))
		if !ok {
			break
		}
		if o.Kind == "event" && o.Fields["type"] == "plugin.policy.changed" && o.Fields["subject"] == Fixture {
			return Pass()
		}
	}
	return Fail("`enable` of the fixture, subscribed", "an event `plugin.policy.changed` about the fixture", "none")
}

// std: yoke:administrative.06
func logQueried(r *Run) Outcome {
	h, absent := administrator(r, "`query-log`")
	if absent != nil {
		return *absent
	}
	// An entry is durable once its batch is written, a fraction of a second after the event: the case asks
	// again until then, within a bound.
	var res Result
	var entries []any
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(200 * time.Millisecond) {
		res = h.Do("query-log", map[string]any{})
		if res.Unrecognised {
			return Absent("query-log")
		}
		entries, _ = res.Value["entries"].([]any)
		for _, e := range entries {
			if field(e, "type") == "instance.ready" {
				return Pass()
			}
		}
	}
	return Fail("`query-log`", "entries, among them `instance.ready`", fmt.Sprintf("%d entries %s", len(entries), res.Refusal))
}
