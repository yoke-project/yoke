package supervisor_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// reader reads a Core's lines until one satisfies holds, keeping every line it read.
type reader struct {
	t     *testing.T
	lines <-chan string
	said  []string
}

func (r *reader) until(what string, within time.Duration, holds func(string) bool) string {
	r.t.Helper()
	deadline := time.After(within)
	for {
		select {
		case line, open := <-r.lines:
			if !open {
				r.t.Fatalf("the Core exited waiting for %s:\n%s", what, strings.Join(r.said, "\n"))
			}
			r.said = append(r.said, line)
			if holds(line) {
				return line
			}
		case <-deadline:
			r.t.Fatalf("%s did not happen within %v:\n%s", what, within, strings.Join(r.said, "\n"))
		}
	}
}

// rest reads every line left, until the Core closes its output.
func (r *reader) rest() []string {
	for line := range r.lines {
		r.said = append(r.said, line)
	}
	return r.said
}

// ended reads every line left, and fails unless the Core closes its output within the time given.
func (r *reader) ended(within time.Duration) []string {
	r.t.Helper()
	deadline := time.After(within)
	for {
		select {
		case line, open := <-r.lines:
			if !open {
				return r.said
			}
			r.said = append(r.said, line)
		case <-deadline:
			r.t.Fatalf("the Core was still running after %v:\n%s", within, strings.Join(r.said, "\n"))
		}
	}
}

func containing(parts ...string) func(string) bool {
	return func(line string) bool {
		for _, p := range parts {
			if !strings.Contains(line, p) {
				return false
			}
		}
		return true
	}
}

// std: yoke:the-inherited-containers.03
func TestACoreAfterOneThatDiedRemovesWhatItLeft(t *testing.T) {
	image := fixtureImage(t, "serve")
	socket := podmanService(t).socket
	d := deployed(t, socket, "units:\n  keeper:\n    kind: oneshot\n    image: "+image+"\n")
	// A case that fails leaves no Core to stop what it launched, and what is launched here runs for an hour.
	t.Cleanup(func() {
		exec.Command("podman", "rm", "-f", "-t", "0", "--filter", "label=dev.yoke-project.unit=keeper").Run()
	})

	first, lines := d.start(t)
	r := &reader{t: t, lines: lines}
	r.until("the first life to run", time.Minute, containing("type=unit.state.changed subject=unit:keeper#1", "to=Running"))
	instance := instanceOf(r.said)
	first.Process.Kill()
	r.rest()
	first.Wait()
	left, _ := exec.Command("podman", "ps", "-q", "--no-trunc", "--filter", "label=dev.yoke-project.unit=keeper",
		"--filter", "label=dev.yoke-project.instance="+instance).Output()
	inherited := strings.TrimSpace(string(left))
	if inherited == "" || strings.Contains(inherited, "\n") {
		t.Fatalf("the Core that died left %q running, want one container", inherited)
	}

	elsewhere := "elsewhere-" + strconv.Itoa(os.Getpid())
	theirs, err := exec.Command("podman", "run", "-d", "--network", "none", "--label", "dev.yoke-project.instance="+elsewhere,
		"--label", "dev.yoke-project.unit=keeper", image).Output()
	if err != nil {
		t.Fatalf("another instance's container does not run: %v", err)
	}
	other := strings.TrimSpace(string(theirs))
	t.Cleanup(func() { exec.Command("podman", "rm", "-f", "-t", "0", other).Run() })

	second, lines := d.start(t)
	r = &reader{t: t, lines: lines}
	named := r.until("the inherited container to be named", time.Minute, containing(inherited))
	for _, want := range []string{"unit=keeper", "incarnation=1", "running=true"} {
		if !strings.Contains(named, want) {
			t.Errorf("the line naming it does not say %s: %s", want, named)
		}
	}
	r.until("step 7 to be done", 10*time.Second, containing(`step="inherited runtime facts"`))
	r.until("the second life to launch", time.Minute, containing("subject=unit:keeper#2", "type=unit.state.changed"))
	if err := exec.Command("podman", "container", "exists", inherited).Run(); err == nil {
		t.Errorf("the inherited container %s still exists", inherited)
	}
	if running, _ := exec.Command("podman", "inspect", "-f", "{{.State.Running}}", other).Output(); strings.TrimSpace(string(running)) != "true" {
		t.Errorf("another instance's container is not running: %q", running)
	}
	second.Process.Signal(syscall.SIGTERM)
	r.rest()
	second.Wait()
}

// std: yoke:the-inherited-containers.04
func TestAnEngineNotReachedWhereAUnitNamesAnImageIsFatal(t *testing.T) {
	nothing := filepath.Join(t.TempDir(), "engine.sock")
	image := "localhost/yoke-l3-absent@sha256:" + strings.Repeat("ab", 32)
	core, lines := deployed(t, nothing, "units:\n  announcer:\n    kind: oneshot\n    image: "+image+"\n").start(t)
	r := &reader{t: t, lines: lines}
	said := strings.Join(r.ended(30*time.Second), "\n")
	err := core.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Errorf("the Core ended with %v, want a failure", err)
	}
	if !strings.Contains(said, "step inherited runtime facts") || !strings.Contains(said, nothing) {
		t.Errorf("the failure does not name step 7 and the engine's address:\n%s", said)
	}
	for _, never := range []string{"type=instance.ready", "subject=unit:announcer"} {
		if strings.Contains(said, never) {
			t.Errorf("the Core went as far as %s:\n%s", never, said)
		}
	}
}

// std: yoke:the-inherited-containers.05
func TestWhereNoUnitNamesAnImageNoEngineIsLookedFor(t *testing.T) {
	nothing := filepath.Join(t.TempDir(), "engine.sock")
	core, lines := deployed(t, nothing, "units:\n  local:\n    kind: oneshot\n    exec: /bin/sh\n    args: [ -c, \"true\" ]\n").start(t)
	r := &reader{t: t, lines: lines}
	r.until("step 7 to be done", 10*time.Second, containing(`step="inherited runtime facts"`))
	r.until("the instance to be ready", 10*time.Second, containing("type=instance.ready"))
	r.until("the unit to complete", 10*time.Second, containing("subject=unit:local#1", "to=Completed"))
	core.Process.Signal(syscall.SIGTERM)
	for _, line := range r.rest() {
		if strings.Contains(line, "engine") {
			t.Errorf("something was said about the container engine: %s", line)
		}
	}
	core.Wait()
}
