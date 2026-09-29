// Package trunk is the Core's startup: the ordered chain from `exec` to serving, the rule that decides
// which failure is fatal, and the stop that undoes it.
//
// Each step establishes what the next depends on. A failure before readiness exits — a host holds a
// working instance or no instance — and a failure after it is reported, since there is then somewhere
// to report it. Cleanup belongs to the next start: nothing depends on an orderly stop having run.
package trunk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/admin"
	"github.com/yoke-project/yoke/internal/core/admission"
	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/core/discovery"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
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

	// Composition is the composition document in force, in the service form; empty reads none.
	Composition string
	// AdmitUnlaunched is the development waiver, declared when the instance is started.
	AdmitUnlaunched bool

	Config     config.Config
	Paths      instance.Paths
	Log        *slog.Logger
	Registry   *registry.Registry
	Logs       *logstore.Store
	Discovery  *discovery.Discovery
	Deployment *gate.Deployment // what the composition in force declares, once it passed the gate
	Admission  *admission.Admission
	Session    *session.Service
	Admin      *admin.Surface
	Supervisor *supervisor.Supervisor
	// Bus is the instance's event bus, which the subsystems publish on from the logging step on.
	Bus *bus.Bus

	tokens *admission.Tokens

	stoppers []func() error
	stopping atomic.Bool
}

// OnStop registers what undoes a step. The stop runs them in the reverse of the order they were given.
func (st *State) OnStop(undo func() error) { st.stoppers = append(st.stoppers, undo) }

// Stop undoes what the trunk set up, in reverse. That the instance is stopping is published first, so
// that the silence which follows is an expected one.
func (st *State) Stop() error {
	st.stopping.Store(true)
	st.publish(event.InstanceStopping(st.Paths.Name))
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
		{"stores", stores},
		{"inherited runtime facts", nothingYet},
		{"declarations", declarations},
		{"channels", channels},
		{ready, func(st *State) error {
			st.Log.Info(ready, "root", st.Paths.Root)
			st.publish(event.InstanceReady(st.Paths.Name))
			return nil
		}},
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
	// The bus exists from here on, and the process logger is told everything published on it until the
	// log store keeps the record.
	st.Bus = bus.New()
	recorded := st.Bus.Subscribe()
	st.OnStop(func() error { recorded.Close(); return nil })
	go record(st.Log, recorded)
	return nil
}

// record writes every event it is told of to the process logger, with the fields of its detail.
func record(log *slog.Logger, s *bus.Subscription) {
	for {
		d, err := s.Next(context.Background())
		if err != nil {
			return
		}
		if d.Overflow {
			log.Warn("event", "overflow", "the process logger fell behind, and events were lost")
			continue
		}
		e := d.Event
		subject := string(e.Subject.Kind) + ":" + e.Subject.ID
		if e.Subject.Incarnation != 0 {
			subject += fmt.Sprintf("#%d", e.Subject.Incarnation)
		}
		attrs := []any{"seq", e.Seq, "type", e.Type, "subject", subject, "severity", e.Severity, "actor", string(e.Actor.Class)}
		if e.Actor.Person != "" {
			attrs = append(attrs, "person", e.Actor.Person)
		}
		if e.Occurrence != "" {
			attrs = append(attrs, "occurrence", e.Occurrence)
		}
		if e.Cause != 0 {
			attrs = append(attrs, "cause", e.Cause)
		}
		var detail map[string]any
		if json.Unmarshal(e.Detail, &detail) == nil {
			for _, k := range slices.Sorted(maps.Keys(detail)) {
				attrs = append(attrs, k, detail[k])
			}
		}
		log.Info("event", attrs...)
	}
}

// publish publishes on the instance's bus, once it exists, and keeps the event's durable counterpart
// in the log store, once it is open. The counterpart is written here and not by a subscriber, which may
// be told of an overflow: an event may be lost, and a record may not.
func (st *State) publish(e event.Event) {
	if st.Bus == nil {
		return
	}
	published, err := st.Bus.Publish(e)
	if err != nil {
		st.Log.Error("event", "type", e.Type, "error", err)
		return
	}
	if st.Logs != nil {
		st.Logs.Keep(published)
	}
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

// stores opens the Registry and the log store and migrates each forward. Nothing is admitted without the
// first nor recorded without the second, so failing here is fatal.
func stores(st *State) error {
	if err := os.MkdirAll(st.Paths.State, 0o700); err != nil {
		return err
	}
	r, err := registry.Open(filepath.Join(st.Paths.State, registry.File))
	if err != nil {
		return err
	}
	st.Registry = r
	st.OnStop(r.Close)
	// The log store is opened second, so it is closed first on the way down — after everything that
	// writes to it has stopped. Nothing could be recorded without it; a write that fails later is
	// reported, and costs evidence only.
	logs, err := logstore.Open(filepath.Join(st.Paths.State, logstore.File), func(err error) { st.Log.Error("log store", "error", err) })
	if err != nil {
		return err
	}
	st.Logs = logs
	st.OnStop(logs.Close)
	return nil
}

// declarations scans the Plugin directory into the Registry and keeps scanning it, then reads the
// composition in force through the gate. In the service form a failure here leaves a deployment — the
// Plugins found are declared — so it is reported rather than fatal.
func declarations(st *State) error {
	if st.Form != Service {
		// The descriptor's units are ingested by the part of the Core that reads it.
		return nil
	}
	plugins := st.Config.Plugins
	d := discovery.New(plugins.Manifests, st.Registry, st.Log)
	d.Publishing(st.publish)
	d.Scan()
	st.Discovery = d
	if plugins.ScanInterval > 0 {
		stop := d.Every(plugins.ScanInterval)
		st.OnStop(func() error { stop(); return nil })
	}
	if st.Composition == "" {
		return nil
	}
	composition := gate.Read(st.Composition, gate.Composition)
	report, dep := gate.Check(gate.Input{
		Document:  composition,
		Moment:    gate.Starting,
		Manifests: plugins.Manifests,
		Host:      &gate.Host{Executables: plugins.Executables, StateDir: st.Paths.State, RuntimeRoot: st.Paths.Root},
	})
	for _, f := range report.Findings {
		level := slog.LevelError
		if f.Class == gate.Weaker {
			level = slog.LevelWarn
		}
		st.Log.Log(context.Background(), level, "finding", "document", f.Document, "code", f.Code, "location", f.Location, "finding", f.Message)
		if dep == nil && f.Class != gate.Weaker {
			st.publish(event.DocumentRejected(f.Document, f.Code, f.Location))
		}
	}
	if dep == nil {
		return fmt.Errorf("the composition %s is refused", st.Composition)
	}
	sum := sha256.Sum256(composition.Bytes)
	st.publish(event.DocumentResolved(st.Composition, fmt.Sprintf("%d units", len(dep.Units)), "sha256:"+hex.EncodeToString(sum[:])))
	st.Deployment = dep
	st.Units = discovery.Units(dep, d.Manifest, plugins.Executables, st.Paths.State)
	return nil
}

// channels binds every channel with its terminator: the plugin surface's, where there is a deployment for
// units to be admitted to; the administrative surface's two projections; and then the others.
func channels(st *State) error {
	all := append(adminChannels(st), st.Channels...)
	if st.Registry != nil && st.Discovery != nil {
		all = append([]Channel{pluginChannel(st)}, all...)
	}
	for _, ch := range all {
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

// pluginChannel is the plugin surface: registration, decided by admission against the Registry, the
// Manifests discovery read and the deployment in force; and the Session each acceptance opens.
func pluginChannel(st *State) Channel {
	composed := func(id string) (admission.Composed, bool) {
		if st.Deployment == nil {
			return admission.Composed{}, false
		}
		u, ok := st.Deployment.Units[id]
		return admission.Composed{Plugin: u.Plugin, Policy: u.Policy}, ok
	}
	st.tokens = admission.NewTokens(func(id string) time.Duration {
		if c, ok := composed(id); ok {
			return c.Policy.StartupWindow
		}
		return gate.Defaults().StartupWindow
	})
	st.Admission = admission.New(admission.Config{
		Registry: st.Registry, Manifest: st.Discovery.Manifest, Unit: composed, Tokens: st.tokens,
		AdmitUnlaunched: st.AdmitUnlaunched, Log: st.Log,
		Observe: func(id string, in unit.Input) {
			if st.Supervisor != nil {
				st.Supervisor.Input(id, in)
			}
		},
	})
	st.Session = session.New(session.Config{
		Lookup: func(id string) (session.Admitted, bool) {
			s, ok := st.Admission.Lookup(id)
			if !ok {
				return session.Admitted{}, false
			}
			var incarnation uint64
			if st.Supervisor != nil {
				incarnation = uint64(st.Supervisor.Status(s.Unit).Incarnation)
			}
			return session.Admitted{Unit: s.Unit, Incarnation: incarnation, Interval: s.Heartbeat.GetInterval().AsDuration(),
				Tolerance: s.Heartbeat.GetTolerance(), Scope: s.Scope}, true
		},
		Observe: func(id string, in unit.Input) {
			if st.Supervisor != nil {
				st.Supervisor.Input(id, in)
			}
		},
		Publish: st.publish,
		Log:     st.Log,
	})
	server := grpc.NewServer()
	pluginv1.RegisterRegisterServer(server, st.Admission)
	pluginv1.RegisterSessionServer(server, st.Session)
	st.OnStop(func() error { server.Stop(); return nil })
	return Channel{Name: "plugin", Path: "plugin.sock", Serve: func(l Listener) { server.Serve(l) }}
}

// adminChannels are the administrative surface's two projections, one socket each, reached by whoever the
// socket's mode lets reach it.
func adminChannels(st *State) []Channel {
	cfg := admin.Config{Publish: st.publish, Log: st.Log, Stopping: st.stopping.Load}
	if st.Registry != nil && st.Logs != nil {
		core := &admin.Core{Registry: st.Registry, Logs: st.Logs, Publish: st.publish,
			Units: supervised{st}, Sessions: held{st}, Manifest: func(id string) (*gate.Manifest, bool) {
				if st.Discovery == nil {
					return nil, false
				}
				return st.Discovery.Manifest(id)
			}}
		cfg.Operations = core.Operations()
	}
	st.Admin = admin.New(cfg)
	operator, shell := st.Admin.Operator(), st.Admin.Shell()
	st.OnStop(func() error { operator.Stop(); shell.Stop(); return nil })
	return []Channel{
		{Name: "operator", Path: "operator.sock", Serve: func(l Listener) { operator.Serve(l) }},
		{Name: "shell", Path: "shell.sock", Serve: func(l Listener) { shell.Serve(l) }},
	}
}

// supervised is the supervisor as the administrative operations reach it, from the moment it exists.
type supervised struct{ st *State }

func (u supervised) Plugin(id string) (string, bool) {
	if u.st.Supervisor == nil {
		return "", false
	}
	return u.st.Supervisor.Plugin(id)
}

func (u supervised) Of(plugin string) []string {
	if u.st.Supervisor == nil {
		return nil
	}
	return u.st.Supervisor.Of(plugin)
}

func (u supervised) Status(id string) supervisor.Status { return u.st.Supervisor.Status(id) }
func (u supervised) StartUnit(id string) error          { return u.st.Supervisor.StartUnit(id) }
func (u supervised) StopUnit(id string) error           { return u.st.Supervisor.StopUnit(id) }
func (u supervised) RestartUnit(id string) error        { return u.st.Supervisor.RestartUnit(id) }

// held are the Sessions as the administrative operations reach them, by the unit that holds one.
type held struct{ st *State }

func (s held) Open(unitID string) bool {
	if s.st.Session == nil {
		return false
	}
	_, ok := s.st.Session.Of(unitID)
	return ok
}

func (s held) Ask(ctx context.Context, unitID string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error) {
	id, ok := s.st.Session.Of(unitID)
	if !ok {
		return nil, errors.New("the unit holds no Session")
	}
	return s.st.Session.Ask(ctx, id, q)
}

func (s held) Revoke(unitID string, cause pluginv1.SessionMessage_Revoked_Cause, line string) error {
	id, ok := s.st.Session.Of(unitID)
	if !ok {
		return errors.New("the unit holds no Session")
	}
	return s.st.Session.Revoke(id, cause, line)
}

// units hands the declared units to the supervisor, which is stopped first on the way down.
func units(st *State) error {
	cfg := supervisor.Config{
		Root: st.Paths.Root, Policy: supervisor.DefaultPolicy(),
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: captured{st.Log, st.Logs},
		Publish: st.publish,
	}
	if st.tokens != nil {
		cfg.Tokens = st.tokens
		// An incarnation that ended is no longer live, and its Session goes with it.
		cfg.Ended = func(id string) { st.Admission.Release(id); st.Session.Forget(id) }
	}
	// A unit whose dependency never arrived is reported, and nothing more: past readiness a failure is
	// reported rather than fatal.
	cfg.NotStarted = func(id, cause string) { st.Log.Warn("not started", "unit", id, "cause", cause) }
	if st.Logs != nil {
		cfg.Incarnations = counted{st.Logs, st.Log}
	}
	s := supervisor.New(cfg)
	st.Supervisor = s
	st.OnStop(s.Stop)
	s.Start(st.Units)
	return nil
}

// captured keeps a unit's output in the log store, one entry per line at the routine grade, and echoes it
// to the process logger until a surface reads the store.
type captured struct {
	log  *slog.Logger
	logs *logstore.Store
}

func (c captured) Line(unitID string, incarnation int, stream, line string) {
	c.log.Info(line, "unit", unitID, "incarnation", incarnation, "stream", stream)
	if c.logs != nil {
		c.logs.Append(logstore.Entry{At: time.Now(), Unit: unitID, Incarnation: uint64(incarnation),
			Source: logstore.Source(stream), Severity: event.Routine, Message: line})
	}
}

// counted numbers a unit's lives with the log store's counter, which survives the Core. A count that
// cannot be taken is reported, and that life has no number.
type counted struct {
	logs *logstore.Store
	log  *slog.Logger
}

func (c counted) Next(unitID string) int {
	n, err := c.logs.Next(unitID)
	if err != nil {
		c.log.Error("log store", "unit", unitID, "error", fmt.Errorf("the launch could not be counted: %w", err))
		return 0
	}
	return int(n)
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
