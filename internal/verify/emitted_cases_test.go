package verify_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// emitted runs the subcommand asking for what it parsed, and reads it back.
func emitted(t *testing.T, root string) []map[string]any {
	t.Helper()
	status, out, findings := runWith(t, "descriptions", "--emit", "json", "--repository", "yoke", root)
	if status != 0 {
		t.Fatalf("descriptions refused the tree: %s", findings)
	}
	var cases []map[string]any
	if err := json.Unmarshal([]byte(out), &cases); err != nil {
		t.Fatalf("what it emitted is not JSON: %v\n%s", err, out)
	}
	return cases
}

// std: yoke:emitted-cases.01
func TestEveryLiveCaseIsEmittedWithItsFields(t *testing.T) {
	root := tree(t, map[string]string{
		"one/first.std.md": description(
			caseBlock("yoke:first.01", "the first case", fields()),
			caseBlock("yoke:first.02", "the second case", fields()),
		),
		"two/second.std.md": description(caseBlock("yoke:second.01", "the third case", fields())),
	})

	cases := emitted(t, root)

	var ids []string
	for _, c := range cases {
		id, _ := c["id"].(string)
		ids = append(ids, id)
	}
	want := "yoke:first.01,yoke:first.02,yoke:second.01"
	if strings.Join(ids, ",") != want {
		t.Fatalf("it emitted %v, want %s in the descriptions' own order", ids, want)
	}

	first := cases[0]
	if first["file"] != "one/first.std.md" {
		t.Errorf("the case does not name the file it was found in: %v", first["file"])
	}
	if first["title"] != "the first case" {
		t.Errorf("the case does not carry its title: %v", first["title"])
	}
	if struck, _ := first["struck"].(bool); struck {
		t.Errorf("a live case is emitted as struck")
	}
	emittedFields, _ := first["fields"].(map[string]any)
	for name, want := range map[string]string{
		"Level": "L1", "Method": "test", "Not applicable in": "—", "Label": "blocking",
		"Precondition": "a tree holding one description", "Action": "run the subcommand over it",
		"Expected": "it exits zero",
	} {
		if got, _ := emittedFields[name].(string); got != want {
			t.Errorf("the field %s is %q, want %q", name, got, want)
		}
	}
}

// std: yoke:emitted-cases.02
func TestAStruckCaseIsEmittedAsStruck(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the case that remains", fields()),
			"## ~~yoke:sample.02 — the case that went away~~ — its rule was withdrawn\n",
		),
	})

	cases := emitted(t, root)

	if len(cases) != 2 {
		t.Fatalf("it emitted %d cases, want the live one and the struck one", len(cases))
	}
	gone := cases[1]
	if gone["id"] != "yoke:sample.02" {
		t.Fatalf("the second case is %v", gone["id"])
	}
	if struck, _ := gone["struck"].(bool); !struck {
		t.Errorf("the struck case is not marked as struck: %v", gone)
	}
	if carried, _ := gone["fields"].(map[string]any); len(carried) != 0 {
		t.Errorf("the struck case carries fields: %v", carried)
	}
}

// std: yoke:emitted-cases.03
func TestTheCitationsComeOutAsTheyWereWritten(t *testing.T) {
	written := "specs/15.52 · arch/10-manager/03 §What install means · prj_structure/40 E1"
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "a case citing three kinds", set(fields(), "Cites", written)),
		),
	})

	cases := emitted(t, root)

	cited, _ := cases[0]["cites"].([]any)
	want := []string{"specs/15.52", "arch/10-manager/03 §What install means", "prj_structure/40 E1"}
	if len(cited) != len(want) {
		t.Fatalf("it emitted %v, want three citations", cited)
	}
	for i := range want {
		if got, _ := cited[i].(string); got != want[i] {
			t.Errorf("citation %d is %q, want %q", i, got, want[i])
		}
	}
}

// std: yoke:emitted-cases.04
func TestNothingIsEmittedForADescriptionOutOfForm(t *testing.T) {
	root := tree(t, map[string]string{
		"good.std.md": description(caseBlock("yoke:good.01", "a case in the form", fields())),
		"bad.std.md":  description(caseBlock("yoke:bad.01", "a case lacking a field", drop(fields(), "Expected"))),
	})

	status, out, findings := runWith(t, "descriptions", "--emit", "json", "--repository", "yoke", root)

	if status == 0 {
		t.Fatalf("it emitted a tree holding a description out of form: %s", out)
	}
	if !strings.Contains(findings, "yoke:bad.01") {
		t.Errorf("the finding does not name the case at fault: %s", findings)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("it emitted something all the same: %s", out)
	}
}
