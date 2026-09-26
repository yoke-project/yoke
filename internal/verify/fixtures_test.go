package verify_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/verify"
)

// The eight fields of a case, in the order a description's form fixes, with values that hold.
func fields() [][2]string {
	return [][2]string{
		{"Cites", "testing/30 §A case"},
		{"Level", "L1"},
		{"Method", "test"},
		{"Not applicable in", "—"},
		{"Label", "blocking"},
		{"Precondition", "a tree holding one description"},
		{"Action", "run the subcommand over it"},
		{"Expected", "it exits zero"},
	}
}

func set(fs [][2]string, name, value string) [][2]string {
	for i := range fs {
		if fs[i][0] == name {
			fs[i][1] = value
		}
	}
	return fs
}

func drop(fs [][2]string, name string) [][2]string {
	out := make([][2]string, 0, len(fs))
	for _, f := range fs {
		if f[0] != name {
			out = append(out, f)
		}
	}
	return out
}

func caseBlock(id, title string, fs [][2]string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s — %s\n\n| Field | Value |\n| --- | --- |\n", id, title)
	for _, f := range fs {
		fmt.Fprintf(&b, "| **%s** | %s |\n", f[0], f[1])
	}
	return b.String()
}

func description(blocks ...string) string {
	return "# A feature\n\n| | |\n| --- | --- |\n" +
		"| **Feature** | something observable happens |\n" +
		"| **Planning item** | yoke-project/yoke#3 |\n\n" +
		strings.Join(blocks, "\n")
}

// tree writes the files given, relative to a new temporary directory, and returns its path.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// run performs one subcommand over a tree, as `yoke-verify <subcommand> --repository yoke <root>`.
func run(t *testing.T, subcommand, root string) (status int, out, findings string) {
	t.Helper()
	var o, e strings.Builder
	status = verify.Run([]string{subcommand, "--repository", "yoke", root}, &o, &e)
	return status, o.String(), e.String()
}

// refused fails the test unless the subcommand refused the tree and said why, naming each term.
func refused(t *testing.T, subcommand, root string, terms ...string) {
	t.Helper()
	status, _, findings := run(t, subcommand, root)
	if status == 0 {
		t.Fatalf("%s accepted the tree; findings: %q", subcommand, findings)
	}
	for _, term := range terms {
		if !strings.Contains(findings, term) {
			t.Errorf("the finding does not name %q: %s", term, findings)
		}
	}
}

// accepted fails the test unless the subcommand accepted the tree with nothing to report.
func accepted(t *testing.T, subcommand, root string) string {
	t.Helper()
	status, out, findings := run(t, subcommand, root)
	if status != 0 {
		t.Fatalf("%s refused the tree: %s", subcommand, findings)
	}
	if strings.TrimSpace(findings) != "" {
		t.Errorf("%s reported a finding on a tree that holds: %s", subcommand, findings)
	}
	return out
}

// runWith performs one subcommand with the arguments given, over a tree.
func runWith(t *testing.T, subcommand string, args ...string) (status int, out, findings string) {
	t.Helper()
	var o, e strings.Builder
	status = verify.Run(append([]string{subcommand}, args...), &o, &e)
	return status, o.String(), e.String()
}

// recordOf writes a record over a tree and reads it back, failing unless one was written. A fixture tree
// is no repository, so it is given a commit unless the test gives its own.
func recordOf(t *testing.T, root string, args ...string) map[string]any {
	t.Helper()
	if !slices.Contains(args, "--commit") {
		args = append(args, "--commit", "0f1e2d3c4b5a69788796a5b4c3d2e1f0abcdef01")
	}
	status, out, findings := runWith(t, "record", append(args, "--repository", "yoke", root)...)
	if status != 0 {
		t.Fatalf("record refused the run: %s", findings)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("the record is not one JSON object: %v\n%s", err, out)
	}
	return record
}

// entries returns a record's cases by identifier, in the order the record gives them.
func entries(t *testing.T, record map[string]any) (map[string]map[string]any, []string) {
	t.Helper()
	raw, ok := record["cases"].([]any)
	if !ok {
		t.Fatalf("the record carries no cases: %v", record["cases"])
	}
	byID := map[string]map[string]any{}
	var order []string
	for _, e := range raw {
		entry, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("a case entry is not an object: %v", e)
		}
		id, _ := entry["id"].(string)
		byID[id] = entry
		order = append(order, id)
	}
	return byID, order
}

// resultIs fails the test unless a case was recorded with the result given.
func resultIs(t *testing.T, byID map[string]map[string]any, id, want string) {
	t.Helper()
	entry, declared := byID[id]
	if !declared {
		t.Fatalf("the record holds no entry for %s", id)
	}
	if got, _ := entry["result"].(string); got != want {
		t.Errorf("%s is recorded %q, want %q", id, got, want)
	}
}
