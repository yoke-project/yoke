package verify_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// A tree with one case, the test performing it, and a Go runner's output saying that test passed.
func oneCasePassing(t *testing.T) (root, results string) {
	t.Helper()
	root = tree(t, map[string]string{
		"sample.std.md":  description(caseBlock("yoke:sample.01", "the case", fields())),
		"sample_test.go": "package sample\n\n// std: yoke:sample.01\nfunc TestTheCase() {}\n",
		"results.json":   `{"Action":"pass","Package":"sample","Test":"TestTheCase"}` + "\n",
	})
	return root, filepath.Join(root, "results.json")
}

// std: yoke:record.01
func TestARunWritesOneRecordCarryingEveryField(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.02", "the second, declared first", fields()),
			caseBlock("yoke:sample.01", "the first, declared second", fields()),
		),
		"sample_test.go": "package sample\n\n" +
			"// std: yoke:sample.02\nfunc TestTwo() {}\n\n" +
			"// std: yoke:sample.01\nfunc TestOne() {}\n",
		"results.json": `{"Action":"pass","Package":"sample","Test":"TestTwo"}` + "\n" +
			`{"Action":"pass","Package":"sample","Test":"TestOne"}` + "\n",
	})

	record := recordOf(t, root,
		"--level", "L1", "--tier", "reference",
		"--environment", "architecture=amd64",
		"--environment", "deployment form=service",
		"--ran", "yoke-verify=0.1.0@0f1e2d3",
		"--started", "2026-09-22T10:00:00Z", "--finished", "2026-09-22T10:00:05Z",
		"--commit", "0f1e2d3c4b5a69788796a5b4c3d2e1f0abcdef01",
		"--results", filepath.Join(root, "results.json"))

	if schema, ok := record["schema"].(float64); !ok || schema < 1 {
		t.Errorf("schema is %v, want an integer of at least one", record["schema"])
	}
	for field, want := range map[string]string{
		"repository": "yoke",
		"commit":     "0f1e2d3c4b5a69788796a5b4c3d2e1f0abcdef01",
		"level":      "L1",
		"tier":       "reference",
		"started":    "2026-09-22T10:00:00Z",
		"finished":   "2026-09-22T10:00:05Z",
		"state":      "passed",
	} {
		if got, _ := record[field].(string); got != want {
			t.Errorf("%s is %q, want %q", field, got, want)
		}
	}
	if blocks, ok := record["blocks"].(bool); !ok || blocks {
		t.Errorf("blocks is %v, want false when every case passed", record["blocks"])
	}
	environment, _ := record["environment"].(map[string]any)
	if environment["architecture"] != "amd64" || environment["deployment form"] != "service" {
		t.Errorf("the environment is %v, want the two dimensions the run fixed", record["environment"])
	}
	ran, _ := record["ran"].([]any)
	if len(ran) != 1 {
		t.Fatalf("ran holds %v, want the one artifact the run involved", record["ran"])
	}
	artifact, _ := ran[0].(map[string]any)
	if artifact["name"] != "yoke-verify" || artifact["version"] != "0.1.0" || artifact["commit"] != "0f1e2d3" {
		t.Errorf("the artifact is %v, want its name, version and commit", artifact)
	}

	_, order := entries(t, record)
	want := []string{"yoke:sample.01", "yoke:sample.02"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("the cases are in the order %v, want %v — a record differs only where the runs do", order, want)
	}
}

// std: yoke:record.02
func TestAResultReachesItsCaseThroughTheMap(t *testing.T) {
	root, results := oneCasePassing(t)

	record := recordOf(t, root, "--level", "L1", "--results", results)

	byID, order := entries(t, record)
	resultIs(t, byID, "yoke:sample.01", "pass")
	for _, id := range order {
		if strings.Contains(id, "TestTheCase") {
			t.Errorf("an entry answers to a test's name: %s", id)
		}
	}
	if entry := byID["yoke:sample.01"]; entry["test"] != nil {
		t.Errorf("the entry carries the test's name as a field of its own: %v", entry)
	}
}

// std: yoke:record.03
func TestARunnersSkipIsRecordedAsAFailure(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the case skipped", fields()),
			caseBlock("yoke:sample.02", "the case that ran", fields()),
		),
		"sample_test.go": "package sample\n\n" +
			"// std: yoke:sample.01\nfunc TestSkipped() {}\n\n" +
			"// std: yoke:sample.02\nfunc TestRan() {}\n",
		"results.json": `{"Action":"output","Package":"sample","Test":"TestSkipped","Output":"    sample_test.go:4: the engine is absent\n"}` + "\n" +
			`{"Action":"skip","Package":"sample","Test":"TestSkipped"}` + "\n" +
			`{"Action":"pass","Package":"sample","Test":"TestRan"}` + "\n",
	})

	record := recordOf(t, root, "--level", "L1", "--results", filepath.Join(root, "results.json"))

	byID, _ := entries(t, record)
	resultIs(t, byID, "yoke:sample.01", "fail")
	resultIs(t, byID, "yoke:sample.02", "pass")
	detail, _ := byID["yoke:sample.01"]["detail"].(string)
	if !strings.Contains(detail, "the engine is absent") {
		t.Errorf("the detail does not say what the runner reported: %q", detail)
	}
}

// std: yoke:record.04
func TestACaseNoResultAnswersForIsAbsent(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the first", fields()),
			caseBlock("yoke:sample.02", "the second", fields()),
			caseBlock("yoke:sample.03", "the one nothing answers for", fields()),
		),
		"sample_test.go": "package sample\n\n" +
			"// std: yoke:sample.01\nfunc TestOne() {}\n\n" +
			"// std: yoke:sample.02\nfunc TestTwo() {}\n\n" +
			"// std: yoke:sample.03\nfunc TestThree() {}\n",
		"results.json": `{"Action":"pass","Package":"sample","Test":"TestOne"}` + "\n" +
			`{"Action":"pass","Package":"sample","Test":"TestTwo"}` + "\n",
	})

	record := recordOf(t, root, "--level", "L1", "--results", filepath.Join(root, "results.json"))

	byID, _ := entries(t, record)
	resultIs(t, byID, "yoke:sample.03", "absent")
	if record["state"] != "failed" {
		t.Errorf("state is %v, want failed when a case is absent", record["state"])
	}
	if blocks, _ := record["blocks"].(bool); !blocks {
		t.Errorf("blocks is false, and an absent blocking case stops what its level stops")
	}
}

// std: yoke:record.05
func TestNotApplicableIsComputedFromTheDescription(t *testing.T) {
	inapplicable := set(fields(), "Not applicable in", "container engine: docker")
	files := map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the case that ran", fields()),
			caseBlock("yoke:sample.02", "the case the environment excludes", inapplicable),
		),
		"sample_test.go": "package sample\n\n" +
			"// std: yoke:sample.01\nfunc TestOne() {}\n\n" +
			"// std: yoke:sample.02\nfunc TestTwo() {}\n",
		"results.json": `{"Action":"pass","Package":"sample","Test":"TestOne"}` + "\n",
	}

	t.Run("in the environment it names", func(t *testing.T) {
		root := tree(t, files)
		record := recordOf(t, root, "--level", "L1",
			"--environment", "container engine=docker",
			"--results", filepath.Join(root, "results.json"))

		byID, _ := entries(t, record)
		resultIs(t, byID, "yoke:sample.02", "not applicable")
		if record["state"] != "passed" {
			t.Errorf("state is %v, and a case that does not apply is not a failure", record["state"])
		}
		if blocks, _ := record["blocks"].(bool); blocks {
			t.Errorf("blocks is true, and a case that does not apply stops nothing")
		}
	})

	t.Run("in an environment it does not", func(t *testing.T) {
		root := tree(t, files)
		record := recordOf(t, root, "--level", "L1",
			"--environment", "container engine=podman",
			"--results", filepath.Join(root, "results.json"))

		byID, _ := entries(t, record)
		resultIs(t, byID, "yoke:sample.02", "absent")
	})
}

// std: yoke:record.06
func TestTheLabelIsRecordedAsTheDescriptionDeclaredIt(t *testing.T) {
	deferred := set(fields(), "Label", "not blocking — yoke-project/yoke#4")
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the blocking one", fields()),
			caseBlock("yoke:sample.02", "the one waiting on an item", deferred),
		),
		"sample_test.go": "package sample\n\n" +
			"// std: yoke:sample.01\nfunc TestOne() {}\n\n" +
			"// std: yoke:sample.02\nfunc TestTwo() {}\n",
		"results.json": `{"Action":"pass","Package":"sample","Test":"TestOne"}` + "\n" +
			`{"Action":"pass","Package":"sample","Test":"TestTwo"}` + "\n",
	})

	record := recordOf(t, root, "--level", "L1", "--results", filepath.Join(root, "results.json"))

	byID, _ := entries(t, record)
	if label, _ := byID["yoke:sample.01"]["label"].(string); label != "blocking" {
		t.Errorf("the label is %q, want blocking", label)
	}
	if label, _ := byID["yoke:sample.02"]["label"].(string); label != "not blocking" {
		t.Errorf("the label is %q, want not blocking", label)
	}
	if item, _ := byID["yoke:sample.02"]["item"].(string); item != "yoke-project/yoke#4" {
		t.Errorf("the item is %q, want the one the label cites", item)
	}
}

// std: yoke:record.07
func TestBlocksIsWhatFailedAndNotHowMuch(t *testing.T) {
	deferred := set(fields(), "Label", "not blocking — yoke-project/yoke#4")
	files := func(results string) map[string]string {
		return map[string]string{
			"sample.std.md": description(
				caseBlock("yoke:sample.01", "the blocking one", fields()),
				caseBlock("yoke:sample.02", "the one waiting on an item", deferred),
			),
			"sample_test.go": "package sample\n\n" +
				"// std: yoke:sample.01\nfunc TestOne() {}\n\n" +
				"// std: yoke:sample.02\nfunc TestTwo() {}\n",
			"results.json": results,
		}
	}

	for _, run := range []struct {
		name    string
		results string
		blocks  bool
	}{
		{
			"only a not-blocking case fails",
			`{"Action":"pass","Package":"sample","Test":"TestOne"}` + "\n" +
				`{"Action":"fail","Package":"sample","Test":"TestTwo"}` + "\n",
			false,
		},
		{
			"a blocking case fails",
			`{"Action":"fail","Package":"sample","Test":"TestOne"}` + "\n" +
				`{"Action":"pass","Package":"sample","Test":"TestTwo"}` + "\n",
			true,
		},
		{
			"a blocking case is absent",
			`{"Action":"pass","Package":"sample","Test":"TestTwo"}` + "\n",
			true,
		},
	} {
		t.Run(run.name, func(t *testing.T) {
			root := tree(t, files(run.results))
			record := recordOf(t, root, "--level", "L1", "--results", filepath.Join(root, "results.json"))

			if blocks, _ := record["blocks"].(bool); blocks != run.blocks {
				t.Errorf("blocks is %v, want %v", blocks, run.blocks)
			}
			if record["state"] != "failed" {
				t.Errorf("state is %v, want failed — the label never changes whether it failed", record["state"])
			}
		})
	}
}

// std: yoke:record.08
func TestALevelWhosePredecessorFailedIsNotReached(t *testing.T) {
	root, _ := oneCasePassing(t)

	record := recordOf(t, root, "--level", "L1", "--not-reached")

	if record["state"] != "not reached" {
		t.Errorf("state is %v, want not reached", record["state"])
	}
	byID, _ := entries(t, record)
	resultIs(t, byID, "yoke:sample.01", "absent")
	for id, entry := range byID {
		if entry["result"] == "not reached" {
			t.Errorf("%s carries the level's state as its result", id)
		}
	}
}

// std: yoke:record.09
func TestAResultNoCaseClaimsRefusesTheRecord(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md":  description(caseBlock("yoke:sample.01", "the case", fields())),
		"sample_test.go": "package sample\n\n// std: yoke:sample.01\nfunc TestTheCase() {}\n",
		"results.json": `{"Action":"pass","Package":"sample","Test":"TestTheCase"}` + "\n" +
			`{"Action":"pass","Package":"sample","Test":"TestNobodyClaims"}` + "\n",
	})

	status, out, findings := runWith(t, "record", "--level", "L1",
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

// std: yoke:record.10
func TestBothFormsARunOfThisProjectEmitsAreRead(t *testing.T) {
	files := map[string]string{
		"sample.std.md":  description(caseBlock("yoke:sample.01", "the case", fields())),
		"sample_test.go": "package sample\n\n// std: yoke:sample.01\nfunc TestTheCase() {}\n",
		"go.json":        `{"Action":"pass","Package":"sample","Test":"TestTheCase"}` + "\n",
		"checks.txt":     "pass  yoke:sample.01\n",
	}

	fromGo := tree(t, files)
	fromChecks := tree(t, files)

	go1 := recordOf(t, fromGo, "--level", "L1", "--results", filepath.Join(fromGo, "go.json"))
	checks := recordOf(t, fromChecks, "--level", "L1", "--results", filepath.Join(fromChecks, "checks.txt"))

	byGo, _ := entries(t, go1)
	byChecks, _ := entries(t, checks)
	resultIs(t, byGo, "yoke:sample.01", "pass")
	resultIs(t, byChecks, "yoke:sample.01", "pass")

	if go1["state"] != checks["state"] || go1["blocks"] != checks["blocks"] {
		t.Errorf("the two forms disagree: %v %v against %v %v",
			go1["state"], go1["blocks"], checks["state"], checks["blocks"])
	}
}
