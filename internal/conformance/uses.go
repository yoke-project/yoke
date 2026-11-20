package conformance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"time"

	"go.yaml.in/yaml/v3"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// Rewritten is the secret a plugin harness declares it rewrites, where the suite names the feature
// `rewrites`: the suite places its first value before the Core starts, and hands the unit its path in
// `CONFORMANCE_SECRET`.
const (
	Rewritten      = "token"
	RewrittenFirst = "the first value"
	RewrittenNext  = "the value the unit wrote"
)

// FixtureQuery is the question the fixture declares, so a channel has a question to name a cause on.
const FixtureQuery = "fixture.status"

// chainForm is a chain's identity as the Core writes it: 16 bytes, as 32 lower-case hex digits.
var chainForm = regexp.MustCompile(`^[0-9a-f]{32}$`)

// UsePluginCases are the plugin contract's cases for a unit that uses another, or is used: what a question
// carries, and a secret the unit rewrites. They run after the cases for streams, before the revocation.
func UsePluginCases() []Case {
	return []Case{
		{Contract: "plugin", ID: "yoke:plugin.21", Title: "a question carries who asked, its deadline and its chain, and the unit states none of them",
			Cites:        []string{"specs/50.107", "specs/50.108", "specs/50.110", "arch/50-plugin-surface/10 §The three fields of a question"},
			Precondition: "the harness of case 8, its Session open, declaring a query",
			Issues:       "a question of the type its Manifest declares, asked of the unit on the administrative surface; then `answer`",
			Requires:     "an observation `question` whose `asker` is `operator:` and a person, whose `deadline` is an instant in milliseconds later than the ask and no more than 31 s after it, and whose `chain` is 32 lower-case hex digits; the answer coming back",
			Run:          questionCarriesItsAsker},
		{Contract: "plugin", ID: "yoke:plugin.22", Title: "a unit replaces a secret it rewrites, in place, and is not restarted",
			Cites:        []string{"specs/42.30", "specs/80.56", "arch/80-security/06 §A secret the unit rewrites"},
			Precondition: "the harness of case 8, declaring the need `secret:token` and `rewrites: [ token ]`, the secret's first value placed in the instance's store, and its path handed to the unit",
			Issues:       "`rewrite` of the secret with a new value",
			Requires:     "the secret's file holding the new value, no `value.new` left beside it, and the unit at the incarnation it had before",
			Run:          secretRewritten},
	}
}

// UseInterfaceCases are the interface contract's cases for asking with a cause and following a stream.
// They run after case 9, with the producer's stream flowing.
func UseInterfaceCases() []Case {
	return []Case{
		{ID: "yoke:interface.10", Title: "a question names its cause, and a declared channel refuses it", Contract: "interface",
			Cites:        []string{"specs/70.60", "arch/70-interface-surface/10 §A question asked on it"},
			Precondition: "the harness attached to `panel`, a declared channel; the producer running, declaring the query `fixture.status`",
			Issues:       "`query` of `fixture.status` of the producer, naming the cause `q-none`",
			Requires:     "a refusal `use.cause.unknown` naming the item `q-none`: the cause reached the Core, and only a need's channel takes one",
			Run:          causeRefusedOnADeclaredChannel},
		{ID: "yoke:interface.11", Title: "a stream followed is subscribed again when its unit restarts", Contract: "interface",
			Cites:        []string{"specs/28.60", "arch/70-interface-surface/10 §When the unit bound restarts", "arch/90-sdks/02 §What using a unit adds to every family"},
			Precondition: "the producer's stream flowing, as case 9 left it",
			Issues:       "`stream-follow` of it; then, once a frame has arrived, the producer restarted on the administrative surface",
			Requires:     "a frame on the follow before the restart; and after it, on the same follow and a delivery other than the first, a frame of the next incarnation, whole",
			Run:          followedAcrossARestart},
	}
}

// rewrites is whether the described Manifest declares that it rewrites the secret the suite places.
func rewrites(manifest string) bool {
	var m struct {
		Rewrites []string `yaml:"rewrites"`
	}
	yaml.Unmarshal([]byte(manifest), &m)
	return slices.Contains(m.Rewrites, Rewritten)
}

// placeRewritten writes the secret's first value in the instance's store, in the layout of a secret its
// unit rewrites: a directory of its own, the value a file in it.
func placeRewritten(state string) error {
	dir := filepath.Join(state, "secrets", Rewritten)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "value"), []byte(RewrittenFirst), 0o600)
}

// incarnationOf is the incarnation a unit's record carries, read on the administrative surface.
func incarnationOf(ctx context.Context, op administrativev1.OperatorClient, unit string) (uint64, error) {
	res, err := op.Call(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_Read{Read: &administrativev1.Read{Kind: "unit", Identity: unit}}}))
	if err != nil {
		return 0, err
	}
	records := res.GetRead().GetRecords()
	if len(records) != 1 || records[0].GetUnit() == nil {
		return 0, fmt.Errorf("the read answered %v", res)
	}
	return records[0].GetUnit().GetObserved().GetIncarnation(), nil
}

// number is an observation's field as an integer, whether the harness wrote it as a number or a string.
func number(v any) (int64, bool) {
	switch v := v.(type) {
	case float64:
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	}
	return 0, false
}

// std: yoke:plugin.21
func questionCarriesItsAsker(r *Run) Outcome {
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
	asked := time.Now()
	got := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := op.Call(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_UnitAsk{UnitAsk: &administrativev1.UnitAsk{
			Unit: h.Hello().Unit, Type: d.Queries[0], Question: []byte("who asks")}}}))
		got <- err
	}()
	q, before, ok := observed(h, "question", 10*time.Second)
	if !ok {
		return Fail("a question of "+d.Queries[0], "an observation `question`", fmt.Sprintf("%v", before))
	}
	asker, _ := q.Fields["asker"].(string)
	deadline, hasDeadline := number(q.Fields["deadline"])
	chain, _ := q.Fields["chain"].(string)
	h.Do("answer", map[string]any{"question": q.Fields["id"], "payload": "well"})
	if len(asker) <= len("operator:") || asker[:len("operator:")] != "operator:" {
		return Fail("a question asked on the administrative surface", "`asker` = `operator:` and a person", fmt.Sprintf("%q", q.Fields["asker"]))
	}
	if !hasDeadline || deadline <= asked.UnixMilli() || deadline > asked.Add(31*time.Second).UnixMilli() {
		return Fail("a question asked at "+strconv.FormatInt(asked.UnixMilli(), 10), "a `deadline` later than the ask, and no more than 31 s after it", fmt.Sprintf("%v", q.Fields["deadline"]))
	}
	if !chainForm.MatchString(chain) {
		return Fail("a question", "a `chain` of 32 lower-case hex digits", fmt.Sprintf("%q", q.Fields["chain"]))
	}
	select {
	case err := <-got:
		if err != nil {
			return Fail("`answer`", "the administrative answer", err.Error())
		}
	case <-time.After(30 * time.Second):
		return Fail("`answer`", "the administrative answer", "none within 30 s")
	}
	return Pass()
}

// std: yoke:plugin.22
func secretRewritten(r *Run) Outcome {
	if !rewrites(r.described) {
		return Fail("`rewrite`", "a Manifest declaring `rewrites: [ "+Rewritten+" ]`, as the suite names the feature `rewrites`", r.described)
	}
	h, err := r.Unit()
	if err != nil {
		return Fail("`rewrite`", "the harness of case 8", err.Error())
	}
	op, err := r.Operator()
	if err != nil {
		return Fail("`rewrite`", "the operator projection", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	unit := h.Hello().Unit
	before, err := incarnationOf(ctx, op, unit)
	if err != nil {
		return Fail("a read of the unit", "its incarnation", err.Error())
	}
	res := h.Do("rewrite", map[string]any{"secret": Rewritten, "value": RewrittenNext})
	if res.Unrecognised {
		return Absent("rewrite")
	}
	if res.Refusal != "" || res.Value["failed"] == true {
		return Fail("`rewrite` of "+Rewritten, "the secret replaced", fmt.Sprintf("%q %v", res.Refusal, res.Value))
	}
	dir := filepath.Join(r.Tree(), "state", "secrets", Rewritten)
	value, err := os.ReadFile(filepath.Join(dir, "value"))
	if err != nil || string(value) != RewrittenNext {
		return Fail("the secret's file after `rewrite`", fmt.Sprintf("%q", RewrittenNext), fmt.Sprintf("%q %v", value, err))
	}
	if _, err := os.Stat(filepath.Join(dir, "value.new")); err == nil {
		return Fail("the secret's directory after `rewrite`", "no `value.new` left: the new value renamed over the old", "value.new is there")
	}
	after, err := incarnationOf(ctx, op, unit)
	if err != nil || after != before {
		return Fail("the unit after `rewrite`", fmt.Sprintf("the incarnation %d, not restarted", before), fmt.Sprintf("%d %v", after, err))
	}
	return Pass()
}

// std: yoke:interface.10
func causeRefusedOnADeclaredChannel(r *Run) Outcome {
	h, _, absent := attached(r, "`query`")
	if absent != nil {
		return *absent
	}
	if err := r.producing(h); err != nil {
		return Fail("the producer", "running, with its Session", err.Error())
	}
	res := h.Do("query", map[string]any{"unit": Producer, "type": FixtureQuery, "payload": "", "cause": "q-none"})
	if res.Unrecognised {
		return Absent("query")
	}
	if res.Refusal != "use.cause.unknown" || res.Value["item"] != "q-none" {
		return Fail("`query` naming the cause `q-none` on `panel`", "`use.cause.unknown` naming the item `q-none`", fmt.Sprintf("%q %v", res.Refusal, res.Value))
	}
	return Pass()
}

// followFrame waits for a frame observed on a follow, ignoring the others'.
func followFrame(h *Harness, follow string, within time.Duration) (Observation, bool) {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		o, _, ok := observed(h, "frame", time.Until(deadline))
		if !ok {
			return Observation{}, false
		}
		if fmt.Sprint(o.Fields["follow"]) == follow {
			return o, true
		}
	}
	return Observation{}, false
}

// std: yoke:interface.11
func followedAcrossARestart(r *Run) Outcome {
	h, _, absent := attached(r, "`stream-follow`")
	if absent != nil {
		return *absent
	}
	if err := r.producing(h); err != nil {
		return Fail("the producer's stream", "flowing", err.Error())
	}
	res := h.Do("stream-follow", map[string]any{"unit": Producer, "stream": ProducedStream})
	if res.Unrecognised {
		return Absent("stream-follow")
	}
	if res.Refusal != "" {
		return Fail("`stream-follow` of the producer's stream", "a follow", res.Refusal)
	}
	follow := fmt.Sprint(res.Value["follow"])
	first, ok := followFrame(h, follow, 15*time.Second)
	if !ok || !wholeFrame(first) {
		return Fail("the follow", "a frame, whole, before the restart", fmt.Sprintf("arrived=%v", ok))
	}
	op, err := r.Operator()
	if err != nil {
		return Fail("the restart", "the operator projection", err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := op.Call(ctx, request(&administrativev1.Request{Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: Producer}}})); err != nil {
		return Fail("the producer restarted", "the restart answered", err.Error())
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		o, ok := followFrame(h, follow, time.Until(deadline))
		if !ok {
			break
		}
		if fmt.Sprint(o.Fields["delivery"]) != fmt.Sprint(first.Fields["delivery"]) && wholeFrame(o) {
			return Pass()
		}
	}
	return Fail("the producer restarted", "a frame of the next incarnation, whole, on the same follow and another delivery", "none within 30 s")
}
