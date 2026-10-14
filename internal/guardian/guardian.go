// Package guardian is the account's guardian: one long-lived process per account, with two duties — bring
// the declared instances to their desired state and keep them there, and enforce that an instance which
// loses its Core loses everything that belonged to it.
//
// It is not a second supervisor; its subject is the Core, one level up. It reads the records, the
// account's configuration and, through the gate, each description; it executes Cores and listens on one
// socket. It opens no instance's stores, speaks no administrative surface, never addresses the container
// engine, and holds nothing that can block on a peer.
package guardian

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcpeer "google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/core/peer"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/hostauth"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/managerconf"
	"github.com/yoke-project/yoke/internal/version"
)

const (
	// LockName is the guardian's claim, in the account's runtime directory beside the instances' roots.
	LockName = "guardian.lock"
	// SocketName is its channel's socket, beside the claim.
	SocketName = "guardian.sock"
)

// What the guardian derives about an instance.
const (
	Starting   = "starting"   // executed, and not yet said it is alive
	Observable = "observable" // it said it is alive
	Stopping   = "stopping"   // asked to stop: an expected silence
	Waiting    = "waiting"    // failed, and waiting to try again
	Refused    = "refused"    // the gate refused it, and it is not retried until told again
	Running    = "running"    // alive under a process this guardian did not execute
	Stopped    = "stopped"
	Failed     = "failed" // an ephemeral run that failed, kept until read or reclaimed
)

// Options say where the account's runtime directory and records are, what its configuration holds, and
// whether this is the pass a host coming up makes.
type Options struct {
	Runtime   string // `$XDG_RUNTIME_DIR/yoke`
	Instances string // the records' root
	Config    managerconf.Config
	Boot      bool
	Stderr    io.Writer
}

// Socket is where the guardian of the account whose runtime directory is given listens.
func Socket(runtime string) string { return filepath.Join(runtime, SocketName) }

// Run is the guardian: it takes the account's claim, binds its socket, reconciles every record, and serves
// until nothing is left to watch. A guardian already running is the refusal *instance.AlreadyRunning.
func Run(ctx context.Context, o Options) error {
	if err := os.MkdirAll(o.Runtime, 0o700); err != nil {
		return err
	}
	held, err := instance.ClaimNamed(o.Runtime, LockName, 0o700)
	var running *instance.AlreadyRunning
	if errors.As(err, &running) {
		return fmt.Errorf("the guardian of this account is already running, as process %d: %w", running.PID, err)
	}
	if err != nil {
		return err
	}
	defer held.Release()

	g := &guardian{o: o, log: slog.New(slog.NewTextHandler(o.Stderr, nil)), watched: map[string]*watch{},
		events: make(chan event, 64), done: make(chan struct{}), subscribers: map[chan *channel.Watched]bool{}}
	// The claim proves no other guardian is entitled to the socket, so what is at its path is debris.
	path := Socket(o.Runtime)
	os.Remove(path)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return err
	}
	server := grpc.NewServer(grpc.Creds(peer.Credentials{}))
	channel.RegisterGuardianServer(server, &surface{g: g})
	go server.Serve(listener)
	g.delegated = g.delegation()
	g.log.Info("the guardian is running", "socket", path, "boot", o.Boot, "control groups", g.delegated)
	defer func() {
		// The socket goes first, so a manager arriving now finds nobody answering and starts another.
		os.Remove(path)
		close(g.done)
		server.Stop()
		g.log.Info("the guardian has nothing left to watch, and exits")
	}()
	return g.loop(ctx)
}

type guardian struct {
	o Options
	// delegated is the account's systemd running units for it, so each instance has its slice; without
	// it, the declared weaker fallback.
	delegated   bool
	log         *slog.Logger
	watched     map[string]*watch
	events      chan event
	done        chan struct{}
	subscribers map[chan *channel.Watched]bool
}

// watch is what the guardian holds about one instance it acts on.
type watch struct {
	name      string
	state     string
	findings  []string
	attempt   uint32
	wait      time.Duration
	since     time.Time
	cmd       *exec.Cmd
	pid       int
	gen       int // which execution the timers and the exit belong to
	policy    gate.Policy
	ephemeral bool
	beats     int    // the beats this execution sent, which a silence is measured from
	reason    string // why the guardian acted unasked: Silent
}

// Silent is the reason an instance is stopped and brought back that nobody asked for: its Core stopped
// saying it is alive.
const Silent = "silent"

type kind int

const (
	wake      kind = iota // reconcile one record, or every one
	exited                // a Core this guardian executed ended
	beat                  // a Core said it is alive
	noBeat                // a Core never said so within its startup window
	retry                 // a wait elapsed
	stable                // an instance stayed observable for the stability window
	kill                  // a Core did not stop within its stop window
	silence               // no beat arrived within the deployment's tolerance
	report                // the set, for the channel
	subscribe             // a subscriber arrives or leaves
)

type event struct {
	kind     kind
	name     string // empty with wake: every record
	explicit bool   // somebody asked, so a refusal is tried again
	stopping bool   // a beat saying the Core has begun an orderly shutdown
	gen      int
	pid      int
	err      error
	reply    chan any
	sub      chan *channel.Watched
	leave    bool
}

func (g *guardian) send(e event) bool {
	select {
	case g.events <- e:
		return true
	case <-g.done:
		return false
	}
}

func (g *guardian) after(d time.Duration, e event) {
	time.AfterFunc(d, func() { g.send(e) })
}

// loop is the one place the guardian's state changes. It starts by reconciling every record, and returns
// once nothing is left to watch — an instance failing to start still counting.
func (g *guardian) loop(ctx context.Context) error {
	g.reconcileAll(false, g.o.Boot)
	for {
		if g.idle() {
			return nil
		}
		select {
		case <-ctx.Done():
			g.abandon()
			return nil
		case e := <-g.events:
			g.handle(e)
		}
	}
}

// idle is a guardian with nothing to watch and nothing told to it.
func (g *guardian) idle() bool {
	if len(g.events) > 0 {
		return false
	}
	for _, w := range g.watched {
		if slices.Contains([]string{Starting, Observable, Stopping, Waiting}, w.state) {
			return false
		}
	}
	return true
}

// abandon ends every Core this guardian executed, as a guardian told to stop must.
func (g *guardian) abandon() {
	for _, w := range g.watched {
		if w.cmd != nil {
			w.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, w := range g.watched {
		if w.cmd == nil {
			continue
		}
		for syscall.Kill(w.pid, 0) == nil && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		w.cmd.Process.Kill()
	}
}

func (g *guardian) handle(e event) {
	w := g.watched[e.name]
	current := w != nil && w.gen == e.gen
	switch e.kind {
	case wake:
		if e.name == "" {
			g.reconcileAll(e.explicit, false)
		} else {
			g.reconcile(e.name, e.explicit, false)
		}
	case exited:
		if current {
			g.ended(w, e.err)
		}
	case beat:
		switch {
		case w == nil || w.pid == 0 || w.pid != e.pid:
			e.reply <- status.Errorf(codes.PermissionDenied, "a beat for %s from process %d, which this guardian did not execute for it", e.name, e.pid)
		case e.stopping && (w.state == Starting || w.state == Observable):
			// An orderly shutdown the Core announced: the silence that follows is an expected one.
			w.state, w.since, w.reason = Stopping, time.Now(), ""
			g.log.Info("the Core says it is stopping", "instance", w.name, "pid", w.pid)
			g.publish(w)
			e.reply <- nil
		case w.state == Starting:
			w.state, w.since, w.reason = Observable, time.Now(), ""
			g.log.Info("the instance is observable", "instance", w.name, "pid", w.pid, "incarnation", w.gen)
			g.after(w.policy.StabilityWindow, event{kind: stable, name: w.name, gen: w.gen})
			g.heard(w)
			g.publish(w)
			e.reply <- nil
		case w.state == Observable:
			g.heard(w)
			e.reply <- nil
		default:
			e.reply <- nil
		}
	case silence:
		if current && w.state == Observable && w.beats == e.pid {
			tolerance := w.policy.HeartbeatInterval * time.Duration(w.policy.HeartbeatTolerance+1)
			g.log.Warn("the instance is silent, and its Core is asked to stop", "instance", w.name, "pid", w.pid,
				"silent for", tolerance)
			w.reason = Silent
			g.stop(w)
		}
	case noBeat:
		if current && w.state == Starting {
			g.log.Warn("the instance never became observable, and its Core is ended", "instance", w.name, "pid", w.pid,
				"window", w.policy.StartupWindow)
			w.cmd.Process.Kill()
		}
	case retry:
		if current && w.state == Waiting {
			w.state = ""
			g.reconcile(w.name, false, false)
		}
	case stable:
		if current && w.state == Observable {
			w.attempt = 0
		}
	case kill:
		if current && w.state == Stopping && w.cmd != nil {
			g.log.Warn("the Core did not stop within its stop window, and is terminated", "instance", w.name, "pid", w.pid)
			w.cmd.Process.Kill()
		}
	case report:
		e.reply <- g.set()
	case subscribe:
		if e.leave {
			delete(g.subscribers, e.sub)
			return
		}
		g.subscribers[e.sub] = true
		for _, s := range g.set() {
			e.sub <- s
		}
	}
}

// heard is a beat: the silence it ends is measured again from now, over the interval and the intervals
// the deployment tolerates missing.
func (g *guardian) heard(w *watch) {
	w.beats++
	tolerance := w.policy.HeartbeatInterval * time.Duration(w.policy.HeartbeatTolerance+1)
	g.after(tolerance, event{kind: silence, name: w.name, gen: w.gen, pid: w.beats})
}

func (g *guardian) reconcileAll(explicit, boot bool) {
	listed, err := instancerecord.List(g.o.Instances)
	if err != nil {
		g.log.Error("the records cannot be read", "root", g.o.Instances, "error", err)
	}
	names := map[string]bool{}
	for _, l := range listed {
		names[l.Name] = true
	}
	for name := range g.watched {
		names[name] = true
	}
	for name := range names {
		g.reconcile(name, explicit, boot)
	}
}

// reconcile compares what the record declares with what is alive, and acts on the difference. At boot an
// instance comes up only where autostart is set and the desired state was not left at stopped; at any
// other time, where the desired state is running. An ephemeral record is never eligible for autostart.
func (g *guardian) reconcile(name string, explicit, boot bool) {
	w := g.watched[name]
	rec, err := instancerecord.Read(g.o.Instances, name)
	if errors.Is(err, fs.ErrNotExist) {
		// Nothing declares it any more: whatever this guardian runs for it goes.
		if w != nil && w.cmd != nil && w.state != Stopping {
			g.stop(w)
		} else if w != nil && w.cmd == nil {
			delete(g.watched, name)
		}
		return
	}
	if err != nil {
		g.log.Error("a record cannot be read", "instance", name, "error", err)
		return
	}
	want := rec.DesiredState == instancerecord.Running && rec.Outcome != instancerecord.Failed
	if boot {
		want = want && rec.Autostart && !rec.Ephemeral
	}
	holder, _ := instance.Holder(filepath.Join(g.o.Runtime, name, instance.LockName))
	ours := w != nil && w.cmd != nil
	switch {
	case ours && want:
		// Starting, observable, or already on its way down — its end reconciles it again.
	case ours:
		if w.state != Stopping {
			g.stop(w)
		}
	case holder != 0:
		// Alive under a process this guardian did not execute: it says so, and asks nothing of it.
		w = g.watching(name)
		w.state, w.pid = Running, holder
		if !want {
			g.log.Info("an instance whose desired state is stopped is alive under another process, which is asked to stop",
				"instance", name, "pid", holder)
			syscall.Kill(holder, syscall.SIGTERM)
		}
		g.publish(w)
	case want:
		if w != nil && ((w.state == Refused && !explicit) || w.state == Waiting) {
			return
		}
		g.start(rec, g.watching(name))
	case w != nil:
		w.state, w.pid = Stopped, 0
		g.publish(w)
		delete(g.watched, name)
	}
}

func (g *guardian) watching(name string) *watch {
	w := g.watched[name]
	if w == nil {
		w = &watch{name: name, policy: gate.Defaults()}
		g.watched[name] = w
	}
	return w
}

// Gated is what the gate said of an instance's start: the bundle it runs from, its deployment where the
// descriptor could be read, the resolved values, and every finding.
type Gated struct {
	Bundle     string
	Deployment *gate.Deployment
	Values     map[string]map[string]string
	Findings   []gate.Finding
}

// Refused says whether the start may not go ahead.
func (g Gated) Refused() bool {
	return g.Deployment == nil || slices.ContainsFunc(g.Findings, func(f gate.Finding) bool { return f.Class == gate.Refusal })
}

// GateStart runs the gate an instance's start runs, over its record and the account's configuration:
// what the guardian runs before it executes a Core, and what a manager may run first to say why a start
// would be refused. Delegated is whether the account's systemd runs a slice per instance.
func GateStart(o Options, rec instancerecord.Record, delegated bool) Gated {
	bundle := filepath.Join(o.Config.InstallRoot, rec.ID, rec.Version, "bundle")
	if rec.Source != "" {
		bundle = rec.Source
	}
	stateDir := instancerecord.StateDir(o.Instances, rec.Name)
	runtimeDir := filepath.Dir(o.Runtime)
	stamp, _ := os.ReadFile(filepath.Join(stateDir, "version"))
	report, dep := gate.Check(gate.Input{
		Document:  gate.Read(filepath.Join(bundle, "yoke.yaml"), gate.Descriptor),
		Moment:    gate.Starting,
		Manifests: filepath.Join(bundle, "manifests"),
		Host: &gate.Host{Bundle: bundle, StateDir: stateDir, RuntimeRoot: filepath.Join(o.Runtime, rec.Name), Instance: rec.Name,
			NoDelegation: !delegated, Arch: gate.HostArch(), Core: coreVersion(o.Config.Core), RuntimeDir: &runtimeDir,
			Authentication: func() error { return hostauth.Probe(resolved(o.Config.Core)) }},
		Instance: &gate.Instance{DescriptorDigest: rec.DescriptorDigest, Stamp: strings.TrimSpace(string(stamp))},
	})
	out := Gated{Bundle: bundle, Deployment: dep, Findings: report.Findings}
	if dep != nil {
		var refused []gate.Finding
		out.Values, refused = gate.Values(dep, rec.Parameters)
		out.Findings = append(out.Findings, refused...)
	}
	return out
}

// start runs the gate, which nothing may execute a Core without, and then executes the Core with the
// invocation drawn from the record and the configuration. The start is finished at its first beat.
func (g *guardian) start(rec instancerecord.Record, w *watch) {
	w.ephemeral = rec.Ephemeral
	gated := GateStart(g.o, rec, g.delegated)
	bundle, dep, values, findings := gated.Bundle, gated.Deployment, gated.Values, gated.Findings
	if gated.Refused() {
		g.refuse(w, findings)
		return
	}
	w.policy = dep.Policy
	for _, f := range findings {
		if f.Class == gate.Weaker {
			g.log.Warn("weaker", "instance", rec.Name, "code", f.Code, "location", f.Location, "finding", f.Message)
		}
	}
	// What an earlier incarnation left is never carried into the next.
	g.empty(rec.Name, 0)

	args := []string{"--instance", rec.Name, "--bundle", bundle}
	if g.o.Config.Engine != "" && slices.ContainsFunc(dep.Order, func(n string) bool { return dep.Units[n].Image != "" }) {
		args = append(args, "--engine", g.o.Config.Engine)
	}
	for _, limit := range []string{"age", "bytes", "entries"} {
		if v, ok := g.o.Config.Retention[limit]; ok {
			args = append(args, "--retention", limit+"="+v)
		}
	}
	for _, group := range sorted(values) {
		for _, name := range sorted(values[group]) {
			args = append(args, "--param", group+"."+name+"="+values[group][name])
		}
	}
	if rec.Verification == instancerecord.Waived {
		args = append(args, "--unverified")
	}
	if rec.Admission == instancerecord.UnlaunchedPermitted {
		args = append(args, "--admit-unlaunched")
	}
	// The Core derives its paths from the same two directories the guardian reads the records and the
	// claims under, whatever the environment left implicit.
	dirs := []string{"XDG_RUNTIME_DIR=" + filepath.Dir(g.o.Runtime), "XDG_STATE_HOME=" + filepath.Dir(filepath.Dir(g.o.Instances))}
	cmd := exec.Command(g.o.Config.Core, args...)
	cmd.Env = append(os.Environ(), dirs...)
	if g.delegated {
		// The Core is executed inside the instance's slice, which systemd makes on demand: it, its host units
		// and the containers it names the slice as the parent of all sit in it. systemd-run reaches the
		// account's systemd with the guardian's own environment, and the Core is handed its directories.
		run, _ := exec.LookPath("systemd-run")
		env, _ := exec.LookPath("env")
		cmd = exec.Command(run, append(append([]string{"--user", "--scope", "--quiet", "--collect", "--slice=" + engine.Slice(rec.Name),
			"--", env}, dirs...), append([]string{g.o.Config.Core}, args...)...)...)
	} else {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Stdout, cmd.Stderr = g.o.Stderr, g.o.Stderr
	w.gen++
	w.findings, w.beats = nil, 0
	if w.reason == Silent {
		g.log.Info("the instance is brought back as a new incarnation", "instance", rec.Name, "attempt", w.attempt, "incarnation", w.gen)
	}
	if err := cmd.Start(); err != nil {
		g.log.Error("the Core cannot be executed", "instance", rec.Name, "core", g.o.Config.Core, "error", err)
		g.failed(w, err)
		return
	}
	w.cmd, w.pid, w.state, w.since = cmd, cmd.Process.Pid, Starting, time.Now()
	g.log.Info("the Core is executed", "instance", rec.Name, "pid", w.pid, "bundle", bundle)
	gen := w.gen
	go func() {
		err := cmd.Wait()
		g.send(event{kind: exited, name: rec.Name, gen: gen, err: err})
	}()
	g.after(w.policy.StartupWindow, event{kind: noBeat, name: rec.Name, gen: gen})
	g.publish(w)
}

// refuse leaves the instance refused: the desired state untouched, nothing retried until somebody says
// something changed, and the reason written where the guardian's output goes.
func (g *guardian) refuse(w *watch, findings []gate.Finding) {
	w.state, w.since, w.findings, w.pid = Refused, time.Now(), nil, 0
	for _, f := range findings {
		if f.Class != gate.Refusal {
			continue
		}
		w.findings = append(w.findings, fmt.Sprintf("%s at %s: %s", f.Code, f.Location, f.Message))
		g.log.Error("the start is refused", "instance", w.name, "code", f.Code, "location", f.Location, "finding", f.Message)
	}
	g.publish(w)
}

// stop asks the Core to stop, and terminates it once its stop window has passed.
func (g *guardian) stop(w *watch) {
	w.state, w.since = Stopping, time.Now()
	if w.reason == "" {
		g.log.Info("the Core is asked to stop", "instance", w.name, "pid", w.pid)
	}
	w.cmd.Process.Signal(syscall.SIGTERM)
	g.after(w.policy.StopWindow, event{kind: kill, name: w.name, gen: w.gen})
	g.publish(w)
}

// ended is a Core this guardian executed coming to an end: an orderly stop, or a failure.
func (g *guardian) ended(w *watch, err error) {
	g.empty(w.name, w.pid)
	w.cmd, w.pid = nil, 0
	if w.state == Stopping && w.reason == Silent {
		// What a silence ends is not an orderly stop: the instance is brought back under the restart figures.
		g.log.Warn("the silent Core has ended, and the instance is brought back", "instance", w.name)
		g.failed(w, err)
		return
	}
	if w.state == Stopping {
		g.log.Info("the Core has stopped", "instance", w.name)
		if w.ephemeral {
			// An ephemeral record disappears on an orderly stop.
			if err := os.RemoveAll(filepath.Join(g.o.Instances, w.name)); err != nil {
				g.log.Error("an ephemeral record cannot be removed", "instance", w.name, "error", err)
			}
		}
		w.state = Stopped
		g.publish(w)
		delete(g.watched, w.name)
		g.reconcile(w.name, false, false)
		return
	}
	g.log.Warn("the Core ended", "instance", w.name, "error", err)
	g.failed(w, err)
}

// failed is a start that did not hold. An ephemeral run is marked failed and kept, which is the one field
// the guardian writes; a declared instance waits, the wait growing to a ceiling, and is tried again.
func (g *guardian) failed(w *watch, err error) {
	if w.ephemeral {
		if err := instancerecord.Update(g.o.Instances, w.name, func(r *instancerecord.Record) error {
			r.Outcome = instancerecord.Failed
			return nil
		}); err != nil {
			g.log.Error("a failed ephemeral run cannot be marked", "instance", w.name, "error", err)
		}
		w.state, w.since = Failed, time.Now()
		g.publish(w)
		delete(g.watched, w.name)
		return
	}
	w.attempt++
	w.wait = w.policy.Backoff
	for i := uint32(1); i < w.attempt && w.wait < w.policy.Ceiling; i++ {
		w.wait *= 2
	}
	w.wait = min(w.wait, w.policy.Ceiling)
	w.state, w.since = Waiting, time.Now()
	g.log.Info("the instance waits to be tried again", "instance", w.name, "attempt", w.attempt, "wait", w.wait)
	g.after(w.wait, event{kind: retry, name: w.name, gen: w.gen})
	g.publish(w)
}

func (w *watch) watched() *channel.Watched {
	out := &channel.Watched{Name: w.name, State: w.state, Findings: slices.Clone(w.findings), Attempt: w.attempt, Pid: int32(w.pid),
		Reason: w.reason}
	if w.wait > 0 && w.state == Waiting {
		out.Wait = durationpb.New(w.wait)
	}
	if !w.since.IsZero() {
		out.Since = timestamppb.New(w.since)
	}
	return out
}

func (g *guardian) publish(w *watch) {
	s := w.watched()
	for sub := range g.subscribers {
		select {
		case sub <- s:
		default:
			// A subscriber that does not keep up loses changes, never the guardian's attention.
		}
	}
}

// set is every declared instance and what was derived about each: the guardian's account where it acts on
// one, and otherwise its claim.
func (g *guardian) set() []*channel.Watched {
	var out []*channel.Watched
	seen := map[string]bool{}
	listed, _ := instancerecord.List(g.o.Instances)
	for _, l := range listed {
		seen[l.Name] = true
		if w := g.watched[l.Name]; w != nil {
			out = append(out, w.watched())
			continue
		}
		state, pid := Stopped, 0
		if l.Record != nil && l.Record.Outcome == instancerecord.Failed {
			state = Failed
		}
		if holder, _ := instance.Holder(filepath.Join(g.o.Runtime, l.Name, instance.LockName)); holder != 0 {
			state, pid = Running, holder
		}
		out = append(out, &channel.Watched{Name: l.Name, State: state, Pid: int32(pid)})
	}
	for name, w := range g.watched {
		if !seen[name] {
			out = append(out, w.watched())
		}
	}
	slices.SortFunc(out, func(a, b *channel.Watched) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func sorted[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// surface is the channel's server: it tells the loop, and answers from what the loop says.
type surface struct {
	channel.UnimplementedGuardianServer
	g *guardian
}

func (s *surface) Reconcile(ctx context.Context, r *channel.ReconcileRequest) (*channel.ReconcileResponse, error) {
	if _, err := os.Stat(instancerecord.File(s.g.o.Instances, r.GetName())); err != nil || instance.ValidateName(r.GetName()) != nil {
		return nil, refusal(codes.NotFound, "record.unknown", r.GetName(), "no record declares an instance named %q", r.GetName())
	}
	if !s.g.send(event{kind: wake, name: r.GetName(), explicit: true}) {
		return nil, status.Error(codes.Unavailable, "the guardian is exiting")
	}
	return &channel.ReconcileResponse{}, nil
}

func (s *surface) ReconcileAll(context.Context, *channel.ReconcileAllRequest) (*channel.ReconcileAllResponse, error) {
	if !s.g.send(event{kind: wake, explicit: true}) {
		return nil, status.Error(codes.Unavailable, "the guardian is exiting")
	}
	return &channel.ReconcileAllResponse{}, nil
}

func (s *surface) Report(ctx context.Context, _ *channel.ReportRequest) (*channel.ReportResponse, error) {
	reply := make(chan any, 1)
	if !s.g.send(event{kind: report, reply: reply}) {
		return nil, status.Error(codes.Unavailable, "the guardian is exiting")
	}
	select {
	case set := <-reply:
		return &channel.ReportResponse{Instances: set.([]*channel.Watched)}, nil
	case <-s.g.done:
		return nil, status.Error(codes.Unavailable, "the guardian is exiting")
	}
}

func (s *surface) Subscribe(_ *channel.SubscribeRequest, stream channel.Guardian_SubscribeServer) error {
	sub := make(chan *channel.Watched, 256)
	if !s.g.send(event{kind: subscribe, sub: sub}) {
		return status.Error(codes.Unavailable, "the guardian is exiting")
	}
	defer s.g.send(event{kind: subscribe, sub: sub, leave: true})
	for {
		select {
		case w := <-sub:
			if err := stream.Send(w); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		case <-s.g.done:
			// What the guardian concluded before it exited is still told: its last changes are queued.
			for {
				select {
				case w := <-sub:
					if err := stream.Send(w); err != nil {
						return err
					}
				default:
					return nil
				}
			}
		}
	}
}

// Alive is a Core's liveness. Its first beat is observability, and a beat is accepted only from the
// process the guardian executed for the instance it names, which the socket's peer credential says.
func (s *surface) Alive(stream channel.Guardian_AliveServer) error {
	var pid int
	if p, ok := grpcpeer.FromContext(stream.Context()); ok {
		if info, ok := p.AuthInfo.(peer.Info); ok {
			pid = int(info.PID)
		}
	}
	for {
		b, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&channel.AliveResponse{})
		}
		if err != nil {
			return err
		}
		reply := make(chan any, 1)
		if !s.g.send(event{kind: beat, name: b.GetInstance(), pid: pid, stopping: b.GetStopping(), reply: reply}) {
			return status.Error(codes.Unavailable, "the guardian is exiting")
		}
		select {
		case answer := <-reply:
			if err, refused := answer.(error); refused && err != nil {
				return err
			}
		case <-s.g.done:
			return status.Error(codes.Unavailable, "the guardian is exiting")
		}
	}
}

// refusal is a status carrying the code, the message and the subject the channel's refusals carry.
func refusal(c codes.Code, code, subject, format string, args ...any) error {
	r := &channel.Refusal{Code: code, Message: fmt.Sprintf(format, args...), Subject: subject}
	st, err := status.New(c, r.Message).WithDetails(r)
	if err != nil {
		return status.Error(c, r.Message)
	}
	return st.Err()
}

// delegation says whether the account's systemd runs units for it, which is what gives every instance a
// slice of its own: asked once, by running nothing in a scope.
func (g *guardian) delegation() bool {
	run, err := exec.LookPath("systemd-run")
	if err != nil {
		g.log.Warn("no control group can be made for an instance: systemd-run is not on the PATH")
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if said, err := exec.CommandContext(ctx, run, "--user", "--scope", "--quiet", "--collect", "true").CombinedOutput(); err != nil {
		g.log.Warn("no control group can be made for an instance: the account's systemd runs nothing for it", "said",
			strings.TrimSpace(string(said)), "error", err)
		return false
	}
	return true
}

// empty ends everything that belonged to an instance, in one operation over its slice, with nothing
// enumerated. Without delegation, the declared weaker fallback: the process group the Core led is
// signalled, and the containers carrying the instance's label are reaped — which addresses the engine.
func (g *guardian) empty(name string, pid int) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if g.delegated {
		systemctl, err := exec.LookPath("systemctl")
		if err != nil {
			g.log.Error("the instance's slice cannot be emptied: systemctl is not on the PATH", "instance", name)
			return
		}
		slice := engine.Slice(name)
		exec.CommandContext(ctx, systemctl, "--user", "kill", "--signal=SIGKILL", slice).Run()
		exec.CommandContext(ctx, systemctl, "--user", "stop", slice).Run()
		return
	}
	if pid > 0 {
		syscall.Kill(-pid, syscall.SIGKILL)
	}
	if g.o.Config.Engine == "" {
		return
	}
	e, err := engine.Reach(ctx, g.o.Config.Engine)
	if err != nil {
		return
	}
	reaped, err := supervisor.Inherited(ctx, e, name)
	for _, f := range reaped {
		g.log.Warn("a container of the instance is reaped by its label", "instance", name, "container", f.ID, "unit", f.Unit)
	}
	if err != nil {
		g.log.Error("the instance's containers cannot all be reaped", "instance", name, "error", err)
	}
}

// resolved is the Core's executable a configuration names: a path, or a program found on the search path.
func resolved(core string) string {
	if !filepath.IsAbs(core) {
		if found, err := exec.LookPath(core); err == nil {
			return found
		}
	}
	return core
}

// coreVersion is the version of the Core the configuration names, read from the binary without
// executing it; nil where it cannot be read, which leaves the range to the Core's own pass.
func coreVersion(core string) *version.Version {
	v, err := version.OfBinary(resolved(core))
	if err != nil {
		return nil
	}
	return &v
}
