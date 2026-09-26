package conformance

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"go.yaml.in/yaml/v3"
)

// Cases are the plugin contract's cases, in the order a run performs them: each one's test is the
// function that performs it, and the rendering is made from the fields each one carries.
func Cases() []Case {
	return []Case{
		{ID: "yoke:plugin.01", Title: "the Manifest a library generates is one the Core reads",
			Cites:        []string{"specs/90.36", "specs/90.38", "specs/42.1", "arch/90-sdks/06 §The declaration a plugin library produces"},
			Precondition: "the harness, launched by the suite with no deployment around it",
			Issues:       "`describe`, then the Manifest written into the Plugin directory of a Core that is started",
			Requires:     "a Manifest; the Core reads it, becomes ready and launches the harness as a unit, which says hello with its unit",
			Run:          describedAndLaunched},
		{ID: "yoke:plugin.02", Title: "a registration is accepted, withholding by name what is not granted",
			Cites:        []string{"specs/50.24", "specs/50.26", "specs/50.30", "specs/90.9", "arch/50-plugin-surface/03 §Three outcomes"},
			Precondition: "the harness the Core launched, its plugin granted nothing",
			Issues:       "`start`",
			Requires:     "`accepted with restrictions`, with every capability, stream, command and query the Manifest declares withheld, each named",
			Run:          acceptedWithRestrictions},
		{ID: "yoke:plugin.03", Title: "a spent token is refused at authentication, once",
			Cites:        []string{"specs/50.23", "specs/90.30", "arch/50-plugin-surface/03 §The stage travels with the refusal"},
			Precondition: "the harness of case 2, admitted",
			Issues:       "`start` again, with the token it was launched with",
			Requires:     "a refusal `admission.auth.consumed` at `authentication`",
			Run:          spentTokenRefused},
		{ID: "yoke:plugin.04", Title: "nothing is emitted on a stream the Core has not activated",
			Cites:        []string{"specs/90.33", "arch/90-sdks/06 §It may not create a stream's transport"},
			Precondition: "the harness of case 2, its Session open and no stream activated",
			Issues:       "`emit` on the stream its Manifest declares",
			Requires:     "a refusal `stream.inactive`",
			Run:          emitBeforeActivation},
		{ID: "yoke:plugin.05", Title: "an orderly close ends the Session, and the process with it",
			Cites:        []string{"specs/50.49", "specs/90.29", "specs/50.41", "arch/90-sdks/06 §It may not hide the end of a Session"},
			Precondition: "the harness of case 2, its Session open",
			Issues:       "`close`",
			Requires:     "the end observed as a close the unit made, and then the harness gone",
			Run:          closedAndGone},
		{ID: "yoke:plugin.06", Title: "the next life is a new process, admitted afresh",
			Cites:        []string{"specs/50.40", "specs/50.41", "specs/50.14", "arch/50-plugin-surface/04 §Identity"},
			Precondition: "the harness of case 5, gone",
			Issues:       "nothing, until the Core launches the unit again; then `start`",
			Requires:     "a new process saying hello with the same unit, and an acceptance",
			Run:          nextLifeAdmitted},
	}
}

// std: yoke:plugin.01
func describedAndLaunched(r *Run) Outcome {
	var m struct{ ID string }
	if err := yaml.Unmarshal([]byte(r.Described()), &m); err != nil || m.ID == "" {
		return Fail("describe", "a Manifest with an identity", r.Described())
	}
	h, err := r.Unit()
	if err != nil {
		return Fail("the Manifest, to a Core", "the harness launched as a unit, saying hello", err.Error())
	}
	if h.Hello().Unit == "" {
		return Fail("the Manifest, to a Core", "a hello carrying the unit", "none")
	}
	return Pass()
}

// std: yoke:plugin.02
func acceptedWithRestrictions(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("start", "a harness", err.Error())
	}
	res := h.Do("start", nil)
	if res.Unrecognised {
		return Absent("start")
	}
	if res.Value["outcome"] != "accepted with restrictions" {
		return Fail("start", "accepted with restrictions", fmt.Sprint(res.Value["outcome"], " ", res.Refusal))
	}
	var m struct {
		Streams      []struct{ ID string }
		Commands     []struct{ ID string }
		Queries      []struct{ ID string }
		Capabilities []struct{ Name string }
	}
	yaml.Unmarshal([]byte(r.Described()), &m)
	want := map[string][]string{}
	for _, s := range m.Capabilities {
		want["capabilities"] = append(want["capabilities"], s.Name)
	}
	for _, s := range m.Streams {
		want["streams"] = append(want["streams"], s.ID)
	}
	for _, s := range m.Commands {
		want["commands"] = append(want["commands"], s.ID)
	}
	for _, s := range m.Queries {
		want["queries"] = append(want["queries"], s.ID)
	}
	withheld, _ := res.Value["withheld"].(map[string]any)
	for _, list := range []string{"capabilities", "streams", "commands", "queries"} {
		if got := stringList(withheld[list]); !slices.Equal(sorted(got), sorted(want[list])) {
			return Fail("start", fmt.Sprintf("the withheld %s %v", list, want[list]), fmt.Sprint(got))
		}
	}
	return Pass()
}

// std: yoke:plugin.03
func spentTokenRefused(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("start again", "a harness", err.Error())
	}
	res := h.Do("start", nil)
	if res.Refusal != "admission.auth.consumed" || res.Value["stage"] != "authentication" {
		return Fail("start again", "admission.auth.consumed at authentication", fmt.Sprintf("%q at %v", res.Refusal, res.Value["stage"]))
	}
	return Pass()
}

// std: yoke:plugin.04
func emitBeforeActivation(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("emit", "a harness", err.Error())
	}
	var m struct{ Streams []struct{ ID string } }
	yaml.Unmarshal([]byte(r.Described()), &m)
	if len(m.Streams) == 0 {
		return Fail("emit", "a Manifest declaring a stream", r.Described())
	}
	res := h.Do("emit", map[string]any{"stream": m.Streams[0].ID, "payload": "frame"})
	if res.Unrecognised {
		return Absent("emit")
	}
	if res.Refusal != "stream.inactive" {
		return Fail("emit "+m.Streams[0].ID, "stream.inactive", fmt.Sprintf("%q %v", res.Refusal, res.Value))
	}
	return Pass()
}

// std: yoke:plugin.05
func closedAndGone(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("close", "a harness", err.Error())
	}
	res := h.Do("close", nil)
	if res.Unrecognised {
		return Absent("close")
	}
	var end Observation
	for _, o := range res.Before {
		if o.Kind == "session-ended" {
			end = o
		}
	}
	if end.Kind == "" {
		o, ok := h.Observe(10 * time.Second)
		if !ok || o.Kind != "session-ended" {
			return Fail("close", "the end observed", fmt.Sprintf("%v", o))
		}
		end = o
	}
	if end.Fields["closed"] != true {
		return Fail("close", "a close the unit made", fmt.Sprint(end.Fields))
	}
	if !h.Gone(10 * time.Second) {
		return Fail("close", "the harness gone", "it is still connected")
	}
	return Pass()
}

// std: yoke:plugin.06
func nextLifeAdmitted(r *Run) Outcome {
	h, err := r.NextUnit(60 * time.Second)
	if err != nil {
		return Fail("nothing", "the unit launched again, saying hello", err.Error())
	}
	res := h.Do("start", nil)
	if outcome, _ := res.Value["outcome"].(string); outcome != "accepted" && outcome != "accepted with restrictions" {
		return Fail("start", "an acceptance", fmt.Sprintf("%v %q", res.Value, res.Refusal))
	}
	return Pass()
}

func stringList(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, s := range list {
		if text, ok := s.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}
