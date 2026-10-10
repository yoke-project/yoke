package supervisor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// expansion is what a launch's needs expand into: the variables handed, and what a container is given.
type expansion struct {
	env     []string
	devices []string
	mounts  []string
	groups  []int
	network bool
}

// expand expands a unit's needs for one launch, from the Core's environment and the host's filesystem as
// they are now. A need that cannot be expanded is an error naming it, and the launch fails on it.
func (s *Supervisor) expand(u Unit, contained bool) (expansion, error) {
	var x expansion
	for _, n := range u.Needs {
		var err error
		switch n.Class {
		case "device":
			err = x.device(n.Path)
		case "storage":
			if err = os.MkdirAll(n.Path, 0o700); err == nil {
				x.mounts = append(x.mounts, n.Path)
			} else {
				err = fmt.Errorf("the storage need's directory %s cannot be made: %w", n.Path, err)
			}
		case "display":
			err = x.display(s.getenv, s.x11(), contained)
		case "audio":
			err = x.audio(s.getenv)
		case "network":
			x.network = true
		}
		if err != nil {
			return expansion{}, err
		}
	}
	return x, nil
}

func (s *Supervisor) getenv(name string) string {
	if s.cfg.Getenv != nil {
		return s.cfg.Getenv(name)
	}
	return os.Getenv(name)
}

func (s *Supervisor) x11() string {
	if s.cfg.X11 != "" {
		return s.cfg.X11
	}
	return "/tmp/.X11-unix"
}

// device checks the bound path and takes the group that owns it.
func (x *expansion) device(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("the device bound to %s does not exist on this host", path)
	}
	x.devices = append(x.devices, path)
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		x.groups = append(x.groups, int(st.Gid))
	}
	return nil
}

// display finds Wayland's socket and X11's in the Core's environment; either is enough, and neither is a
// fault. On the host the runtime directory is handed beside them; in a container there is none.
func (x *expansion) display(getenv func(string) string, x11 string, contained bool) error {
	found := false
	if name := getenv("WAYLAND_DISPLAY"); name != "" {
		socket := name
		if !filepath.IsAbs(socket) {
			socket = filepath.Join(getenv("XDG_RUNTIME_DIR"), name)
		}
		if exists(socket) {
			found = true
			x.env = append(x.env, "WAYLAND_DISPLAY="+socket)
			x.mounts = append(x.mounts, socket)
			if runtime := getenv("XDG_RUNTIME_DIR"); runtime != "" && !contained {
				x.env = append(x.env, "XDG_RUNTIME_DIR="+runtime)
			}
		}
	}
	if display := getenv("DISPLAY"); strings.HasPrefix(display, ":") {
		number, _, _ := strings.Cut(display[1:], ".")
		if socket := filepath.Join(x11, "X"+number); number != "" && exists(socket) {
			found = true
			x.env = append(x.env, "DISPLAY="+display)
			x.mounts = append(x.mounts, socket)
			if authority := getenv("XAUTHORITY"); authority != "" && exists(authority) {
				x.env = append(x.env, "XAUTHORITY="+authority)
				x.mounts = append(x.mounts, authority)
			}
		}
	}
	if !found {
		return errors.New("the unit needs a display, and the Core's environment names none whose socket exists")
	}
	return nil
}

// audio finds PipeWire's socket and PulseAudio's; either is enough, and neither is a fault.
func (x *expansion) audio(getenv func(string) string) error {
	runtime := getenv("XDG_RUNTIME_DIR")
	found := false
	pipewire := getenv("PIPEWIRE_REMOTE")
	if !filepath.IsAbs(pipewire) && runtime != "" {
		pipewire = filepath.Join(runtime, "pipewire-0")
	}
	if filepath.IsAbs(pipewire) && exists(pipewire) {
		found = true
		x.env = append(x.env, "PIPEWIRE_REMOTE="+pipewire)
		x.mounts = append(x.mounts, pipewire)
	}
	pulse, unix := strings.CutPrefix(getenv("PULSE_SERVER"), "unix:")
	if !unix && runtime != "" {
		pulse = filepath.Join(runtime, "pulse", "native")
	}
	if filepath.IsAbs(pulse) && exists(pulse) {
		found = true
		x.env = append(x.env, "PULSE_SERVER=unix:"+pulse)
		x.mounts = append(x.mounts, pulse)
	}
	if !found {
		return errors.New("the unit needs audio, and neither PipeWire's socket nor PulseAudio's exists")
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
