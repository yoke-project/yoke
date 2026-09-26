package supervisor_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// std: yoke:the-launch-order.07
func TestTheCoreLaunchesInTheOrderItDerives(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte(`units:
  second: { kind: oneshot, exec: /bin/sh, args: [ -c, "echo second-line" ], depends_on: [ first ] }
  first: { kind: oneshot, exec: /bin/sh, args: [ -c, "sleep 0.3; echo first-line" ] }
  provision: { kind: oneshot, exec: /bin/sh, args: [ -c, "exit 1" ] }
  store: { kind: oneshot, exec: /bin/sh, args: [ -c, "true" ], depends_on: [ provision ], policy: { startup_window: 1s } }
  archive: { kind: oneshot, exec: /bin/sh, args: [ -c, "true" ], depends_on: [ store ], policy: { startup_window: 1s } }
`), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\n", dir, run)), 0o644)

	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+composition)
	out, _ := command.StderrPipe()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	var said []string
	deadline := time.After(5 * time.Second)
reading:
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
		case <-deadline:
			break reading
		}
	}
	index := func(holds func(string) bool) int { return slices.IndexFunc(said, holds) }
	first := index(func(l string) bool { return strings.Contains(l, "msg=first-line") })
	second := index(func(l string) bool { return strings.Contains(l, "msg=second-line") })
	if first < 0 || second < 0 || second < first {
		t.Errorf("first's line at %d, second's at %d:\n%s", first, second, strings.Join(said, "\n"))
	}
	for id, cause := range map[string]string{
		"archive": "archive did not start because store did not start because provision exited 1",
		"store":   "store did not start because provision exited 1",
	} {
		if index(func(l string) bool {
			return strings.Contains(l, `msg="not started"`) && strings.Contains(l, "unit="+id+" ") && strings.Contains(l, `cause="`+cause+`"`)
		}) < 0 {
			t.Errorf("the Core did not report %s not started with %q:\n%s", id, cause, strings.Join(said, "\n"))
		}
	}
	if command.ProcessState != nil {
		t.Errorf("the Core is no longer running")
	}
}
