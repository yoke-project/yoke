package supervisor_test

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/gate"
)

// appends is a program appending its first argument, on a line of its own, to the file OUT names.
const appends = "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"$OUT\"\n"

// std: yoke:the-migration-step.03
func TestACoreOnABundleRunsTheMigrationOnceAcrossTheVersionItRewrites(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	bundle := filepath.Join(t.TempDir(), "bundle")
	os.MkdirAll(filepath.Join(bundle, "components"), 0o755)
	os.WriteFile(filepath.Join(bundle, "components", "append"), []byte(appends), 0o755)
	out := filepath.Join(t.TempDir(), "said")
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(appends)))
	os.WriteFile(filepath.Join(bundle, "yoke.yaml"), []byte(`model: 1
id: com.yoke.station
version: 2.2.0
arch: `+gate.HostArch()+`
core: ">=0.0.0-0"
data: ">=2.0"
units:
  migrate: { kind: oneshot, exec: components/append, digest: "`+digest+`", args: [ migrate ], env: { OUT: "`+out+`" }, migrates_from: "<2.2" }
  store: { kind: oneshot, exec: components/append, digest: "`+digest+`", args: [ store ], env: { OUT: "`+out+`" }, depends_on: [ migrate ] }
`), 0o644)
	run, err := os.MkdirTemp("", "ym")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	stateHome := t.TempDir()
	state := filepath.Join(stateHome, "yoke", "instances", "bench-a", "state")
	os.MkdirAll(state, 0o700)
	os.WriteFile(filepath.Join(state, "version"), []byte("2.1.0\n"), 0o600)

	// once starts the Core on the bundle until store completes, then stops it, and returns what it said.
	once := func() string {
		t.Helper()
		cmd := exec.Command(binary, "--instance", "bench-a", "--bundle", bundle)
		cmd.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+run, "XDG_STATE_HOME="+stateHome)
		stderr, _ := cmd.StderrPipe()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		lines := make(chan string)
		go func() {
			s := bufio.NewScanner(stderr)
			for s.Scan() {
				lines <- s.Text()
			}
			close(lines)
		}()
		var said []string
		deadline := time.After(30 * time.Second)
	reading:
		for {
			select {
			case line, open := <-lines:
				if !open {
					t.Fatalf("the Core ended before store completed:\n%s", strings.Join(said, "\n"))
				}
				said = append(said, line)
				if strings.Contains(line, "subject=unit:store#") && strings.Contains(line, "to=Completed") {
					break reading
				}
			case <-deadline:
				cmd.Process.Kill()
				t.Fatalf("store did not complete:\n%s", strings.Join(said, "\n"))
			}
		}
		cmd.Process.Signal(syscall.SIGTERM)
		for line := range lines {
			said = append(said, line)
		}
		cmd.Wait()
		return strings.Join(said, "\n")
	}
	skipped := func(said, version string) bool {
		for _, line := range strings.Split(said, "\n") {
			if strings.Contains(line, "subject=unit:migrate ") && strings.Contains(line, "to=Completed") &&
				strings.Contains(line, "skipped=") && strings.Contains(line, "version:"+version) {
				return true
			}
		}
		return false
	}

	first := once()
	if !strings.Contains(first, "subject=unit:migrate#1") || skipped(first, "2.1.0") {
		t.Errorf("the first start did not run migrate:\n%s", first)
	}
	if stamp, _ := os.ReadFile(filepath.Join(state, "version")); strings.TrimSpace(string(stamp)) != "2.2.0" {
		t.Errorf("the stamp after the first start is %q", stamp)
	}
	if second := once(); !skipped(second, "2.2.0") {
		t.Errorf("the second start did not skip migrate:\n%s", second)
	}
	os.RemoveAll(state)
	if third := once(); !skipped(third, "none") {
		t.Errorf("a start on cleared state did not skip migrate:\n%s", third)
	}
	if said, _ := os.ReadFile(out); string(said) != "migrate\nstore\nstore\nstore\n" {
		t.Errorf("the units wrote %q", said)
	}
}
