package verify_test

import (
	"strings"
	"testing"
)

// std: yoke:descriptions-and-markers.06
func TestEveryCaseHasOneMarkerAndEveryMarkerOneCase(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "the case two tests claim", fields()),
			caseBlock("yoke:sample.02", "the case no test performs", fields()),
			caseBlock("yoke:sample.03", "the case one test performs", fields()),
		),
		"first_test.go": "package sample\n\n" +
			"// std: yoke:sample.01\nfunc TestOne() {}\n\n" +
			"// std: yoke:sample.01\nfunc TestOneAgain() {}\n\n" +
			"// std: yoke:sample.03\nfunc TestThree() {}\n",
		"second_test.go": "package sample\n\n// std: yoke:sample.09\nfunc TestNine() {}\n",
	})

	status, _, findings := run(t, "markers", root)
	if status == 0 {
		t.Fatalf("the scan accepted a tree in three broken states: %s", findings)
	}
	for _, term := range []string{
		"yoke:sample.01", // two tests carry it
		"yoke:sample.02", // no test carries it
		"yoke:sample.09", // no case declares it
		"first_test.go",  // the file the doubled marker is in
		"second_test.go", // the file the orphan marker is in
	} {
		if !strings.Contains(findings, term) {
			t.Errorf("the findings do not name %q: %s", term, findings)
		}
	}
}

// std: yoke:descriptions-and-markers.07
func TestTheScanEmitsTheCorrespondenceItChecked(t *testing.T) {
	root := tree(t, map[string]string{
		"one/one.std.md":  description(caseBlock("yoke:one.01", "the first", fields())),
		"one/one_test.go": "package one\n\n// std: yoke:one.01\nfunc TestTheFirst() {}\n",
		"two/two.std.md":  description(caseBlock("yoke:two.01", "the second", fields())),
		"two/two_test.go": "package two\n\n// std: yoke:two.01\nfunc TestTheSecond() {}\n",
	})

	out := accepted(t, "markers", root)

	for _, pair := range [][3]string{
		{"yoke:one.01", "TestTheFirst", "one/one_test.go"},
		{"yoke:two.01", "TestTheSecond", "two/two_test.go"},
	} {
		line := lineNaming(out, pair[0])
		if line == "" {
			t.Fatalf("the map holds no entry for %s: %s", pair[0], out)
		}
		for _, term := range pair[1:] {
			if !strings.Contains(line, term) {
				t.Errorf("the entry for %s does not name %q: %s", pair[0], term, line)
			}
		}
	}
}

// std: yoke:descriptions-and-markers.08
func TestAMarkerIsFoundInEveryLanguageOfTheProject(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(
			caseBlock("yoke:sample.01", "in Go", fields()),
			caseBlock("yoke:sample.02", "in Rust", fields()),
			caseBlock("yoke:sample.03", "in Python", fields()),
			caseBlock("yoke:sample.04", "in C", fields()),
			caseBlock("yoke:sample.05", "in C++", fields()),
			caseBlock("yoke:sample.06", "in a shell script", fields()),
		),
		"go/sample_test.go": "package sample\n\n// std: yoke:sample.01\nfunc TestInGo() {}\n",
		"rust/sample.rs":    "// std: yoke:sample.02\nfn in_rust() {}\n",
		"python/sample.py":  "# std: yoke:sample.03\ndef test_in_python():\n    pass\n",
		"c/sample.c":        "/* std: yoke:sample.04 */\nvoid in_c(void) {}\n",
		"cpp/sample.cpp":    "// std: yoke:sample.05\nvoid in_cpp() {}\n",
		"shell/sample.sh":   "# std: yoke:sample.06\ncheck_in_shell() { :; }\n",
	})

	out := accepted(t, "markers", root)

	for id, name := range map[string]string{
		"yoke:sample.01": "TestInGo",
		"yoke:sample.02": "in_rust",
		"yoke:sample.03": "test_in_python",
		"yoke:sample.04": "in_c",
		"yoke:sample.05": "in_cpp",
		"yoke:sample.06": "check_in_shell",
	} {
		line := lineNaming(out, id)
		if line == "" {
			t.Errorf("the map holds no entry for %s: %s", id, out)
			continue
		}
		if !strings.Contains(line, name) {
			t.Errorf("the entry for %s does not name the test %q: %s", id, name, line)
		}
	}
}

// lineNaming returns the emitted line holding an identifier, or the empty string.
func lineNaming(out, id string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, id) {
			return line
		}
	}
	return ""
}

// std: yoke:descriptions-and-markers.11
func TestAMarkerQuotedInAFixtureIsNotAMarker(t *testing.T) {
	root := tree(t, map[string]string{
		"sample.std.md": description(caseBlock("yoke:sample.01", "the case a test performs", fields())),
		"sample_test.go": "package sample\n\n" +
			"// std: yoke:sample.01\n" +
			"func TestTheCase() {\n" +
			"\tfixture := \"// std: yoke:sample.02\\nfunc TestQuoted() {}\\n\"\n" +
			"\t_ = fixture\n" +
			"}\n",
	})

	out := accepted(t, "markers", root)

	if strings.Contains(out, "yoke:sample.02") {
		t.Errorf("a marker quoted inside a fixture was read as one: %s", out)
	}
}
