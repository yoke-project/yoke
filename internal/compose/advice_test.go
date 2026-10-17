package compose

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// advisedProject is the descriptor of advice-and-waivers.01, every one of the five combinations in it,
// with the replacements given applied.
func advisedProject(t *testing.T, replace ...string) string {
	t.Helper()
	dir := t.TempDir()
	body := "#!/bin/sh\nexit 0\n"
	os.MkdirAll(filepath.Join(dir, Components), 0o755)
	os.MkdirAll(filepath.Join(dir, Manifests), 0o755)
	os.WriteFile(filepath.Join(dir, Components, "run"), []byte(body), 0o755)
	os.WriteFile(filepath.Join(dir, Manifests, "com.example.bench.yaml"), []byte("manifest: 1\nid: com.example.bench\nprotocol: 1\n"), 0o644)
	descriptor := fmt.Sprintf(`model: 1
id: com.example.station
version: 0.1.0
arch: x86_64
core: ">=0.5"
parameters:
  remote:
    listen_port: { type: integer, required: false, default: 8080, label: "Diagnostics port, loopback only" }
units:
  acquire:
    kind: plugin
    plugin: com.example.bench
    exec: components/run
    digest: "sha256:%[1]x"
    policy: { retention: { entries: 500 } }
  setup:
    kind: oneshot
    exec: components/run
    digest: "sha256:%[1]x"
    needs: [ "storage:datasets" ]
    policy: { restart: { on_failure: true } }
channels:
  panel: { transport: local, clients: single }
  remote:
    transport: http+ws
    clients: single
    address: { class: loopback, port: "${listen_port}" }
arbitration:
  - prevails: remote
    over: [ panel ]
`, sha256.Sum256([]byte(body)))
	for i := 0; i+1 < len(replace); i += 2 {
		if !strings.Contains(descriptor, replace[i]) {
			t.Fatalf("the descriptor holds no %q", replace[i])
		}
		descriptor = strings.Replace(descriptor, replace[i], replace[i+1], 1)
	}
	os.WriteFile(filepath.Join(dir, Descriptor), []byte(descriptor), 0o644)
	return dir
}

func adviceCodes(dir string) []string {
	_, advised := Advise(dir)
	var out []string
	for _, f := range advised {
		out = append(out, f.Code+" "+f.Location)
	}
	slices.Sort(out)
	return out
}

var allFive = []string{
	"arbitration.panel.starved arbitration.0",
	"data.range.unstated units.setup",
	"oneshot.restart.unchecked units.setup.policy.restart.on_failure",
	"parameter.label.overclaims parameters.remote.listen_port.label",
	"retention.budget.small units.acquire.policy.retention.entries",
}

// std: yoke:advice-and-waivers.01
func TestEachAdvisoryIsRaisedOnItsConditionAtItsLocation(t *testing.T) {
	dir := advisedProject(t)
	if got := adviceCodes(dir); !slices.Equal(got, allFive) {
		t.Errorf("advised %v, want %v", got, allFive)
	}
	code, out := run("check", "--dir", dir)
	if code != Done {
		t.Errorf("check exited %d with advice alone:\n%s", code, out)
	}
	lines := strings.Split(out, "\n")
	firstAdvice, lastGate := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "advice") && firstAdvice < 0 {
			firstAdvice = i
		}
		if strings.HasPrefix(l, "weaker") || strings.HasPrefix(l, "refusal") {
			lastGate = i
		}
	}
	if firstAdvice < 0 || firstAdvice < lastGate {
		t.Errorf("the advice is not printed after the gate's findings:\n%s", out)
	}
	for _, c := range []struct {
		without string
		replace []string
	}{
		{allFive[0], []string{"  - prevails: remote\n    over: [ panel ]\n", "  - prevails: panel\n    over: [ remote ]\n"}},
		{allFive[1], []string{"core: \">=0.5\"\n", "core: \">=0.5\"\ndata: \">=1 <2\"\n"}},
		{allFive[2], []string{"policy: { restart: { on_failure: true } }", "policy: { restart: { on_failure: false } }"}},
		{allFive[3], []string{"loopback only", "loopback"}},
		{allFive[4], []string{"entries: 500", "entries: 50000"}},
	} {
		want := slices.DeleteFunc(slices.Clone(allFive), func(s string) bool { return s == c.without })
		if got := adviceCodes(advisedProject(t, c.replace...)); !slices.Equal(got, want) {
			t.Errorf("without %s: advised %v", c.without, got)
		}
	}
	refused := advisedProject(t, "kind: oneshot", "kind: daemon")
	if code, out := run("check", "--dir", refused); code != Refused || strings.Contains(out, "advice") {
		t.Errorf("a refused project: %d\n%s", code, out)
	}
}

// std: yoke:advice-and-waivers.02
func TestTheThresholdsAndTheWordsAreExact(t *testing.T) {
	for _, c := range []struct {
		replace []string
		raised  bool
	}{
		{[]string{"entries: 500", "age: 1d"}, false},
		{[]string{"entries: 500", "age: 23h"}, true},
		{[]string{"entries: 500", "bytes: 5MB"}, false},
		{[]string{"entries: 500", "bytes: 4MB"}, true},
		{[]string{"entries: 500", "entries: 10000"}, false},
		{[]string{"entries: 500", "entries: 0"}, false},
		{[]string{"entries: 500", "entries: 50000", "policy: { restart: { on_failure: true } }", "policy: { retention: { age: 23h } }"}, false},
	} {
		got := slices.ContainsFunc(adviceCodes(advisedProject(t, c.replace...)), func(s string) bool { return strings.HasPrefix(s, RetentionBudgetSmall) })
		if got != c.raised {
			t.Errorf("%v: retention.budget.small raised %v", c.replace, got)
		}
	}
	routable := func(caller string) []string {
		return []string{"address: { class: loopback, port: \"${listen_port}\" }",
			"address: { class: routable, host: 127.0.0.1, port: \"${listen_port}\", security: { transport: encrypted, caller: " + caller + " } }"}
	}
	for _, c := range []struct {
		replace []string
		raised  bool
	}{
		{[]string{"Diagnostics port, loopback only", "SECURE port"}, true},
		{[]string{"Diagnostics port, loopback only", "insecure port"}, false},
		{[]string{"Diagnostics port, loopback only", "only"}, true},
		{append([]string{"Diagnostics port, loopback only", "only"}, routable("authenticated")...), false},
	} {
		got := slices.ContainsFunc(adviceCodes(advisedProject(t, c.replace...)), func(s string) bool { return strings.HasPrefix(s, ParameterLabelOverclaim) })
		if got != c.raised {
			t.Errorf("%v: parameter.label.overclaims raised %v", c.replace, got)
		}
	}
}

// std: yoke:advice-and-waivers.03
func TestAWaiverSilencesOnePlaceAndAStaleOneIsListed(t *testing.T) {
	dir := advisedProject(t)
	record := filepath.Join(dir, Advisories)
	at := "parameters.remote.listen_port.label"
	for _, args := range [][]string{
		{"waive", "style.taste", "--at", at, "--reason", "because"},
		{"waive", ParameterLabelOverclaim, "--at", at, "--reason", " "},
		{"waive", ParameterLabelOverclaim, "--at", "units.acquire", "--reason", "because"},
	} {
		before, _ := os.ReadFile(record)
		if code, out := run(append(args, "--dir", dir)...); code != Refused {
			t.Errorf("%v exited %d: %s", args, code, out)
		}
		if after, _ := os.ReadFile(record); !bytes.Equal(before, after) {
			t.Errorf("%v changed advisories.yaml", args)
		}
	}
	must(t, "waive", ParameterLabelOverclaim, "--at", at, "--reason", "Reviewed with the operator's manual; this bench is on an isolated segment.", "--dir", dir)
	before, _ := os.ReadFile(record)
	if code, out := run("waive", ParameterLabelOverclaim, "--at", at, "--reason", "again", "--dir", dir); code != Refused {
		t.Errorf("a second waiver exited %d: %s", code, out)
	}
	if after, _ := os.ReadFile(record); !bytes.Equal(before, after) {
		t.Error("a refused second waiver changed advisories.yaml")
	}
	if written(t, record, "waived.0.code") != ParameterLabelOverclaim || written(t, record, "waived.0.at") != at ||
		!strings.Contains(fmt.Sprint(written(t, record, "waived.0.reason")), "isolated segment") || written(t, record, "waived.0.waived_at") == nil {
		t.Errorf("advisories.yaml holds:\n%s", before)
	}
	_, out := run("check", "--dir", dir)
	if strings.Contains(out, ParameterLabelOverclaim) || strings.Count(out, "\nadvice") != 4 || !strings.Contains(out, "waived: 1 advisory item,") {
		t.Errorf("check with one waiver:\n%s", out)
	}
	if _, out := run("waivers", "--dir", dir); !strings.Contains(out, "isolated segment") || strings.Contains(out, "stale") {
		t.Errorf("waivers:\n%s", out)
	}
	descriptor := filepath.Join(dir, Descriptor)
	data, _ := os.ReadFile(descriptor)
	os.WriteFile(descriptor, []byte(strings.Replace(string(data), "loopback only", "loopback", 1)), 0o644)
	if _, out := run("waivers", "--dir", dir); !strings.Contains(out, "stale") || !strings.Contains(out, "isolated segment") {
		t.Errorf("waivers after the change:\n%s", out)
	}
	if _, out := run("check", "--dir", dir); strings.Contains(out, "waived:") {
		t.Errorf("check after the change still waives:\n%s", out)
	}
	if written(t, record, "waived.0.code") != ParameterLabelOverclaim {
		t.Error("the stale waiver was dropped")
	}
	bundle, err := archive(dir)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bundle, []byte(Advisories)) {
		t.Error("the bundle holds advisories.yaml")
	}
}
