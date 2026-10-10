package supervisor_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
)

// std: yoke:the-container-backend.06
func TestTheCoreRunsAOneshotInAContainer(t *testing.T) {
	image := fixtureImage(t, "announce")
	socket := enginetest.Serve(t).Socket
	core, lines := coreWith(t, socket, "units:\n  announcer:\n    kind: oneshot\n    image: "+image+"\n")

	want := fmt.Sprintf("announced as uid=%d gid=%d", os.Getuid(), os.Getgid())
	announced, completed := false, false
	var said []string
	deadline := time.After(time.Minute)
	for !announced || !completed {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			if strings.Contains(line, want) && strings.Contains(line, "unit=announcer incarnation=1 stream=stdout") {
				announced = true
			}
			if strings.Contains(line, "type=unit.state.changed subject=unit:announcer#1") && strings.Contains(line, "to=Completed") {
				completed = true
			}
		case <-deadline:
			t.Fatalf("announced=%v completed=%v within a minute:\n%s", announced, completed, strings.Join(said, "\n"))
		}
	}
	core.Process.Signal(syscall.SIGTERM)
	for range lines {
	}
	core.Wait()
	if left := enginetest.Labelled("dev.yoke-project.unit=announcer", "dev.yoke-project.instance="+instanceOf(said)); len(left) != 0 {
		t.Errorf("containers are left under the instance's label: %s", left)
	}
}

// coreWith starts the Core with the engine at socket and a composition of units, and returns it and the
// lines it writes, one at a time.
func coreWith(t *testing.T, socket, units string) (*exec.Cmd, <-chan string) {
	t.Helper()
	return deployed(t, socket, units).start(t)
}

// deployment is a built Core and the configuration it is started with, which a case can start again.
type deployment struct{ binary, config, composition string }

// deployed builds the Core and writes its configuration, with the engine at socket and a composition of
// units.
func deployed(t *testing.T, socket, units string) deployment {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	os.MkdirAll(filepath.Join(dir, "plugins.d"), 0o755)
	os.MkdirAll(filepath.Join(dir, "plugins"), 0o755)
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte(units), 0o644)
	config := filepath.Join(dir, "core.yaml")
	os.WriteFile(config, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nengine: unix://%s\nplugins:\n  manifests: %s/plugins.d\n  executables: %s/plugins\n",
		dir, run, socket, dir, dir)), 0o644)
	return deployment{binary, config, composition}
}

// start starts the Core, and returns it and the lines it writes, one at a time.
func (d deployment) start(t *testing.T) (*exec.Cmd, <-chan string) {
	t.Helper()
	core := exec.Command(d.binary)
	core.Env = append(os.Environ(), "YOKE_CONFIG="+d.config, "YOKE_COMPOSITION="+d.composition)
	out, _ := core.StderrPipe()
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { core.Process.Kill(); core.Wait() })
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	return core, lines
}

// std: yoke:the-quiet-engine.07
func TestAContainerThatEndedWhileTheEngineWasAwayIsConcludedOnItsReturn(t *testing.T) {
	enginetest.Not(t, enginetest.Docker)
	image := fixtureImage(t, "linger")
	engine := enginetest.Serve(t)
	core, lines := coreWith(t, engine.Socket, "units:\n  lingerer:\n    kind: oneshot\n    image: "+image+"\n")
	var said []string
	// next reads the Core's lines until one satisfies holds, failing on anything forbidden on the way.
	next := func(what string, within time.Duration, holds func(string) bool) {
		t.Helper()
		deadline := time.After(within)
		for {
			select {
			case line, open := <-lines:
				if !open {
					t.Fatalf("the Core exited waiting for %s:\n%s", what, strings.Join(said, "\n"))
				}
				said = append(said, line)
				if strings.Contains(line, "subject=unit:lingerer#2") {
					t.Fatalf("a second incarnation was launched:\n%s", strings.Join(said, "\n"))
				}
				if holds(line) {
					return
				}
			case <-deadline:
				t.Fatalf("%s did not happen within %v:\n%s", what, within, strings.Join(said, "\n"))
			}
		}
	}
	about := func(typ string, also ...string) func(string) bool {
		return func(line string) bool {
			if !strings.Contains(line, "type="+typ+" subject=unit:lingerer#1") {
				return false
			}
			for _, a := range also {
				if !strings.Contains(line, a) {
					return false
				}
			}
			return true
		}
	}
	next("the unit to run", time.Minute, about("unit.state.changed", "to=Running"))

	engine.Kill()
	next("the condition", 10*time.Second, about("unit.condition.changed"))
	instance := instanceOf(said)
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		state := enginetest.State("dev.yoke-project.unit=lingerer", "dev.yoke-project.instance="+instance)
		if state == "exited" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the container never ended while the engine was away: %q", state)
		}
	}
	for _, line := range said {
		if strings.Contains(line, "to=Completed") {
			t.Fatalf("the unit was concluded while the engine was away:\n%s", strings.Join(said, "\n"))
		}
	}

	engine.Start()
	next("the condition cleared", 30*time.Second, about("unit.condition.changed"))
	next("the unit to complete", 30*time.Second, about("unit.state.changed", "to=Completed"))
	core.Process.Signal(syscall.SIGTERM)
	for line := range lines {
		said = append(said, line)
	}
	core.Wait()
	if left := enginetest.Labelled("dev.yoke-project.instance=" + instance); len(left) != 0 {
		t.Errorf("containers are left under the instance's label: %s", left)
	}
}

// instanceOf is the instance's identity, as the Core's ready event names it.
func instanceOf(said []string) string {
	for _, line := range said {
		if _, after, ok := strings.Cut(line, "subject=instance:"); ok {
			name, _, _ := strings.Cut(after, " ")
			return name
		}
	}
	return "yoke"
}

// fixtureImage builds, over scratch, a static copy of this test binary playing part, and returns it
// referenced by its digest, held by the engine under test.
func fixtureImage(t *testing.T, part string) string {
	t.Helper()
	dir := t.TempDir()
	build := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "unit"), ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the fixture does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY unit /unit\nENV "+role+"="+part+"\nENTRYPOINT [\"/unit\"]\n"), 0o644)
	return enginetest.Image(t, dir, "yoke-l3-"+part)
}
