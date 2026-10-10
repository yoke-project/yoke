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
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/admin"
	"github.com/yoke-project/yoke/internal/core/admission"
	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/core/discovery"
	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/session"
	"github.com/yoke-project/yoke/internal/core/streams"
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
	// Streams are the streams' own transports, which the Core creates before it activates a stream.
	Streams    *streams.Service
	Admin      *admin.Surface
	Supervisor *supervisor.Supervisor
	// Bus is the instance's event bus, which the subsystems publish on from the logging step on.
	Bus *bus.Bus

	tokens *admission.Tokens

	stoppers []func() error
	stopping atomic.Bool

	mu             sync.Mutex
	readyAt        time.Time
	stoppingAt     time.Time
	compositionSum string         // the digest the composition in force was read at
	composed       *composed      // the composition in force as the gate found it, read once, at step 7
	engine         *engine.Engine // the container engine step 7 reached, where a unit names an image
	weaker         []string       // the weaker arrangements the gate reported, in force
	interfaces     *interfaces.Bound
}

// channelRecords are the declared channels, as the administrative surface reads them.
func (st *State) channelRecords() []*administrativev1.ChannelRecord {
	st.mu.Lock()
	bound := st.interfaces
	st.mu.Unlock()
	if bound == nil {
		return nil
	}
	var out []*administrativev1.ChannelRecord
	for _, c := range bound.Channels() {
		d, o := c.GetDeclared(), c.GetObserved()
		out = append(out, &administrativev1.ChannelRecord{
			Declared: &administrativev1.ChannelRecord_Declared{Name: d.GetName(), Projection: d.GetProjection(), AddressClass: d.GetAddressClass()},
			Observed: &administrativev1.ChannelRecord_Observed{Attached: o.GetAttached(), Client: o.GetClient(), Suspended: o.GetSuspended(), Reason: o.GetReason()},
		})
	}
	return out
}

// transports are the streams' own transports, made the first time a step needs them: the plugin channel
// activates streams, the administrative one starts and stops them, and the supervisor ends them with a
// life.
func (st *State) transports() *streams.Service {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.Streams == nil {
		st.Streams = streams.New(streams.Config{Root: st.Paths.Root, Log: st.Log, Publish: st.publish})
	}
	return st.Streams
}

// OnStop registers what undoes a step. The stop runs them in the reverse of the order they were given.
func (st *State) OnStop(undo func() error) { st.stoppers = append(st.stoppers, undo) }

// Stop undoes what the trunk set up, in reverse. That the instance is stopping is published first, so
// that the silence which follows is an expected one.
func (st *State) Stop() error {
	st.mu.Lock()
	st.stoppingAt = time.Now()
	st.mu.Unlock()
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

// engineReach bounds how long the Core waits for the container engine to answer at step 7, and
// engineClear how long it waits for what an earlier life left to be removed.
const (
	engineReach = 10 * time.Second
	engineClear = time.Minute
)

// Steps are the eleven, in their order.
func Steps() []Step {
	return []Step{
		{"identity and paths", identity},
		{"parameters", parameters},
		{"logging", logging},
		{"claim", claim},
		{"debris", func(st *State) error { return ClearDebris(st.Paths.Root) }},
		{"stores", stores},
		{"inherited runtime facts", inherited},
		{"declarations", declarations},
		{"channels", channels},
		{ready, func(st *State) error {
			// Readiness is recorded before it is announced, so whoever reads the announcement and then the
			// instance finds it ready.
			st.mu.Lock()
			st.readyAt = time.Now()
			st.mu.Unlock()
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
	// The cleaner is the only thing that deletes what nobody asked to delete. It is stopped before the
	// store it cleans is closed.
	cleaner := &logstore.Cleaner{Store: logs, Instance: st.Paths.Name, Interval: time.Hour,
		Policy: RetentionOf(logs, func() *gate.Deployment {
			st.mu.Lock()
			defer st.mu.Unlock()
			return st.Deployment
		}),
		Report: func(err error) { st.Log.Error("retention", "error", err) }}
	ctx, stop := context.WithCancel(context.Background())
	st.Log.Info("retention", "first", cleaner.First(time.Now()).Format(time.RFC3339Nano), "interval", time.Hour)
	go cleaner.Run(ctx)
	st.OnStop(func() error { stop(); return nil })
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
	c := st.composition()
	composition, report, dep := c.document, c.report, c.deployment
	for _, f := range report.Findings {
		level := slog.LevelError
		if f.Class == gate.Weaker {
			level = slog.LevelWarn
			st.weaker = append(st.weaker, f.Code+" at "+f.Location)
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
	st.compositionSum = "sha256:" + hex.EncodeToString(sum[:])
	st.publish(event.DocumentResolved(st.Composition, fmt.Sprintf("%d units", len(dep.Units)), st.compositionSum))
	st.mu.Lock()
	st.Deployment = dep
	st.mu.Unlock()
	st.Units = discovery.Units(dep, d.Manifest, plugins.Executables, st.Paths.State)
	return nil
}

// composed is the composition in force as the gate found it.
type composed struct {
	document   gate.Document
	report     gate.Report
	deployment *gate.Deployment // nil where it is refused
}

// composition reads the composition in force and passes it through the gate, once: step 7 needs to know
// whether a unit names an image, and step 8 reports what the gate found and takes the deployment from the
// same reading. Nil where the form reads none.
func (st *State) composition() *composed {
	if st.Form != Service || st.Composition == "" {
		return nil
	}
	if st.composed == nil {
		plugins := st.Config.Plugins
		doc := gate.Read(st.Composition, gate.Composition)
		report, dep := gate.Check(gate.Input{
			Document:  doc,
			Moment:    gate.Starting,
			Manifests: plugins.Manifests,
			Host: &gate.Host{Executables: plugins.Executables, StateDir: st.Paths.State, RuntimeRoot: st.Paths.Root,
				Engine: st.reachEngine},
		})
		st.composed = &composed{doc, report, dep}
	}
	return st.composed
}

// reachEngine is the gate's way to the container engine: the one the Core then launches on. The gate asks
// it only where a unit names an image.
func (st *State) reachEngine() (gate.EngineFacts, error) {
	ctx, cancel := context.WithTimeout(context.Background(), engineReach)
	defer cancel()
	e, err := engine.Reach(ctx, st.Config.Engine)
	if err != nil {
		return gate.EngineFacts{}, err
	}
	st.Log.Info("the container engine is reached", "engine", e.Kind, "version", e.Version, "rootless", e.Rootless)
	st.engine = e
	return gate.EngineFacts{Rootless: e.Rootless}, nil
}

// inherited is step 7: what an earlier life of the instance left in the engine is stopped and removed,
// each container named. The engine is looked for only where a unit names an image, and there a Core that
// cannot establish that nothing of its own instance still runs has failed the claim's exclusivity, so an
// engine not reached or a container not removed is fatal. The engine reached is the one units launch on.
func inherited(st *State) error {
	c := st.composition()
	if c == nil {
		return nil
	}
	// The gate reached the engine where a unit names an image; an engine it could not reach is the refusal
	// this step fails on, since nothing can establish that this instance left nothing running.
	for _, f := range c.report.Findings {
		if f.Code == "engine.unreachable" {
			return fmt.Errorf("%s: %s", f.Code, f.Message)
		}
	}
	if c.deployment == nil || !namesAnImage(c.deployment) || st.engine == nil {
		return nil
	}
	e := st.engine
	ctx, cancel := context.WithTimeout(context.Background(), engineClear)
	defer cancel()
	cleared, err := supervisor.Inherited(ctx, e, st.Paths.Name)
	for _, f := range cleared {
		st.Log.Warn("an inherited container is removed", "container", f.ID, "unit", f.Unit, "incarnation", f.Incarnation, "running", f.Running)
	}
	if err != nil {
		st.engine = nil
		return err
	}
	return nil
}

func namesAnImage(d *gate.Deployment) bool {
	for _, u := range d.Units {
		if u.Image != "" {
			return true
		}
	}
	return false
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
	return interfaceChannels(st)
}

// interfaceChannels binds the channels the deployment declares, by the class of their address, and
// tells each managed interface where its channel is. Binding is fatal: a channel that failed to bind
// would change which channel prevails.
func interfaceChannels(st *State) error {
	if st.Deployment == nil || len(st.Deployment.Channels) == 0 {
		return nil
	}
	var declared []gate.Channel
	for _, ch := range st.Deployment.Channels {
		declared = append(declared, ch)
	}
	// A channel named in an arbitration rule must confirm its subscription.
	required := map[string]bool{}
	for _, rule := range st.Deployment.Arbitration {
		for _, name := range append([]string{rule.Prevails}, rule.Over...) {
			required[name] = true
		}
	}
	confirm := &interfaces.Confirmation{Every: interfaces.ConfirmEvery, Tolerance: interfaces.ConfirmTolerance, Required: required}
	transports := st.transports()
	arbiter := interfaces.NewArbiter(declared, st.Deployment.Arbitration, st.publish)
	surface := func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{
			Channel: ch, Root: st.Paths.Root, Bus: st.Bus, Publish: st.publish, Confirm: confirm, Units: supervised{st}, Active: transports.Active,
			Sessions: held{st}, Transports: transports, Stopping: st.stopping.Load, Arbiter: arbiter,
			Declared: func(id string) (*gate.Manifest, bool) {
				u, ok := st.Deployment.Units[id]
				if !ok || u.Plugin == "" || st.Discovery == nil {
					return nil, false
				}
				return st.Discovery.Manifest(u.Plugin)
			},
			Instance: func() *interfacev1.InstanceRecord {
				r := st.instanceRecord()
				return &interfacev1.InstanceRecord{Ready: r.GetReady(), Stopping: r.GetStopping(), Since: r.GetSince()}
			},
			Granted: func(id string) ([]string, []string, []string) {
				if st.Session == nil {
					return nil, nil, nil
				}
				return st.Session.Granted(id)
			},
		})
	}
	bound, err := interfaces.BindServing(st.Paths.Root, st.Form.SocketMode(), declared, surface)
	if err != nil {
		return err
	}
	st.OnStop(bound.Close)
	st.mu.Lock()
	st.interfaces = bound
	st.mu.Unlock()
	for _, ch := range declared {
		st.Log.Info("channel", "name", ch.Name, "transport", ch.Transport, "address", bound.Address(ch.Name))
		if ch.Unit == "" {
			continue
		}
		for i := range st.Units {
			if st.Units[i].ID == ch.Unit {
				st.Units[i].Channel = bound.Address(ch.Name)
			}
		}
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
	transports := st.transports()
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
			// A Session that ends takes every stream with it, before its machine concludes anything.
			if _, ended := in.(unit.SessionEnded); ended {
				transports.CloseAll(id, streams.SessionEnded)
			}
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
		core.Instance, core.Documents, core.Bus = st.instanceRecord, st.documents, st.Bus
		core.Streams = st.transports()
		core.Channels = st.channelRecords
		core.Composed = func(plugin string) bool {
			if st.Deployment == nil {
				return false
			}
			for _, u := range st.Deployment.Units {
				if u.Plugin == plugin {
					return true
				}
			}
			return false
		}
		core.Connections = func() []admin.Connection { return st.Admin.Connections() }
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

// instanceRecord is the instance as a read answers it.
func (st *State) instanceRecord() *administrativev1.InstanceRecord {
	st.mu.Lock()
	defer st.mu.Unlock()
	r := &administrativev1.InstanceRecord{Identity: st.Paths.Name, Form: map[Form]string{Service: "service", Application: "application"}[st.Form],
		Ready: !st.readyAt.IsZero(), Stopping: st.stopping.Load(), Description: st.Composition, DescriptionDigest: st.compositionSum,
		Weaker: slices.Clone(st.weaker),
		Parameters: map[string]string{
			"state_dir": st.Config.StateDir, "runtime_dir": st.Config.RuntimeDir, "engine": st.Config.Engine, "log.level": st.Config.Log.Level,
			"plugins.manifests": st.Config.Plugins.Manifests, "plugins.executables": st.Config.Plugins.Executables,
			"plugins.scan_interval": st.Config.Plugins.ScanInterval.String(),
		}}
	since := st.readyAt
	if r.Stopping {
		since = st.stoppingAt
	}
	if !since.IsZero() {
		r.Since = timestamppb.New(since)
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		r.Version = info.Main.Version
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				r.Stamp = setting.Value
			}
		}
	}
	return r
}

// documents are the documents the Core read, each as it was read: the Manifests discovery resolved, and
// the composition in force.
func (st *State) documents() []*administrativev1.DocumentRecord {
	var out []*administrativev1.DocumentRecord
	if st.Discovery != nil && st.Registry != nil {
		plugins, _ := st.Registry.Plugins()
		for _, id := range plugins {
			m, ok := st.Discovery.Manifest(id)
			p, known, _ := st.Registry.Plugin(id)
			if ok && known {
				out = append(out, &administrativev1.DocumentRecord{Path: m.Path, Resolved: id, Digest: p.ManifestDigest})
			}
		}
	}
	if st.Deployment != nil {
		out = append(out, &administrativev1.DocumentRecord{Path: st.Composition, Resolved: fmt.Sprintf("%d units", len(st.Deployment.Units)), Digest: st.compositionSum})
	}
	return out
}

// supervised is the supervisor as the administrative operations reach it, from the moment it exists.
type supervised struct{ st *State }

// declared is a unit as the deployment declares it, before the supervisor has started: from ready to the
// units step a unit is declared and observed as nothing yet.
func (u supervised) declared(id string) (supervisor.Unit, bool) {
	for _, d := range u.st.Units {
		if d.ID == id {
			return d, true
		}
	}
	return supervisor.Unit{}, false
}

func (u supervised) Plugin(id string) (string, bool) {
	if u.st.Supervisor == nil {
		d, ok := u.declared(id)
		return d.Plugin, ok
	}
	return u.st.Supervisor.Plugin(id)
}

func (u supervised) Of(plugin string) []string {
	if u.st.Supervisor == nil {
		return nil
	}
	return u.st.Supervisor.Of(plugin)
}

func (u supervised) IDs() []string {
	if u.st.Supervisor == nil {
		var ids []string
		for _, d := range u.st.Units {
			ids = append(ids, d.ID)
		}
		slices.Sort(ids)
		return ids
	}
	return u.st.Supervisor.IDs()
}

func (u supervised) Kind(id string) unit.Kind {
	if u.st.Supervisor == nil {
		d, _ := u.declared(id)
		return d.Kind
	}
	return u.st.Supervisor.Kind(id)
}

func (u supervised) Status(id string) supervisor.Status {
	if u.st.Supervisor == nil {
		return supervisor.Status{}
	}
	return u.st.Supervisor.Status(id)
}
func (u supervised) StartUnit(id string) error   { return u.st.Supervisor.StartUnit(id) }
func (u supervised) StopUnit(id string) error    { return u.st.Supervisor.StopUnit(id) }
func (u supervised) RestartUnit(id string) error { return u.st.Supervisor.RestartUnit(id) }

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

func (s held) Instruct(ctx context.Context, unitID string, c *pluginv1.Control) (*pluginv1.Ack, error) {
	id, ok := s.st.Session.Of(unitID)
	if !ok {
		return nil, errors.New("the unit holds no Session")
	}
	return s.st.Session.Instruct(ctx, id, c)
}

// units hands the declared units to the supervisor, which is stopped first on the way down.
func units(st *State) error {
	cfg := supervisor.Config{
		Root: st.Paths.Root, Instance: st.Paths.Name, Policy: supervisor.DefaultPolicy(),
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: captured{st.Log, st.Logs},
		Publish: st.publish,
	}
	if st.tokens != nil {
		cfg.Tokens = st.tokens
		// An incarnation that ended is no longer live, and its Session goes with it.
		cfg.Ended = func(id string) { st.Admission.Release(id); st.Session.Forget(id) }
	}
	// An incarnation that ended takes its streams' transports with it, whatever its own code did.
	release, transports := cfg.Ended, st.transports()
	cfg.Ended = func(id string) {
		transports.CloseAll(id, streams.UnitExited)
		if release != nil {
			release(id)
		}
	}
	// A unit whose dependency never arrived is reported, and nothing more: past readiness a failure is
	// reported rather than fatal.
	cfg.NotStarted = func(id, cause string) { st.Log.Warn("not started", "unit", id, "cause", cause) }
	if st.Logs != nil {
		cfg.Incarnations = counted{st.Logs, st.Log}
	}
	// The engine is the one step 7 reached, where a unit names an image. Where there is none, a launch in a
	// container is a fault, on which the restart policy waits.
	if st.engine != nil {
		cfg.Containers = st.engine
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

// RetentionOf resolves a group's limits, at every cleaning cycle: the store's override where the unit has
// one, replacing everything; otherwise the unit's own resolved policy; otherwise, for the Core's own group
// or a unit no longer declared, the deployment's. Without a deployment, the defaults.
func RetentionOf(logs *logstore.Store, deployment func() *gate.Deployment) func(group string) logstore.Limits {
	limits := func(p gate.Policy) logstore.Limits {
		return logstore.Limits{Age: p.RetentionAge, Bytes: uint64(p.RetentionBytes), Entries: uint64(p.RetentionEntries)}
	}
	return func(group string) logstore.Limits {
		if group != logstore.CoreGroup {
			// An override is read live, and a limit it leaves out is unconstrained.
			if o, set, err := logs.Override(group); err == nil && set {
				l := logstore.Limits{Age: o.Age}
				if o.Bytes != nil {
					l.Bytes = *o.Bytes
				}
				if o.Entries != nil {
					l.Entries = *o.Entries
				}
				return l
			}
		}
		d := deployment()
		if d == nil {
			return logstore.DefaultLimits()
		}
		if u, declared := d.Units[group]; declared {
			return limits(u.Policy)
		}
		return limits(d.Policy)
	}
}
