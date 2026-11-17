package supervisor_test

import (
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// handedAny is every variable a unit was handed, at any incarnation.
func handedAny(out *output, id string) []string {
	var env []string
	for _, line := range out.all() {
		if rest, ok := strings.CutPrefix(line, id+"#"); ok {
			if _, v, ok := strings.Cut(rest, " env "); ok {
				env = append(env, v)
			}
		}
	}
	return env
}

// ended says whether the unit has completed or failed.
func ended(s *supervisor.Supervisor, id string) func() bool {
	return func() bool {
		st := s.Status(id).State
		return st == unit.Completed || st == unit.Failed
	}
}

// listen makes a Unix socket at the path, as a compositor does, for as long as the test runs.
func listen(t *testing.T, path string) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
}

// shortDir is a temporary directory whose path a socket's address can hold.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "yk")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// std: yoke:the-graphical-session.01
func TestASocketThatAppearsAfterTheCoreStartedIsFoundWhetherOrNotItsEnvironmentNamedIt(t *testing.T) {
	panel := declared(t, "panel", unit.Oneshot, "environment")
	panel.Needs = []supervisor.Need{{Class: "display"}}
	speaker := declared(t, "speaker", unit.Oneshot, "environment")
	speaker.Needs = []supervisor.Need{{Class: "audio"}}

	// A Core started before the compositor: a runtime directory, and no display named.
	before := newHost(t)
	runtime := shortDir(t)
	before.env["XDG_RUNTIME_DIR"] = runtime
	s, out := underHost(t, before, nil)
	// A Core whose environment named the display before its socket existed.
	named := newHost(t)
	namedRuntime := shortDir(t)
	named.env["XDG_RUNTIME_DIR"] = namedRuntime
	named.env["WAYLAND_DISPLAY"] = "wayland-1"
	n, nout := underHost(t, named, nil)

	s.Launch(panel)
	s.Launch(speaker)
	n.Launch(panel)
	for _, w := range []struct {
		s    *supervisor.Supervisor
		id   string
		need string
	}{{s, "panel", "display"}, {s, "speaker", "audio"}, {n, "panel", "display"}} {
		until(t, w.id+" to fail with nothing on the host", 3*time.Second, failed(w.s, w.id))
		if f := w.s.Status(w.id).Failure; !strings.Contains(f, w.need) {
			t.Errorf("%s's failure is %q, naming no %s", w.id, f, w.need)
		}
	}

	// The compositor and the session's audio start: their sockets appear under each runtime directory.
	for _, dir := range []string{runtime, namedRuntime} {
		listen(t, filepath.Join(dir, "wayland-1"))
		touch(t, filepath.Join(dir, "pipewire-0"))
	}
	s.Launch(panel)
	s.Launch(speaker)
	n.Launch(panel)
	until(t, "the panels and the speaker to end again", 3*time.Second, func() bool {
		return ended(s, "panel")() && ended(s, "speaker")() && ended(n, "panel")()
	})
	// Give each a moment to reach its end rather than the state of its first launch.
	time.Sleep(200 * time.Millisecond)

	if st := s.Status("panel"); st.State != unit.Completed {
		t.Errorf("under the Core that named no display, the panel is %v (%q) once the socket exists", st.State, st.Failure)
	}
	if env := handedAny(out, "panel"); !slices.Contains(env, "WAYLAND_DISPLAY="+filepath.Join(runtime, "wayland-1")) {
		t.Errorf("under the Core that named no display, the panel was handed %v", env)
	}
	if st := s.Status("speaker"); st.State != unit.Completed {
		t.Errorf("under the same Core the speaker is %v (%q) once PipeWire's socket exists", st.State, st.Failure)
	}
	if env := handedAny(out, "speaker"); !slices.Contains(env, "PIPEWIRE_REMOTE="+filepath.Join(runtime, "pipewire-0")) {
		t.Errorf("the speaker was handed %v", env)
	}
	if st := n.Status("panel"); st.State != unit.Completed {
		t.Errorf("under the Core that named the display, the panel is %v (%q) once the socket exists", st.State, st.Failure)
	}
	if env := handedAny(nout, "panel"); !slices.Contains(env, "WAYLAND_DISPLAY="+filepath.Join(namedRuntime, "wayland-1")) {
		t.Errorf("the panel was handed %v", env)
	}
}

// std: yoke:the-graphical-session.03
func TestOfSeveralWaylandSocketsNoneIsChosenUnlessTheEnvironmentNamesOne(t *testing.T) {
	panel := declared(t, "panel", unit.Oneshot, "environment")
	panel.Needs = []supervisor.Need{{Class: "display"}}

	runtime := shortDir(t)
	listen(t, filepath.Join(runtime, "wayland-0"))
	listen(t, filepath.Join(runtime, "wayland-1"))
	// Beside them, what is not a compositor's socket: a lock file, and a file that is no socket.
	touch(t, filepath.Join(runtime, "wayland-1.lock"))
	touch(t, filepath.Join(runtime, "wayland-2"))

	unnamed := newHost(t)
	unnamed.env["XDG_RUNTIME_DIR"] = runtime
	u, _ := underHost(t, unnamed, nil)
	named := newHost(t)
	named.env["XDG_RUNTIME_DIR"] = runtime
	named.env["WAYLAND_DISPLAY"] = "wayland-1"
	n, nout := underHost(t, named, nil)

	u.Launch(panel)
	n.Launch(panel)
	until(t, "both panels to end", 3*time.Second, func() bool { return ended(u, "panel")() && ended(n, "panel")() })
	time.Sleep(200 * time.Millisecond)

	st := u.Status("panel")
	if st.State != unit.Failed || !strings.Contains(st.Failure, "wayland-0, wayland-1") || strings.Contains(st.Failure, "wayland-2") || strings.Contains(st.Failure, ".lock") {
		t.Errorf("with two sockets and none named, the panel is %v (%q)", st.State, st.Failure)
	}
	if st := n.Status("panel"); st.State != unit.Completed {
		t.Errorf("with the environment naming one of two, the panel is %v (%q)", st.State, st.Failure)
	}
	if env := handedAny(nout, "panel"); !slices.Contains(env, "WAYLAND_DISPLAY="+filepath.Join(runtime, "wayland-1")) {
		t.Errorf("with the environment naming one of two, the panel was handed %v", env)
	}
}
