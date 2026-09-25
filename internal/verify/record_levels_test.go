package verify_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// A tree with one case at L1 and one at L3, and the tests performing them.
func twoLevels(t *testing.T, results string) string {
	t.Helper()
	return tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the unit case", fields()),
			caseBlock("yoke:sample.02", "the instance case", set(fields(), "Level", "L3")),
		),
		"sample_test.go": "package sample\n\n" +
			"// std: yoke:sample.01\nfunc TestUnit() {}\n\n" +
			"// std: yoke:sample.02\nfunc TestInstance() {}\n",
		"results.json": results,
	})
}

// std: yoke:record-per-level.01
func TestAResultForAnotherLevelIsLeftToItsRecord(t *testing.T) {
	root := twoLevels(t, `{"Action":"pass","Package":"sample","Test":"TestUnit"}`+"\n"+
		`{"Action":"pass","Package":"sample","Test":"TestInstance"}`+"\n")
	results := filepath.Join(root, "results.json")

	for level, want := range map[string]string{"L1": "yoke:sample.01", "L3": "yoke:sample.02"} {
		record := recordOf(t, root, "--level", level, "--results", results)
		byID, order := entries(t, record)
		if strings.Join(order, ",") != want {
			t.Errorf("the %s record holds %v, want only %s", level, order, want)
		}
		resultIs(t, byID, want, "pass")
		if record["state"] != "passed" {
			t.Errorf("the %s record is %v, want passed", level, record["state"])
		}
	}
}

// std: yoke:record-per-level.02
func TestAResultAnsweringNoCaseStillRefuses(t *testing.T) {
	root := twoLevels(t, `{"Action":"pass","Package":"sample","Test":"TestInstance"}`+"\n"+
		`{"Action":"pass","Package":"sample","Test":"TestNobodyClaims"}`+"\n")

	status, out, findings := runWith(t, "record", "--level", "L3",
		"--results", filepath.Join(root, "results.json"), "--repository", "yoke", root)

	if status == 0 {
		t.Fatalf("the record was written around an unattributable result: %s", out)
	}
	if !strings.Contains(findings, "TestNobodyClaims") {
		t.Errorf("the finding does not name the test: %s", findings)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("a record was written all the same: %s", out)
	}
}
