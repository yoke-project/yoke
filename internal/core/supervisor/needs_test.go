package supervisor_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/discovery"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// host is a Core's environment and the host's X11 directory, as a launch reads them.
type host struct {
	env map[string]string
	x11 string
}

func newHost(t *testing.T) *host { return &host{env: map[string]string{}, x11: t.TempDir()} }

// touch makes the file at path, and its directory.
func touch(t *testing.T, path string) string {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// underHost starts a supervisor reading h, launching containers on c where c is not nil.
func underHost(t *testing.T, h *host, c *containers) (*supervisor.Supervisor, *output) {
	t.Helper()
	out := &output{}
	cfg := supervisor.Config{Root: t.TempDir(), Policy: fast(), Instance: "bench", Incarnations: supervisor.NewCounter(),
		Tokens: supervisor.NewTokens(), Output: out, Getenv: func(name string) string { return h.env[name] }, X11: h.x11}
	if c != nil {
		cfg.Containers = c
	}
	s := supervisor.New(cfg)
	t.Cleanup(func() { s.Stop() })
	return s, out
}

func failed(s *supervisor.Supervisor, id string) func() bool {
	return func() bool { return s.Status(id).State == unit.Failed }
}

func handed(out *output, id string) []string {
	var env []string
	for _, line := range out.all() {
		if v, ok := strings.CutPrefix(line, id+"#1 env "); ok {
			env = append(env, v)
		}
	}
	return env
}

// std: yoke:what-a-need-expands-into.01
func TestADeviceThatDoesNotExistIsAFaultOfTheLaunch(t *testing.T) {
	s, _ := underHost(t, newHost(t), nil)
	present := declared(t, "present", unit.Oneshot, "exit-0")
	present.Needs = []supervisor.Need{{Class: "device", Path: touch(t, filepath.Join(t.TempDir(), "head-a"))}}
	absent := declared(t, "absent", unit.Oneshot, "exit-0")
	missing := filepath.Join(t.TempDir(), "head-b")
	absent.Needs = []supervisor.Need{{Class: "device", Path: missing}}
	s.Launch(present)
	s.Launch(absent)
	until(t, "the unit with its device to complete", 3*time.Second, func() bool { return s.Status("present").State == unit.Completed })
	until(t, "the unit without it to fail", 3*time.Second, failed(s, "absent"))
	if st := s.Status("absent"); !strings.Contains(st.Failure, "device") || !strings.Contains(st.Failure, missing) || st.PID != 0 {
		t.Errorf("the failure is %q, with process %d", st.Failure, st.PID)
	}
}

// std: yoke:what-a-need-expands-into.02
func TestAStorageNeedIsADirectoryTheCoreCreates(t *testing.T) {
	_, dep := gate.Check(gate.Input{Document: gate.Document{Path: "bench.yaml", Kind: gate.Composition, Bytes: []byte(
		"units:\n  keeper: { kind: oneshot, exec: /bin/true, needs: [ \"storage:datasets\" ], args: [ \"${bind.datasets}\" ] }\n")}})
	if dep == nil {
		t.Fatal("the composition is refused")
	}
	state := t.TempDir()
	units := discovery.Units(dep, func(string) (*gate.Manifest, bool) { return nil, false }, "", state)
	dir := filepath.Join(state, "storage", "datasets")
	if len(units) != 1 || !slices.Equal(units[0].Args, []string{dir}) || !slices.Equal(units[0].Needs, []supervisor.Need{{Class: "storage", Path: dir}}) {
		t.Fatalf("resolved as %+v", units)
	}
	s, out := underHost(t, newHost(t), nil)
	for _, id := range []string{"first", "second"} {
		u := declared(t, id, unit.Oneshot, "keep")
		u.Args, u.Needs = units[0].Args, units[0].Needs
		s.Launch(u)
		until(t, id+" to complete", 3*time.Second, func() bool { return s.Status(id).State == unit.Completed })
	}
	said := strings.Join(out.all(), "\n")
	for _, want := range []string{"first#1 mode 700 uid " + strconv.Itoa(os.Getuid()), "second#1 found what was kept"} {
		if !strings.Contains(said, want) {
			t.Errorf("the units said %q, not %q", said, want)
		}
	}
}

// std: yoke:what-a-need-expands-into.03
func TestADisplayIsTheHostsOwnFoundInTheCoresEnvironment(t *testing.T) {
	h := newHost(t)
	runtime := t.TempDir()
	touch(t, filepath.Join(runtime, "wayland-1"))
	touch(t, filepath.Join(h.x11, "X7"))
	authority := touch(t, filepath.Join(t.TempDir(), "Xauthority"))
	h.env = map[string]string{"XDG_RUNTIME_DIR": runtime, "WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":7", "XAUTHORITY": authority, "HOME": "/home/someone"}
	s, out := underHost(t, h, nil)
	panel := declared(t, "panel", unit.Oneshot, "environment")
	panel.Needs = []supervisor.Need{{Class: "display"}}
	s.Launch(panel)
	until(t, "the unit to complete", 3*time.Second, func() bool { return s.Status("panel").State == unit.Completed })
	env := handed(out, "panel")
	for _, want := range []string{"WAYLAND_DISPLAY=" + filepath.Join(runtime, "wayland-1"), "XDG_RUNTIME_DIR=" + runtime, "DISPLAY=:7", "XAUTHORITY=" + authority} {
		if !slices.Contains(env, want) {
			t.Errorf("the unit was handed %v, without %s", env, want)
		}
	}
	if slices.ContainsFunc(env, func(v string) bool { return strings.HasPrefix(v, "HOME=") }) {
		t.Errorf("the unit was handed the rest of the Core's environment: %v", env)
	}

	s, _ = underHost(t, newHost(t), nil)
	s.Launch(panel)
	until(t, "the unit with no display to fail", 3*time.Second, failed(s, "panel"))
	if st := s.Status("panel"); !strings.Contains(st.Failure, "display") {
		t.Errorf("the failure is %q", st.Failure)
	}
}

// std: yoke:what-a-need-expands-into.04
func TestAnAudioPathIsPipeWiresSocketOrPulseAudios(t *testing.T) {
	speaker := declared(t, "speaker", unit.Oneshot, "environment")
	speaker.Needs = []supervisor.Need{{Class: "audio"}}
	launch := func(sockets ...string) (*supervisor.Supervisor, []string, string) {
		h := newHost(t)
		runtime := t.TempDir()
		for _, socket := range sockets {
			touch(t, filepath.Join(runtime, socket))
		}
		h.env["XDG_RUNTIME_DIR"] = runtime
		s, out := underHost(t, h, nil)
		s.Launch(speaker)
		until(t, "the unit to end", 3*time.Second, func() bool {
			st := s.Status("speaker")
			return st.State == unit.Completed || st.State == unit.Failed
		})
		return s, handed(out, "speaker"), runtime
	}
	_, env, runtime := launch("pipewire-0", "pulse/native")
	for _, want := range []string{"PIPEWIRE_REMOTE=" + filepath.Join(runtime, "pipewire-0"), "PULSE_SERVER=unix:" + filepath.Join(runtime, "pulse", "native")} {
		if !slices.Contains(env, want) {
			t.Errorf("with both, the unit was handed %v, without %s", env, want)
		}
	}
	_, env, runtime = launch("pulse/native")
	if !slices.Contains(env, "PULSE_SERVER=unix:"+filepath.Join(runtime, "pulse", "native")) ||
		slices.ContainsFunc(env, func(v string) bool { return strings.HasPrefix(v, "PIPEWIRE_REMOTE=") }) {
		t.Errorf("with PulseAudio alone, the unit was handed %v", env)
	}
	s, _, _ := launch()
	if st := s.Status("speaker"); st.State != unit.Failed || !strings.Contains(st.Failure, "audio") {
		t.Errorf("with neither, the unit is %s: %q", st.State, st.Failure)
	}
}

// std: yoke:what-a-need-expands-into.05
func TestInAContainerEachNeedIsMountedOrMappedAtItsPath(t *testing.T) {
	h := newHost(t)
	runtime := t.TempDir()
	wayland := touch(t, filepath.Join(runtime, "wayland-1"))
	pipewire := touch(t, filepath.Join(runtime, "pipewire-0"))
	pulse := touch(t, filepath.Join(runtime, "pulse", "native"))
	x7 := touch(t, filepath.Join(h.x11, "X7"))
	authority := touch(t, filepath.Join(t.TempDir(), "Xauthority"))
	h.env = map[string]string{"XDG_RUNTIME_DIR": runtime, "WAYLAND_DISPLAY": "wayland-1", "DISPLAY": ":7", "XAUTHORITY": authority}
	device := touch(t, filepath.Join(t.TempDir(), "head-a"))
	info, _ := os.Stat(device)
	storage := filepath.Join(t.TempDir(), "storage", "cache")

	c := newContainers()
	s, _ := underHost(t, h, c)
	panel := imaged("panel", unit.Interface)
	panel.Needs = []supervisor.Need{{Class: "device", Path: device}, {Class: "storage", Path: storage}, {Class: "display"}, {Class: "audio"}, {Class: "network"}}
	s.Launch(panel)
	until(t, "the interface to be created", 3*time.Second, func() bool { return len(c.acts()) > 0 })
	s.Launch(imaged("bare", unit.Oneshot))
	until(t, "both to be created", 3*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.launched) == 2
	})
	c.mu.Lock()
	needing, bare, unmade := c.launched[0], c.launched[1], c.unmade
	c.mu.Unlock()
	if needing.Unit != "panel" {
		needing, bare = bare, needing
	}
	mounts := slices.Sorted(slices.Values(needing.Mounts))
	want := slices.Sorted(slices.Values([]string{storage, wayland, x7, authority, pipewire, pulse}))
	if !slices.Equal(needing.Devices, []string{device}) || !slices.Equal(needing.Groups, []int{int(info.Sys().(*syscall.Stat_t).Gid)}) ||
		!slices.Equal(mounts, want) || !needing.Network {
		t.Errorf("the interface was created with %+v, want the mounts %v", needing, want)
	}
	if !slices.Contains(needing.Env, "WAYLAND_DISPLAY="+wayland) || slices.ContainsFunc(needing.Env, func(v string) bool { return strings.HasPrefix(v, "XDG_RUNTIME_DIR=") }) {
		t.Errorf("the interface's environment is %v", needing.Env)
	}
	if len(unmade) != 0 {
		t.Errorf("mounted before it existed: %v", unmade)
	}
	if len(bare.Mounts)+len(bare.Devices)+len(bare.Groups) != 0 || bare.Network {
		t.Errorf("the oneshot needing nothing was created with %+v", bare)
	}
}

// std: yoke:what-a-need-expands-into.07
func TestADevicesGroupIsCarriedOnlyWhereTheAccountReachesItThroughIt(t *testing.T) {
	grouped := touch(t, filepath.Join(t.TempDir(), "grouped"))
	os.Chmod(grouped, 0o660)
	open := touch(t, filepath.Join(t.TempDir(), "open"))
	os.Chmod(open, 0o666)
	c := newContainers()
	s, _ := underHost(t, newHost(t), c)
	for id, path := range map[string]string{"grouped": grouped, "open": open, "null": "/dev/null"} {
		u := imaged(id, unit.Oneshot)
		u.Needs = []supervisor.Need{{Class: "device", Path: path}}
		s.Launch(u)
	}
	until(t, "the three to be created", 3*time.Second, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.launched) == 3
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, l := range c.launched {
		want := []int(nil)
		if l.Unit == "grouped" {
			want = []int{os.Getgid()}
		}
		if !slices.Equal(l.Groups, want) {
			t.Errorf("%s was created with the groups %v, want %v", l.Unit, l.Groups, want)
		}
	}
}
