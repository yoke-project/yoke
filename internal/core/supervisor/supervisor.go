// Package supervisor is the only part of the Core that touches operating-system processes. It launches
// what the deployment declares, learns of each exit as it happens, captures output, and applies the
// restart policy to the terminal state the lifecycle machine concluded.
//
// It is authoritative for whether a process ended and how, and for nothing about what that means. Where
// it cannot observe, it says so and concludes nothing.
package supervisor

import (
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

	"github.com/yoke-project/yoke/internal/core/unit"
)

// Unit is one declared unit, as the composing document gives it.
type Unit struct {
	ID               string
	Kind             unit.Kind
	Plugin           string // the plugin a Plugin unit is a copy of
	Exec             string // the resolved executable
	Digest           string // `sha256:<hex>`, the identity the executable must have; empty where there is none to compare
	Args             []string
	Env              map[string]string
	RestartOnFailure bool     // `restart.on_failure`, a unit that runs to completion's to declare
	DependsOn        []string // the units this one waits on, each until it is ready by its kind
	Policy           *Policy  // the unit's own figures; nil takes the deployment's
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

// Output receives each line a unit writes, with the incarnation that wrote it.
type Output interface {
	Line(unitID string, incarnation int, line string)
}

// Config is what a supervisor is built with.
type Config struct {
	Root         string // the instance root: every path handed to a unit derives from it
	Policy       Policy
	Incarnations Incarnations
	Tokens       Tokens
	Output       Output
	// Ended is told when an incarnation reaches a terminal state, so that what is keyed by a live unit
	// elsewhere can let it go. Optional.
	Ended func(unitID string)
	// NotStarted is told of a unit whose dependency never arrived, with the chain that caused it. Optional.
	NotStarted func(unitID, cause string)
}

// Status is what is observed of a unit: its state, and the facts about its next incarnation.
type Status struct {
	State        unit.State
	Incarnation  int
	PID          int
	Token        string
	Condition    unit.Condition
	HasCondition bool
	Failure      string

	Waiting bool          // between attempts: a fact about the next incarnation, not a state
	Attempt int           // the attempt the unit is on, within this episode
	Wait    time.Duration // how long this wait is
	NextAt  time.Time     // when the next attempt is due

	Unobservable      bool
	UnobservableSince time.Time

	Awaiting   []string // the dependencies not yet ready, while the unit waits to be launched
	NotStarted string   // why the unit was not started, once its window ran out waiting
}

// The backend this supervisor launches on.
const backend = "host"

type managed struct {
	decl        Unit
	machine     *unit.Machine
	status      Status
	process     *os.Process
	exited      chan struct{}
	incarnation int
	failures    int
	readyAt     time.Time
	windowOut   bool
	restart     *time.Timer

	exitStatus int  // the status the last process ended with
	hasExit    bool // whether the last process ended by exiting, rather than never starting
	held       bool // waiting on its dependencies, with no incarnation yet
	hold       *time.Timer
}

// Supervisor supervises the units of one instance.
type Supervisor struct {
	cfg Config

	mu         sync.Mutex
	units      map[string]*managed
	order      []string
	stopOrder  []string
	stopping   bool
	quiet      bool
	quietSince time.Time
	held       []*managed // the units waiting on their dependencies, in the order they were given
}

// New is a supervisor with no unit yet.
func New(cfg Config) *Supervisor {
	return &Supervisor{cfg: cfg, units: map[string]*managed{}}
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
	m.status.Waiting, m.status.Failure, m.windowOut = false, "", false
	m.exitStatus, m.hasExit = 0, false
	m.process, m.exited = nil, nil

	if s.quiet {
		s.failedLaunch(m, fmt.Sprintf("the %s backend cannot be reached", backend))
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

	token := s.cfg.Tokens.Issue(m.decl.ID)
	command := &exec.Cmd{
		Path:        "/proc/self/fd/3",
		Args:        append([]string{m.decl.Exec}, m.decl.Args...),
		Env:         s.environment(m.decl, token),
		ExtraFiles:  []*os.File{file},
		SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
		WaitDelay:   time.Second,
	}
	incarnation := s.cfg.Incarnations.Next(m.decl.ID)
	lines := &lineWriter{emit: func(line string) { s.cfg.Output.Line(m.decl.ID, incarnation, line) }}
	command.Stdout, command.Stderr = lines, lines
	if err := command.Start(); err != nil {
		file.Close()
		s.failedLaunch(m, fmt.Sprintf("the unit could not be started: %v", err))
		return
	}
	file.Close()

	m.incarnation, m.process, m.exited = incarnation, command.Process, make(chan struct{})
	m.status.Incarnation, m.status.PID, m.status.Token = incarnation, command.Process.Pid, token
	s.apply(m, unit.ProcessStarted{})

	exited, window := m.exited, time.AfterFunc(s.policy(m).StartupWindow, func() { s.windowElapsed(m, incarnation) })
	go func() {
		command.Wait()
		lines.flush()
		window.Stop()
		s.ended(m, incarnation, command.ProcessState.ExitCode())
		close(exited)
	}()
}

// environment is what a unit is handed: the reserved variables, then the declaration's own.
func (s *Supervisor) environment(u Unit, token string) []string {
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

// failedLaunch is an attempt that produced no process: an ordinary failed attempt. Lock held.
func (s *Supervisor) failedLaunch(m *managed, why string) {
	m.status.Failure = why
	s.apply(m, unit.LaunchFailed{Reason: why})
	s.settle(m)
}

// windowElapsed ends a unit that is still starting when its window runs out. The machine concludes
// from the exit this causes.
func (s *Supervisor) windowElapsed(m *managed, incarnation int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.incarnation != incarnation || m.machine.State() != unit.Starting || m.process == nil {
		return
	}
	m.windowOut = true
	syscall.Kill(-m.process.Pid, syscall.SIGKILL)
}

// ended is the kernel's report that an incarnation's process is gone.
func (s *Supervisor) ended(m *managed, incarnation, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m.incarnation != incarnation {
		return
	}
	m.process = nil
	m.exitStatus, m.hasExit = status, true
	if m.windowOut {
		s.apply(m, unit.WindowElapsed{})
	} else {
		s.apply(m, unit.ProcessExited{Status: status})
	}
	s.settle(m)
}

// apply gives the machine one input and records what it concluded. Lock held.
func (s *Supervisor) apply(m *managed, in unit.Input) {
	t, moved := m.machine.Apply(in)
	m.status.State = m.machine.State()
	m.status.Condition, m.status.HasCondition = m.machine.Condition()
	if moved && t.To == unit.Running {
		m.readyAt = time.Now()
	}
	if moved && ready(m.decl.Kind, t.To) {
		s.release(m.decl.ID)
	}
	// A terminal state reached while the process lingers: disposing of it follows the conclusion.
	if moved && t.To.Terminal() && m.process != nil {
		// Asked, given the stop window, then ended — as any ending is.
		process, exited, window := m.process, m.exited, s.policy(m).StopWindow
		syscall.Kill(-process.Pid, syscall.SIGTERM)
		go func() {
			select {
			case <-exited:
			case <-time.After(window):
				syscall.Kill(-process.Pid, syscall.SIGKILL)
			}
		}()
	}
	if moved && t.To.Terminal() && s.cfg.Ended != nil {
		s.cfg.Ended(m.decl.ID)
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
	status.Unobservable, status.UnobservableSince = s.quiet, s.quietSince
	if !s.quiet {
		status.UnobservableSince = time.Time{}
	}
	return status
}

// Quiet records that the backend's source of facts has gone quiet, and since when. Nothing concludes.
func (s *Supervisor) Quiet(since time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quiet, s.quietSince = true, since
}

// Observable records that the source of facts has returned.
func (s *Supervisor) Observable() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.quiet, s.quietSince = false, time.Time{}
}

// StopUnit stops one unit: asks, waits the stop window, then ends it.
func (s *Supervisor) StopUnit(unitID string) error {
	s.mu.Lock()
	m, ok := s.units[unitID]
	quiet := s.quiet
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no unit %s", unitID)
	}
	if quiet {
		return fmt.Errorf("the unit %s cannot be stopped: the %s backend cannot be reached", unitID, backend)
	}
	s.stopOne(m)
	return nil
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
		quiet := s.quiet
		s.mu.Unlock()
		if quiet {
			failed = append(failed, fmt.Errorf("the unit %s cannot be stopped: the %s backend cannot be reached", order[i], backend))
			continue
		}
		s.stopOne(m)
	}
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
	process, exited, window := m.process, m.exited, s.policy(m).StopWindow
	if process == nil {
		s.mu.Unlock()
		return
	}
	s.apply(m, unit.StopAsked{})
	s.mu.Unlock()

	// Ask the unit's process group, so what it forked goes with it; then end what does not go.
	syscall.Kill(-process.Pid, syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(window):
		syscall.Kill(-process.Pid, syscall.SIGKILL)
		<-exited
	}
}

// lineWriter hands a unit's output on one line at a time.
type lineWriter struct {
	mu      sync.Mutex
	pending []byte
	emit    func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		i := indexByte(w.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.emit(string(w.pending[:i]))
		w.pending = w.pending[i+1:]
	}
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) > 0 {
		w.emit(string(w.pending))
		w.pending = nil
	}
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// NewCounter counts incarnations in memory, until the log store keeps the persistent counter.
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
