package verify_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs git in dir, as a contributor with no configuration of their own would.
func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "init.defaultBranch=main"}, args...)...)
	command.Dir = dir
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// committed is the one-case tree made a repository with one commit, and that commit.
func committed(t *testing.T) (root, results, commit string) {
	t.Helper()
	root, results = oneCasePassing(t)
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "one")
	return root, results, git(t, root, "rev-parse", "HEAD")
}

// commitRecorded runs the record writer over root with no commit given, and returns the commit it wrote.
func commitRecorded(t *testing.T, root, results string) string {
	t.Helper()
	status, out, findings := runWith(t, "record", "--level", "L1", "--tier", "reference", "--results", results, "--repository", "yoke", root)
	if status != 0 {
		t.Fatalf("record refused the run: %s", findings)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(out), &record); err != nil {
		t.Fatalf("the record is not one JSON object: %v\n%s", err, out)
	}
	commit, _ := record["commit"].(string)
	return commit
}

// std: yoke:record.11
func TestTheCommitIsReadFromAWorktree(t *testing.T) {
	root, results, _ := committed(t)
	worktree := filepath.Join(t.TempDir(), "worktree")
	git(t, root, "worktree", "add", "-q", "-b", "elsewhere", worktree)
	git(t, worktree, "commit", "-q", "--allow-empty", "-m", "two")
	want := git(t, worktree, "rev-parse", "HEAD")
	if got := commitRecorded(t, worktree, results); got != want {
		t.Errorf("the record from a worktree names %q, want %q", got, want)
	}
}

// std: yoke:record.12
func TestTheCommitIsReadFromPackedReferences(t *testing.T) {
	root, results, want := committed(t)
	git(t, root, "pack-refs", "--all")
	if got := commitRecorded(t, root, results); got != want {
		t.Errorf("the record over packed references names %q, want %q", got, want)
	}
}

// std: yoke:record.13
func TestARecordThatCanNameNoCommitIsRefused(t *testing.T) {
	root, results := oneCasePassing(t)
	status, out, findings := runWith(t, "record", "--level", "L1", "--tier", "reference", "--results", results, "--repository", "yoke", root)
	if status == 0 || !strings.Contains(findings, "commit") {
		t.Errorf("a tree that is no repository was recorded, status %d: %s", status, findings)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("a record was written all the same: %s", out)
	}
}
