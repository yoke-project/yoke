package conformance_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/conformance"
	"github.com/yoke-project/yoke/internal/verify"
)

func twoCases() conformance.Contract {
	return conformance.Contract{Name: "plugin", Feature: "the plugin contract, as the suite measures it", Item: "yoke-project/yoke#19",
		Cases: []conformance.Case{
			{ID: "yoke:plugin.01", Title: "the first", Cites: []string{"specs/50.9", "arch/50-plugin-surface/03 §The nine stages"},
				Precondition: "a harness", Issues: "start", Requires: "an acceptance"},
			{ID: "yoke:plugin.02", Title: "the second", Cites: []string{"specs/50.47"},
				Precondition: "an open Session", Issues: "close", Requires: "the end surfaced"},
		}}
}

// check runs the description check over one file, as its repository's test verb does.
func check(t *testing.T, name string, content []byte) (int, string) {
	t.Helper()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, name), content, 0o644)
	var out, findings strings.Builder
	return verify.Run([]string{"descriptions", "--repository", "yoke", root}, &out, &findings), findings.String()
}

// std: yoke:the-rendered-description.01
func TestAContractsCasesAreRenderedInTheForm(t *testing.T) {
	rendering := conformance.Render(twoCases())
	if status, findings := check(t, "plugin.std.md", rendering); status != 0 {
		t.Fatalf("the rendering was refused: %s\n%s", findings, rendering)
	}
	text := string(rendering)
	for _, want := range []string{
		"## yoke:plugin.01 — the first",
		"| **Cites** | specs/50.9 · arch/50-plugin-surface/03 §The nine stages |",
		"| **Level** | L2 |", "| **Method** | test |", "| **Not applicable in** | — |", "| **Label** | blocking |",
		"| **Precondition** | a harness |", "| **Action** | start |", "| **Expected** | an acceptance |",
		"## yoke:plugin.02 — the second",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the rendering lacks %q:\n%s", want, text)
		}
	}
}

// std: yoke:the-rendered-description.02
func TestTheCommittedRenderingIsCurrent(t *testing.T) {
	committed := "plugin.std.md"
	if err := conformance.Current(committed, conformance.Plugin()); err != nil {
		t.Fatalf("the committed rendering is not current: %v", err)
	}
	b, _ := os.ReadFile(committed)
	stale := filepath.Join(t.TempDir(), "plugin.std.md")
	os.WriteFile(stale, append(b, []byte("\nedited by hand\n")...), 0o644)
	if err := conformance.Current(stale, conformance.Plugin()); err == nil || !strings.Contains(err.Error(), stale) {
		t.Errorf("a stale copy gave %v", err)
	}
}

// std: yoke:the-rendered-description.03
func TestL2IsAcceptedInARenderingAndNowhereElse(t *testing.T) {
	rendering := conformance.Render(twoCases())
	if status, findings := check(t, "plugin.std.md", rendering); status != 0 {
		t.Fatalf("a rendering was refused: %s", findings)
	}
	byHand := bytes.Replace(rendering, []byte(conformance.RenderedLine+"\n"), nil, 1)
	if status, findings := check(t, "plugin.std.md", byHand); status == 0 || !strings.Contains(findings, "yoke:plugin.01") || !strings.Contains(findings, "L2") {
		t.Errorf("L2 written by hand gave %d: %s", status, findings)
	}
	atL1 := bytes.Replace(rendering, []byte("| **Level** | L2 |"), []byte("| **Level** | L1 |"), 1)
	if status, findings := check(t, "plugin.std.md", atL1); status == 0 || !strings.Contains(findings, "yoke:plugin.01") || !strings.Contains(findings, "L1") {
		t.Errorf("a rendering at L1 gave %d: %s", status, findings)
	}
}

// std: yoke:the-rendered-description.04
func TestYokeConformancePrintsAContractsRendering(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-conformance")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-conformance").CombinedOutput(); err != nil {
		t.Fatalf("yoke-conformance does not build: %v\n%s", err, said)
	}
	out, err := exec.Command(binary, "--render", "plugin").Output()
	if err != nil {
		t.Fatalf("--render plugin failed: %v", err)
	}
	committed, _ := os.ReadFile("plugin.std.md")
	if !bytes.Equal(out, committed) {
		t.Errorf("it printed\n%s\nand the committed rendering is\n%s", out, committed)
	}
}
