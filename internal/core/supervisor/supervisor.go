// Package supervisor is the only part of the Core that touches operating-system processes. It launches
// what the deployment declares, learns of each exit as it happens, captures output, and applies the
// restart policy to the terminal state the lifecycle machine concluded.
//
// It is authoritative for whether a process ended and how, and for nothing about what that means. Where
// it cannot observe, it says so and concludes nothing.
package supervisor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// Unit is one declared unit, as the composing document gives it.
type Unit struct {
	ID               string
	Kind             unit.Kind
	Plugin           string // the plugin a Plugin unit is a copy of
	Exec             string // the resolved executable
	Image            string // an image pinned by digest, for a unit that runs in a container; then Exec is empty
	Digest           string // `sha256:<hex>`, the identity the executable must have; empty where there is none to compare
	Args             []string
	Env              map[string]string
	RestartOnFailure bool     // `restart.on_failure`, a unit that runs to completion's to declare
	DependsOn        []string // the units this one waits on, each until it is ready by its kind
	Policy           *Policy  // the unit's own figures; nil takes the deployment's
	Channel          string   // a managed interface's: the address of the channel that names it
	Needs            []Need   // what each launch expands, on the host or in a container
}

// Need is one declared need as a launch expands it: its class, and the path a device is bound to or a
// storage need is materialised at. A secret is not among them: it is 0.5's.
type Need struct {
	Class string // device, storage, display, audio or network
	Path  string
}

// Policy holds the deployment's figures.
type Policy struct {
	StartupWindow   time.Duration
	StopWindow      time.Duration
	Backoff         time.Duration // the first wait; each attempt doubles it
	Ceiling         time.Duration // the wait stops growing here, and attempts continue
	StabilityWindow time.Duration // how long a unit stays ready for its count to reset
}

// DefaultPolicy is the figures a deployment gets when its policy block says nothing.
func DefaultPolicy() Policy {
	return Policy{StartupWindow: 30 * time.Second, StopWindow: 10 * time.Second,
		Backoff: 5 * time.Second, Ceiling: 5 * time.Minute, StabilityWindow: 60 * time.Second}
}

// Incarnations counts the lives of each unit.
type Incarnations interface{ Next(unitID string) int }

// Tokens issues the bootstrap token a launch carries.
type Tokens interface{ Issue(unitID string) string }

// Output receives each line a unit writes, with the incarnation that wrote it and the stream it wrote
// it on: "stdout" or "stderr".
type Output interface {
	Line(unitID string, incarnation int, stream, line string)
}

// Config is what a supervisor is built with.
type Config struct {
	Root     string // the instance root: every path handed to a unit derives from it
	Instance string // the instance's identity, which labels its containers and names its slice
	// Containers is the engine a unit naming an image is launched on; nil where none was reached.
	Containers   Containers
	Policy       Policy
	Incarnations Incarnations
	Tokens       Tokens
	Output       Output
	// Ended is told when an incarnation reaches a terminal state, so that what is keyed by a live unit
	// elsewhere can let it go. Optional.
	Ended func(unitID string)
	// NotStarted is told of a unit whose dependency never arrived, with the chain that caused it. Optional.
	NotStarted func(unitID, cause string)
	// LaunchFailed is told of a launch that produced nothing, with why. It never waits. Optional.
	LaunchFailed func(unitID, why string)
	// Publish is told each state a unit's life enters, as the event the Core concluded. It never waits.
	// Optional.
	Publish func(event.Event)
	// Getenv reads the Core's own environment, where a display and an audio path are found at each
	// launch. Nil reads the process's.
	Getenv func(string) string
	// X11 is the directory X11 servers listen in. Empty is /tmp/.X11-unix.
	X11 string
}

// Containers is what the supervisor asks of a container engine.
type Containers interface {
	Create(ctx context.Context, l engine.Launch) (string, error)
	Attach(ctx context.Context, id string, stdout, stderr io.Writer) (<-chan struct{}, error)
	Start(ctx context.Context, id string) error
	Signal(ctx context.Context, id, signal string) error
	Remove(ctx context.Context, id string) error
	Events(ctx context.Context, instance string) (<-chan engine.Event, error)
	List(ctx context.Context, instance string) ([]engine.Found, error)
	Returned(ctx context.Context) (<-chan struct{}, error)
}

// Status is what is observed of a unit: its state, and the facts about its next incarnation.
type Status struct {
	State   unit.State
	Backend string
	// Since is the moment of the last transition, and ConditionSince when the condition last changed.
	Since          time.Time
	ConditionSince time.Time
	Incarnation    int
	PID            int
	Token          string
	Condition      unit.Condition
	HasCondition   bool
	Failure        string

	Waiting bool          // between attempts: a fact about the next incarnation, not a state
	Attempt int           // the attempt the unit is on, within this episode
	Wait    time.Duration // how long this wait is
	NextAt  time.Time     // when the next attempt is due

	Unobservable      bool
	UnobservableSince time.Time

	Awaiting   []string // the dependencies not yet ready, while the unit waits to be launched
	NotStarted string   // why the unit was not started, once its window ran out waiting
}

// The two backends a unit runs on: the host, where the kernel reports its process's end, and a container,
// where the engine's events do.
const (
	hostBackend      = "host"
	containerBackend = "container"
)

// backendOf is the backend a unit runs on.
func backendOf(u Unit) string {
	if u.Image != "" {
		return containerBackend
	}
	return hostBackend
}

// unobserved is the condition a unit carries while its backend is quiet: the one the Core grades itself.
func unobserved(backend string) unit.Condition {
	source := "the host"
	if backend == containerBackend {
		source = "the container engine"
	}
	return unit.Condition{Grade: event.Notable, Line: "not currently observable: " + source + " has stopped reporting"}
}

// ErrUnreachable is the error of an act that needs the backend while it cannot be reached.
var ErrUnreachable = errors.New("the backend cannot be reached")

type managed struct {
	decl        Unit
	machine     *unit.Machine
	status      Status
	running     running // the incarnation's process, however it was launched; nil when there is none
	exited      chan struct{}
	launching   int // the attempt a container is being launched for, which a stop or a new attempt supersedes
	incarnation int
	failures    int
	readyAt     time.Time
	windowOut   bool
	restart     *time.Timer

	exitStatus int  // the status the last process ended with
	hasExit    bool // whether the last process ended by exiting, rather than never starting
	held       bool // waiting on its dependencies, with no incarnation yet
	unobserved bool // carrying the condition of a quiet backend, which it was running on when it went quiet
	hold       *time.Timer
}

// Supervisor supervises the units of one instance.
type Supervisor struct {
	cfg Config

	mu        sync.Mutex
	units     map[string]*managed
	order     []string
	stopOrder []string
	stopping  bool
	quiet     map[string]time.Time // the backends gone quiet, and since when

	life context.Context // ends once the supervisor has stopped, and what follows the engine with it
	end  context.CancelFunc

	follows sync.Mutex // held while the engine is followed again and reconciled with, never with mu or ev

	launches sync.Mutex // held while one container is created, attached and started; taken before mu or ev, never with them

	ev        sync.Mutex          // guards what follows, never held with mu
	events    bool                // whether the engine's events are being followed
	returning bool                // whether the engine's return is being waited for
	watching  map[string]*watched // the containers launched, by identity, until their end is concluded
	inflight  map[string]bool     // the lives being launched in a container, as unit#incarnation
	held      []*managed          // the units waiting on their dependencies, in the order they were given
}

// New is a supervisor with no unit yet.
func New(cfg Config) *Supervisor {
	life, end := context.WithCancel(context.Background())
	return &Supervisor{cfg: cfg, units: map[string]*managed{}, quiet: map[string]time.Time{}, life: life, end: end,
		watching: map[string]*watched{}, inflight: map[string]bool{}}
}

// Root is the instance root the supervisor derives a unit's paths from.
func (s *Supervisor) Root() string { return s.cfg.Root }

// Launch starts a unit, and keeps it supervised for as long as the instance runs.
func (s *Supervisor) Launch(u Unit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := &managed{decl: u, machine: unit.NewMachine(u.Kind)}
	s.units[u.ID] = m
	s.order = append(s.order, u.ID)
	s.attempt(m)
}

// Start launches the units a deployment declares, in the order their dependencies fix: each unit with
// no unmet dependency at once, each other one when its last dependency becomes ready by its kind. A unit
// whose dependencies have not all arrived when its startup window runs out is not started, and told.
func (s *Supervisor) Start(units []Unit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var waiting []*managed
	for _, u := range units {
		m := &managed{decl: u, machine: unit.NewMachine(u.Kind)}
		s.units[u.ID] = m
		waiting = append(waiting, m)
	}
	for _, m := range waiting {
		if len(m.decl.DependsOn) == 0 {
			s.order = append(s.order, m.decl.ID)
			s.attempt(m)
			continue
		}
		m.held = true
		m.status.Awaiting = append([]string(nil), m.decl.DependsOn...)
		m.hold = time.AfterFunc(s.policy(m).StartupWindow, func() { s.neverArrived(m) })
		s.held = append(s.held, m)
	}
}

// ready says whether a unit has become ready by the definition its kind fixes: a Plugin unit when its
// Session is open, a unit that runs to completion when it exited zero.
func ready(kind unit.Kind, state unit.State) bool {
	if kind == unit.Oneshot {
		return state == unit.Completed
	}
	return state == unit.Running
}

// release launches every held unit whose last dependency has just become ready. Lock held.
func (s *Supervisor) release(id string) {
	var still []*managed
	var free []*managed
	for _, h := range s.held {
		h.status.Awaiting = slices.DeleteFunc(h.status.Awaiting, func(d string) bool { return d == id })
		if len(h.status.Awaiting) == 0 && !s.stopping {
			free = append(free, h)
			continue
		}
		still = append(still, h)
	}
	s.held = still
	for _, h := range free {
		h.hold.Stop()
		h.held, h.status.Awaiting = false, nil
		s.order = append(s.order, h.decl.ID)
		s.attempt(h)
	}
}

// neverArrived is a held unit's window running out: it is not started, and the chain is reported.
func (s *Supervisor) neverArrived(m *managed) {
	s.mu.Lock()
	if s.stopping || !m.held {
		s.mu.Unlock()
		return
	}
	cause := s.chain(m, map[string]bool{})
	m.held, m.status.Awaiting, m.status.NotStarted = false, nil, cause
	s.held = slices.DeleteFunc(s.held, func(h *managed) bool { return h == m })
	tell := s.cfg.NotStarted
	s.mu.Unlock()
	if tell != nil {
		tell(m.decl.ID, cause)
	}
}

// chain says why a unit did not start, back to the unit that actually failed. Lock held.
func (s *Supervisor) chain(m *managed, seen map[string]bool) string {
	seen[m.decl.ID] = true
	var reasons []string
	for _, d := range m.status.Awaiting {
		reasons = append(reasons, s.why(d, seen))
	}
	return m.decl.ID + " did not start because " + strings.Join(reasons, " and ")
}

// why says what a dependency is doing instead of being ready. Lock held.
func (s *Supervisor) why(id string, seen map[string]bool) string {
	d, declared := s.units[id]
	switch {
	case !declared:
		return id + " does not start with the instance"
	case seen[id]:
		return id + " is waiting as well"
	case d.status.NotStarted != "":
		return d.status.NotStarted
	case d.held:
		return s.chain(d, seen)
	}
	state := d.machine.State()
	switch {
	case !state.Terminal():
		return fmt.Sprintf("%s had not become ready, being %s", id, state)
	case state == unit.Refused:
		return id + " was refused"
	case state == unit.Stopped:
		return id + " was stopped"
	case d.status.Failure != "":
		return id + " could not be launched: " + d.status.Failure
	case d.windowOut:
		return id + " did not become ready within its startup window"
	case d.hasExit:
		return fmt.Sprintf("%s exited %d", id, d.exitStatus)
	}
	return id + " failed"
}

// Declare changes a unit's declaration for its next attempt.
func (s *Supervisor) Declare(unitID string, change func(*Unit)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.units[unitID]; ok {
		change(&m.decl)
	}
}

// attempt performs one launch: a new machine, a new incarnation, a new token. Called with the lock held.
func (s *Supervisor) attempt(m *managed) {
	m.machine = unit.NewMachine(m.decl.Kind)
	// Until the launch is counted, this attempt's life has no number.
	m.incarnation = 0
	m.status.Waiting, m.status.Failure, m.windowOut = false, "", false
	m.exitStatus, m.hasExit = 0, false
	m.running, m.exited = nil, nil
	m.launching++

	// A launch on a quiet host fails as an attempt; one in a container tries to reach the engine, and fails
	// as an attempt where it cannot.
	if _, quiet := s.quiet[hostBackend]; quiet && backendOf(m.decl) == hostBackend {
		s.failedLaunch(m, fmt.Sprintf("the %s backend cannot be reached", hostBackend))
		return
	}
	// A Plugin unit binds its own socket under plugins/, which the Core provides: the unit supplies the
	// bind and never the place.
	if m.decl.Kind == unit.Plugin {
		if err := os.MkdirAll(filepath.Join(s.cfg.Root, "plugins"), 0o750); err != nil {
			s.failedLaunch(m, fmt.Sprintf("the directory for the unit's socket cannot be made: %v", err))
			return
		}
	}
	// Every need is expanded at every launch: what held when the instance started says nothing about now.
	needs, err := s.expand(m.decl, m.decl.Image != "")
	if err != nil {
		s.failedLaunch(m, err.Error())
		return
	}
	if m.decl.Image != "" {
		s.inContainer(m, needs)
		return
	}
	file, err := os.Open(m.decl.Exec)
	if err != nil {
		s.failedLaunch(m, fmt.Sprintf("the executable %s cannot be opened: %v", m.decl.Exec, err))
		return
	}
	// The digest is taken through the descriptor that is then executed, so nothing can come in between.
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		file.Close()
		s.failedLaunch(m, fmt.Sprintf("the executable %s cannot be read: %v", m.decl.Exec, err))
		return
	}
	// In the service form there is no expected value: the files are the system's, under a root-owned
	// directory, and a comparison would be with nothing.
	if got := "sha256:" + hex.EncodeToString(sum.Sum(nil)); m.decl.Digest != "" && got != m.decl.Digest {
		file.Close()
		s.failedLaunch(m, fmt.Sprintf("the executable %s is %s, and the declaration says %s", m.decl.Exec, got, m.decl.Digest))
		return
	}

	token := s.issue(m.decl)
	command := &exec.Cmd{
		Path:        "/proc/self/fd/3",
		Args:        append([]string{m.decl.Exec}, m.decl.Args...),
		Env:         append(s.Environment(m.decl, token), needs.env...),
		ExtraFiles:  []*os.File{file},
		SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
		WaitDelay:   time.Second,
	}
	incarnation := s.cfg.Incarnations.Next(m.decl.ID)
	stdout := heldUntilOpened(func(line string) { s.cfg.Output.Line(m.decl.ID, incarnation, "stdout", line) })
	stderr := heldUntilOpened(func(line string) { s.cfg.Output.Line(m.decl.ID, incarnation, "stderr", line) })
	defer func() { stdout.open(); stderr.open() }()
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		file.Close()
		s.failedLaunch(m, fmt.Sprintf("the unit could not be started: %v", err))
		return
	}
	file.Close()

	m.incarnation, m.running, m.exited = incarnation, hostProcess{command.Process.Pid}, make(chan struct{})
	m.status.Incarnation, m.status.PID, m.status.Token = incarnation, command.Process.Pid, token
	s.publish(event.StateChangedOf(m.decl.ID, pluginOf(m.decl), uint64(incarnation), "", unit.Starting))
	m.status.Since = time.Now()
	s.apply(m, unit.ProcessStarted{})

	exited, window := m.exited, time.AfterFunc(s.policy(m).StartupWindow, func() { s.windowElapsed(m, incarnation) })
	go func() {
		command.Wait()
		stdout.flush()
		stderr.flush()
		window.Stop()
		s.ended(m, incarnation, command.ProcessState.ExitCode())
		close(exited)
	}()
}

// Environment is what a unit is handed: the reserved variables, then the declaration's own.
func (s *Supervisor) Environment(u Unit, token string) []string {
	if u.Kind == unit.Oneshot {
		// Nothing connects to an exit status: a unit that runs to completion is told who it is, and has no
		// socket to bind, no admission to present a token to and no Manifest.
		env := []string{"YOKE_UNIT=" + u.ID}
		for name, value := range u.Env {
			env = append(env, name+"="+value)
		}
		return env
	}
	if u.Kind == unit.Interface {
		// A managed interface consumes a channel the Core bound: it is told where, and nothing a Plugin is
		// told, having nothing to bind, no admission to present a token to and no Manifest.
		env := []string{"YOKE_UNIT=" + u.ID, "YOKE_SOCKET=" + u.Channel}
		for name, value := range u.Env {
			env = append(env, name+"="+value)
		}
		return env
	}
	env := []string{
		"YOKE_UNIT=" + u.ID,
		"YOKE_SOCKET=" + filepath.Join(s.cfg.Root, "plugin.sock"),
		"YOKE_BIND=" + filepath.Join(s.cfg.Root, "plugins", u.ID+".sock"),
		"YOKE_TOKEN=" + token,
	}
	if u.Kind == unit.Plugin {
		env = append(env, "YOKE_PLUGIN="+u.Plugin)
	}
	for name, value := range u.Env {
		env = append(env, name+"="+value)
	}
	return env
}

// issue is the bootstrap token a launch carries: a Plugin unit's alone, the one kind that presents an
// admission.
func (s *Supervisor) issue(u Unit) string {
	if u.Kind != unit.Plugin {
		return ""
	}
	return s.cfg.Tokens.Issue(u.ID)
}

// failedLaunch is an attempt that produced no process: an ordinary failed attempt. Lock held.
func (s *Supervisor) failedLaunch(m *managed, why string) {
	m.status.Failure = why
	if tell := s.cfg.LaunchFailed; tell != nil {
		go tell(m.decl.ID, why)
	}
	s.apply(m, unit.LaunchFailed{Reason: why})
	s.settle(m)
}

// windowElapsed ends a unit that is still starting when its window runs out. The machine concludes
// from the exit this causes.
func (s *Supervisor) windowElapsed(m *managed, incarnation int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.incarnation != incarnation || m.machine.State() != unit.Starting || m.running == nil {
		return
	}
	m.windowOut = true
	m.running.kill()
}

// ended is the kernel's report that an incarnation's process is gone.
func (s *Supervisor) ended(m *managed, incarnation, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.incarnation != incarnation {
		return
	}
	m.running = nil
	m.exitStatus, m.hasExit = status, true
	if m.windowOut {
		s.apply(m, unit.WindowElapsed{})
	} else {
		s.apply(m, unit.ProcessExited{Status: status})
	}
	s.settle(m)
}

// pluginOf is the plugin a unit is a copy of, which its lifecycle events name; none for another kind.
func pluginOf(u Unit) string {
	if u.Kind == unit.Plugin {
		return u.Plugin
	}
	return ""
}

// apply gives the machine one input and records what it concluded. Lock held.
func (s *Supervisor) apply(m *managed, in unit.Input) {
	t, moved := m.machine.Apply(in)
	if moved {
		s.publish(event.StateChangedOf(m.decl.ID, pluginOf(m.decl), uint64(m.incarnation), t.From, t.To))
		m.status.Since = time.Now()
	}
	m.status.State = m.machine.State()
	condition, has := m.machine.Condition()
	if has && (!m.status.HasCondition || condition != m.status.Condition) {
		m.status.ConditionSince = time.Now()
	}
	m.status.Condition, m.status.HasCondition = condition, has
	if moved && t.To == unit.Running {
		m.readyAt = time.Now()
	}
	if moved && ready(m.decl.Kind, t.To) {
		s.release(m.decl.ID)
	}
	// A terminal state reached while the process lingers: disposing of it follows the conclusion.
	if moved && t.To.Terminal() && m.running != nil {
		// Asked, given the stop window, then ended — as any ending is.
		r, exited, window := m.running, m.exited, s.policy(m).StopWindow
		go func() {
			r.terminate()
			select {
			case <-exited:
			case <-time.After(window):
				r.kill()
			}
		}()
	}
	if moved && t.To.Terminal() && s.cfg.Ended != nil {
		s.cfg.Ended(m.decl.ID)
	}
}

func (s *Supervisor) publish(e event.Event) {
	if s.cfg.Publish != nil {
		s.cfg.Publish(e)
	}
}

// policy is the unit's own figures where it has them, and the deployment's otherwise.
func (s *Supervisor) policy(m *managed) Policy {
	if m.decl.Policy != nil {
		return *m.decl.Policy
	}
	return s.cfg.Policy
}

// settle reads the terminal state the machine reached and schedules the next attempt, if there is one.
// Lock held.
func (s *Supervisor) settle(m *managed) {
	state := m.machine.State()
	if s.stopping || !state.Terminal() || !unit.Restarts(m.decl.Kind, state, m.decl.RestartOnFailure) {
		return
	}
	policy := s.policy(m)
	if !m.readyAt.IsZero() && time.Since(m.readyAt) >= policy.StabilityWindow {
		m.failures = 0
	}
	m.readyAt = time.Time{}
	m.failures++
	wait := policy.Backoff
	for i := 1; i < m.failures && wait < policy.Ceiling; i++ {
		wait *= 2
	}
	if wait > policy.Ceiling {
		wait = policy.Ceiling
	}
	m.status.Waiting, m.status.Attempt, m.status.Wait, m.status.NextAt = true, m.failures, wait, time.Now().Add(wait)
	m.restart = time.AfterFunc(wait, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.stopping {
			s.attempt(m)
		}
	})
}

// Input gives a unit's machine something observed or decided elsewhere: admission, the Session, a report.
func (s *Supervisor) Input(unitID string, in unit.Input) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.units[unitID]; ok {
		s.apply(m, in)
	}
}

// Status is what is observed of a unit now.
func (s *Supervisor) Status(unitID string) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.units[unitID]
	if !ok {
		return Status{}
	}
	status := m.status
	status.Awaiting = append([]string(nil), m.status.Awaiting...)
	status.Backend = backendOf(m.decl)
	if since, quiet := s.quiet[status.Backend]; quiet && m.unobserved {
		// The condition the Core concluded stands in front of whatever the unit last reported, for as long
		// as its cause lasts.
		status.Unobservable, status.UnobservableSince = true, since
		status.Condition, status.HasCondition, status.ConditionSince = unobserved(status.Backend), true, since
	}
	return status
}

// Quiet records that the host's source of facts has gone quiet, and since when. Nothing concludes.
func (s *Supervisor) Quiet(since time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goQuiet(hostBackend, since)
}

// Observable records that the host's source of facts has returned.
func (s *Supervisor) Observable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observable(hostBackend)
}

// goQuiet records that a backend's facts stopped arriving, and since when: each unit running on it keeps
// its state and carries the condition, which is published. A unit with nothing running is not affected.
// Lock held.
func (s *Supervisor) goQuiet(backend string, since time.Time) {
	if _, quiet := s.quiet[backend]; quiet {
		return
	}
	s.quiet[backend] = since
	line := unobserved(backend).Line
	for _, id := range s.order {
		m := s.units[id]
		if backendOf(m.decl) != backend || m.running == nil {
			continue
		}
		m.unobserved = true
		var from *int
		if own, has := m.machine.Condition(); has {
			from = &own.Grade
		}
		s.publish(event.Unobservable(id, uint64(m.incarnation), from, line))
	}
}

// observable records that a backend's facts arrive again: each unit that carried its condition carries its
// own again, or none, and that is published. Lock held.
func (s *Supervisor) observable(backend string) {
	if _, quiet := s.quiet[backend]; !quiet {
		return
	}
	delete(s.quiet, backend)
	for _, id := range s.order {
		m := s.units[id]
		if !m.unobserved {
			continue
		}
		m.unobserved = false
		var own *unit.Condition
		if c, has := m.machine.Condition(); has {
			own = &c
		}
		s.publish(event.Observable(id, uint64(m.incarnation), own))
	}
}

// unreachable is the fault of an operation on a unit whose backend is quiet, naming the backend; nil where
// it is not. Lock held.
func (s *Supervisor) unreachable(m *managed, op string) error {
	backend := backendOf(m.decl)
	if _, quiet := s.quiet[backend]; !quiet {
		return nil
	}
	return fmt.Errorf("the unit %s cannot be %s on the %s backend: %w", m.decl.ID, op, backend, ErrUnreachable)
}

// StopUnit stops one unit: asks, waits the stop window, then ends it.
func (s *Supervisor) StopUnit(unitID string) error {
	s.mu.Lock()
	m, ok := s.units[unitID]
	var fault error
	if ok {
		fault = s.unreachable(m, "stopped")
	}
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no unit %s", unitID)
	}
	if fault != nil {
		return fault
	}
	s.stopOne(m)
	return nil
}

// StartUnit starts a unit that is not running: a new life, launched as any other is. A unit that is
// already live, or waiting on its dependencies, is left as it is.
func (s *Supervisor) StartUnit(unitID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.units[unitID]
	if !ok {
		return fmt.Errorf("no unit %s", unitID)
	}
	if err := s.unreachable(m, "started"); err != nil {
		return err
	}
	if m.held || (m.running != nil && !m.machine.State().Terminal()) {
		return nil
	}
	if m.restart != nil {
		m.restart.Stop()
	}
	m.failures = 0
	if !slices.Contains(s.order, unitID) {
		s.order = append(s.order, unitID)
	}
	s.attempt(m)
	return nil
}

// RestartUnit stops a unit and starts it again: the life that ends and the one that begins are two.
func (s *Supervisor) RestartUnit(unitID string) error {
	if err := s.StopUnit(unitID); err != nil {
		return err
	}
	return s.StartUnit(unitID)
}

// Plugin is the plugin a declared unit runs, empty for a unit of another kind; false for none declared.
func (s *Supervisor) Plugin(unitID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.units[unitID]
	if !ok {
		return "", false
	}
	return m.decl.Plugin, true
}

// IDs are every unit declared, in the order of their identities.
func (s *Supervisor) IDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.units))
	for id := range s.units {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// Kind is a declared unit's kind, empty for none declared.
func (s *Supervisor) Kind(unitID string) unit.Kind {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.units[unitID]; ok {
		return m.decl.Kind
	}
	return ""
}

// Of are the units declared to run a plugin, by identity.
func (s *Supervisor) Of(plugin string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id, m := range s.units {
		if m.decl.Plugin == plugin {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// Stop stops every unit in the reverse of the order they were launched, each within its own window.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	s.stopping = true
	order := append([]string(nil), s.order...)
	for _, m := range s.units {
		if m.restart != nil {
			m.restart.Stop()
		}
		if m.hold != nil {
			m.hold.Stop()
		}
	}
	s.mu.Unlock()

	var failed []error
	for i := len(order) - 1; i >= 0; i-- {
		s.mu.Lock()
		m := s.units[order[i]]
		s.stopOrder = append(s.stopOrder, order[i])
		fault := s.unreachable(m, "stopped")
		s.mu.Unlock()
		if fault != nil {
			failed = append(failed, fault)
			continue
		}
		s.stopOne(m)
	}
	// Nothing is followed once everything that could be stopped has been.
	s.end()
	return errors.Join(failed...)
}

// StopOrder is the order units were asked to stop in.
func (s *Supervisor) StopOrder() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.stopOrder...)
}

func (s *Supervisor) stopOne(m *managed) {
	s.mu.Lock()
	if m.restart != nil {
		m.restart.Stop()
	}
	r, exited, window := m.running, m.exited, s.policy(m).StopWindow
	// A container still being launched is superseded, and removed once it is.
	m.launching++
	if r == nil {
		s.mu.Unlock()
		return
	}
	s.apply(m, unit.StopAsked{})
	s.mu.Unlock()

	// Ask, wait the stop window, then end what did not go.
	r.terminate()
	select {
	case <-exited:
	case <-time.After(window):
		r.kill()
		<-exited
	}
}

// running is an incarnation's process, however it was launched: what ending it asks for.
type running interface {
	terminate()
	kill()
}

// hostProcess is a unit launched on the host, asked through its process group so what it forked goes
// with it.
type hostProcess struct{ pid int }

func (p hostProcess) terminate() { syscall.Kill(-p.pid, syscall.SIGTERM) }
func (p hostProcess) kill()      { syscall.Kill(-p.pid, syscall.SIGKILL) }

// lineWriter hands a unit's output on one line at a time.
type lineWriter struct {
	mu      sync.Mutex
	pending []byte
	emit    func(string)
	closed  bool     // the incarnation is not yet opened: its lines are held, never the writer
	held    []string // the lines written before it was opened, in order
}

// heldUntilOpened is a writer whose lines wait for open: an incarnation speaks only once the change that
// starts it has been told.
func heldUntilOpened(emit func(string)) *lineWriter { return &lineWriter{emit: emit, closed: true} }

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		i := indexByte(w.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.line(string(w.pending[:i]))
		w.pending = w.pending[i+1:]
	}
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		w.line(string(w.pending))
		w.pending = nil
	}
}

// line emits a line, or holds it while the incarnation is not opened. w.mu held.
func (w *lineWriter) line(s string) {
	if w.closed {
		w.held = append(w.held, s)
		return
	}
	w.emit(s)
}

// open emits what was held, in order, and every line after it as it comes.
func (w *lineWriter) open() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = false
	for _, s := range w.held {
		w.emit(s)
	}
	w.held = nil
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// NewCounter counts incarnations in memory, where there is no log store to keep the persistent counter.
func NewCounter() Incarnations { return &counter{next: map[string]int{}} }

type counter struct {
	mu   sync.Mutex
	next map[string]int
}

func (c *counter) Next(unitID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next[unitID]++
	return c.next[unitID]
}

// NewTokens issues random tokens, until admission issues and checks its own.
func NewTokens() Tokens { return tokens{} }

type tokens struct{}

func (tokens) Issue(string) string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
