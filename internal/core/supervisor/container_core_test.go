package supervisor_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
)

// std: yoke:the-container-backend.06
func TestTheCoreRunsAOneshotInAContainer(t *testing.T) {
	image := fixtureImage(t, "announce")
	socket := podmanService(t).socket
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
	left, _ := exec.Command("podman", "ps", "-aq", "--filter", "label=dev.yoke-project.unit=announcer",
		"--filter", "label=dev.yoke-project.instance="+instanceOf(said)).Output()
	if len(strings.TrimSpace(string(left))) != 0 {
		t.Errorf("containers are left under the instance's label: %s", left)
	}
}

// coreWith starts the Core with the engine at socket and a composition of units, and returns it and the
// lines it writes, one at a time.
func coreWith(t *testing.T, socket, units string) (*exec.Cmd, <-chan string) {
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
	core := exec.Command(binary)
	core.Env = append(os.Environ(), "YOKE_CONFIG="+config, "YOKE_COMPOSITION="+composition)
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
	image := fixtureImage(t, "linger")
	engine := podmanService(t)
	core, lines := coreWith(t, engine.socket, "units:\n  lingerer:\n    kind: oneshot\n    image: "+image+"\n")
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

	engine.kill()
	next("the condition", 10*time.Second, about("unit.condition.changed"))
	instance := instanceOf(said)
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		state, _ := exec.Command("podman", "ps", "-a", "--format", "{{.State}}", "--filter", "label=dev.yoke-project.unit=lingerer",
			"--filter", "label=dev.yoke-project.instance="+instance).Output()
		if strings.TrimSpace(string(state)) == "exited" {
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

	engine.start()
	next("the condition cleared", 30*time.Second, about("unit.condition.changed"))
	next("the unit to complete", 30*time.Second, about("unit.state.changed", "to=Completed"))
	core.Process.Signal(syscall.SIGTERM)
	for line := range lines {
		said = append(said, line)
	}
	core.Wait()
	left, _ := exec.Command("podman", "ps", "-aq", "--filter", "label=dev.yoke-project.instance="+instance).Output()
	if len(strings.TrimSpace(string(left))) != 0 {
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
// referenced by its digest.
func fixtureImage(t *testing.T, part string) string {
	t.Helper()
	dir := t.TempDir()
	build := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "unit"), ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the fixture does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY unit /unit\nENV "+role+"="+part+"\nENTRYPOINT [\"/unit\"]\n"), 0o644)
	tag := "localhost/yoke-l3-" + part + ":" + strconv.Itoa(os.Getpid())
	if said, err := exec.Command("podman", "build", "-q", "-t", tag, dir).CombinedOutput(); err != nil {
		t.Fatalf("the fixture image does not build: %v\n%s", err, said)
	}
	t.Cleanup(func() { exec.Command("podman", "rmi", "-f", tag).Run() })
	digest, err := exec.Command("podman", "images", "--digests", "--format", "{{.Digest}}", tag).Output()
	if err != nil {
		t.Fatal(err)
	}
	return "localhost/yoke-l3-" + part + "@" + strings.TrimSpace(string(digest))
}

// service is rootless Podman's API, served on a socket of the test's, which a case can take away and
// bring back on the same path.
type service struct {
	t       *testing.T
	socket  string
	running *exec.Cmd
}

// podmanService starts rootless Podman's API on a socket of the test's.
func podmanService(t *testing.T) *service {
	t.Helper()
	dir, _ := os.MkdirTemp("", "eng-")
	t.Cleanup(func() { os.RemoveAll(dir) })
	s := &service{t: t, socket: filepath.Join(dir, "podman.sock")}
	s.start()
	t.Cleanup(s.kill)
	return s
}

// start serves the API on the socket, and returns once the API answers. A socket that exists is not yet
// an engine that answers: on a loaded runner Podman's first answer can take longer than the Core waits
// at its start, and a case about a container would then be about a cold engine.
func (s *service) start() {
	s.t.Helper()
	s.running = exec.Command("podman", "system", "service", "--time=0", "unix://"+s.socket)
	if err := s.running.Start(); err != nil {
		s.t.Fatalf("the environment declares rootless Podman, and it cannot be run: %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(s.socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			s.t.Fatal("Podman's API never appeared")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := engine.Reach(ctx, "unix://"+s.socket); err != nil {
		s.t.Fatalf("Podman's API never answered: %v", err)
	}
}

// kill takes the API away as a crash would, leaving what it ran running, and its socket's file behind it
// removed as a restart would remove it.
func (s *service) kill() {
	if s.running == nil {
		return
	}
	s.running.Process.Kill()
	s.running.Wait()
	s.running = nil
	os.Remove(s.socket)
}
