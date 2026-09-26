// Package trunk is the Core's startup: the ordered chain from `exec` to serving, the rule that decides
// which failure is fatal, and the stop that undoes it.
//
// Each step establishes what the next depends on. A failure before readiness exits — a host holds a
// working instance or no instance — and a failure after it is reported, since there is then somewhere
// to report it. Cleanup belongs to the next start: nothing depends on an orderly stop having run.
package trunk

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/core/supervisor"
)

// Form is the deployment form, which decides where parameters come from and what the modes are.
type Form int

const (
	Service Form = iota
	Application
)

// RootMode is the instance root's mode: a dedicated group in the service form, the launching user's
// ownership in the application form.
func (f Form) RootMode() os.FileMode {
	if f == Service {
		return os.ModeSetgid | 0o750
	}
	return 0o700
}

// SocketMode is the mode of a socket the Core binds.
func (f Form) SocketMode() os.FileMode {
	if f == Service {
		return 0o660
	}
	return 0o600
}

// A Channel is bound together with its terminator, which serves what arrives on it.
type Channel struct {
	Name  string
	Path  string // relative to the instance root
	Serve func(Listener)
}

// State is what the steps establish, each for the ones after it.
type State struct {
	Form     Form
	Name     string // the application form's instance name; the service form's is a constant
	Env      func(string) string
	Stderr   io.Writer
	Channels []Channel
	Units    []supervisor.Unit // the units the deployment declares, launched at step 11

	Config config.Config
	Paths  instance.Paths
	Log    *slog.Logger

	stoppers []func() error
}

// OnStop registers what undoes a step. The stop runs them in the reverse of the order they were given.
func (st *State) OnStop(undo func() error) { st.stoppers = append(st.stoppers, undo) }

// Stop undoes what the trunk set up, in reverse.
func (st *State) Stop() error {
	var failed []error
	for i := len(st.stoppers) - 1; i >= 0; i-- {
		if err := st.stoppers[i](); err != nil {
			failed = append(failed, err)
		}
	}
	st.stoppers = nil
	return errors.Join(failed...)
}

// A Step is one link of the chain.
type Step struct {
	Name  string
	Start func(*State) error
}

// The step whose success makes the instance observable: failure before it is fatal.
const ready = "ready"

// Steps are the eleven, in their order. A step a later part of the Core owns does nothing yet.
func Steps() []Step {
	nothingYet := func(*State) error { return nil }
	return []Step{
		{"identity and paths", identity},
		{"parameters", parameters},
		{"logging", logging},
		{"claim", claim},
		{"debris", func(st *State) error { return ClearDebris(st.Paths.Root) }},
		{"stores", nothingYet},
		{"inherited runtime facts", nothingYet},
		{"declarations", nothingYet},
		{"channels", channels},
		{ready, func(st *State) error { st.Log.Info(ready, "root", st.Paths.Root); return nil }},
		{"units", units},
	}
}

// Run performs the steps in order. A failure before readiness is returned; one after it is reported.
// Declarations are the one qualified entry: in the service form a deployment is left after they fail.
func Run(st *State, steps []Step) error {
	readyAt := len(steps)
	for i, s := range steps {
		if s.Name == ready {
			readyAt = i
		}
	}
	for i, s := range steps {
		err := s.Start(st)
		if err == nil {
			if st.Log != nil {
				st.Log.Info("step", "step", s.Name)
			}
			continue
		}
		fatal := i < readyAt && !(s.Name == "declarations" && st.Form == Service)
		if fatal || st.Log == nil {
			return fmt.Errorf("step %s: %w", s.Name, err)
		}
		st.Log.Error("step failed", "step", s.Name, "error", err)
	}
	return nil
}

func identity(st *State) error {
	if st.Form == Service {
		// The service form's paths are its configured directories, read at the next step.
		st.Paths.Name = instance.ServiceName
		return nil
	}
	paths, err := instance.ApplicationPaths(st.Name, st.Env)
	if err != nil {
		return err
	}
	st.Paths = paths
	return nil
}

func parameters(st *State) error {
	if st.Form == Application {
		// The descriptor and the resolved values are read by the part of the Core that ingests them.
		st.Config = config.Defaults()
		return nil
	}
	c, err := config.Load(st.Env)
	if err != nil {
		return err
	}
	st.Config = c
	st.Paths = instance.ServicePaths(c.RuntimeDir, c.StateDir)
	return nil
}

func logging(st *State) error {
	levels := map[string]slog.Level{"debug": slog.LevelDebug, "info": slog.LevelInfo, "warn": slog.LevelWarn, "error": slog.LevelError}
	level, known := levels[st.Config.Log.Level]
	if !known {
		return fmt.Errorf("no such log level %q", st.Config.Log.Level)
	}
	st.Log = slog.New(slog.NewTextHandler(st.Stderr, &slog.HandlerOptions{Level: level}))
	return nil
}

func claim(st *State) error {
	held, err := instance.Claim(st.Paths.Root, st.Form.RootMode())
	if err != nil {
		return err
	}
	// Undone last: the sockets and the runtime directory go, then the claim is released.
	st.OnStop(func() error {
		removed := os.RemoveAll(st.Paths.Root)
		return errors.Join(removed, held.Release())
	})
	return nil
}

func channels(st *State) error {
	for _, ch := range st.Channels {
		path := filepath.Join(st.Paths.Root, ch.Path)
		if err := os.MkdirAll(filepath.Dir(path), st.Form.RootMode().Perm()); err != nil {
			return err
		}
		listener, err := Bind(path, st.Form)
		if err != nil {
			return fmt.Errorf("the channel %s: %w", ch.Name, err)
		}
		st.OnStop(listener.Close)
		go ch.Serve(listener)
	}
	return nil
}

// units hands the declared units to the supervisor, which is stopped first on the way down.
func units(st *State) error {
	s := supervisor.New(supervisor.Config{
		Root: st.Paths.Root, Policy: supervisor.DefaultPolicy(),
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: logged{st.Log},
	})
	st.OnStop(s.Stop)
	for _, u := range st.Units {
		s.Launch(u)
	}
	return nil
}

// logged writes a unit's output to the process logger, until the log store keeps it.
type logged struct{ log *slog.Logger }

func (l logged) Line(unitID string, incarnation int, line string) {
	l.log.Info(line, "unit", unitID, "incarnation", incarnation)
}

// ClearDebris removes everything under root but the claim: once the claim is held, the whole tree is a
// dead process's property, whoever bound what is in it.
func ClearDebris(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == instance.LockName {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
