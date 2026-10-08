package logstore_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/logstore"
)

// std: yoke:the-cleaner.07
func TestTheCoreSchedulesItsCleaner(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	for _, d := range []string{"plugins.d", "plugins"} {
		os.MkdirAll(filepath.Join(dir, d), 0o755)
	}
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte("units: {}\n"), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s/plugins.d\n  executables: %s/plugins\n", dir, run, dir, dir)), 0o644)
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+composition)
	out, _ := command.StderrPipe()
	began := time.Now()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })

	var said []string
	var first time.Time
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		line := scanner.Text()
		said = append(said, line)
		if _, after, ok := strings.Cut(line, "msg=retention first="); ok {
			stamp, _, _ := strings.Cut(after, " ")
			first, _ = time.Parse(time.RFC3339Nano, stamp)
		}
		if strings.Contains(line, "msg=ready") {
			break
		}
	}
	want := began.Add(time.Hour + logstore.Offset(instanceOf(said), time.Hour))
	if first.IsZero() || first.Sub(want).Abs() > 5*time.Second {
		t.Errorf("the first cycle is at %v, want about %v:\n%s", first, want, strings.Join(said, "\n"))
	}
}

// instanceOf is the instance's name, as the Core's root names it in the service form.
func instanceOf(said []string) string {
	for _, line := range said {
		if _, after, ok := strings.Cut(line, "subject=instance:"); ok {
			name, _, _ := strings.Cut(after, " ")
			return name
		}
	}
	return "yoke"
}
