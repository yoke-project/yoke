package logstore_test

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/logstore"
)

// std: yoke:the-log-store.07
func TestThroughTheCoreOutputAndLifeAreRecorded(t *testing.T) {
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
	os.WriteFile(composition, []byte("units:\n  hello: { kind: oneshot, exec: /bin/sh, args: [ -c, 'echo to-stdout; echo to-stderr >&2' ] }\n"), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s/plugins.d\n  executables: %s/plugins\n", dir, run, dir, dir)), 0o644)

	// Each start runs until the unit completed, and is then stopped.
	for life := 1; life <= 2; life++ {
		command := exec.Command(binary)
		command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+composition)
		out, _ := command.StderrPipe()
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(out)
		completed := make(chan bool)
		go func() {
			done := false
			for scanner.Scan() {
				if !done && strings.Contains(scanner.Text(), "type=unit.state.changed") && strings.Contains(scanner.Text(), fmt.Sprintf("subject=unit:hello#%d", life)) &&
					strings.Contains(scanner.Text(), "to=Completed") {
					done = true
					completed <- true
				}
			}
			close(completed)
		}()
		select {
		case ok := <-completed:
			if !ok {
				t.Fatalf("the Core exited before the unit's life %d completed", life)
			}
		case <-time.After(20 * time.Second):
			command.Process.Kill()
			t.Fatalf("life %d did not complete within twenty seconds", life)
		}
		command.Process.Signal(syscall.SIGTERM)
		command.Wait()
	}

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "state", logstore.File)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	has := func(query string, args ...any) bool {
		var n int
		if err := db.QueryRow(query, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return n > 0
	}
	for life := 1; life <= 2; life++ {
		if !has("SELECT count(*) FROM entry WHERE unit = 'hello' AND incarnation = ? AND source = 'stdout' AND message = 'to-stdout'", life) ||
			!has("SELECT count(*) FROM entry WHERE unit = 'hello' AND incarnation = ? AND source = 'stderr' AND message = 'to-stderr'", life) {
			t.Errorf("the two lines of life %d were not recorded", life)
		}
	}
	if !has("SELECT count(*) FROM entry WHERE unit = 'hello' AND incarnation = 1 AND source = 'core' AND type = 'unit.state.changed'") {
		t.Error("the first life's state changes were not recorded")
	}
}
