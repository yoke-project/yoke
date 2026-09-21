package verify_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/verify"
)

// The eight fields of a case, in the order testing/30 fixes, with values that hold.
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
