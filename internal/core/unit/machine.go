// Package unit is the lifecycle machine: the seven states a unit's incarnation passes through, concluded
// from what is observed of it and never assigned by the unit.
//
// The machine runs per unit and weighs its inputs in company: system facts outrank the Core's decisions,
// which outrank the unit's own reports. A terminal state ends the incarnation, and the next launch starts
// a new machine. What the state does not say — what to do next — is the restart policy's.
package unit

// Kind is how a deployment expects a unit to behave.
type Kind string

const (
	Plugin    Kind = "plugin"    // long-running, holding a Session
	Oneshot   Kind = "oneshot"   // runs to completion; its success is its exit status
	Interface Kind = "interface" // a managed interface: a process behind a channel
)

// State is where an incarnation is.
type State string

const (
	Starting  State = "Starting"
	Admitted  State = "Admitted"
	Running   State = "Running"
	Completed State = "Completed"
	Stopped   State = "Stopped"
	Failed    State = "Failed"
	Refused   State = "Refused"
)

// Terminal says whether the state ends the incarnation.
func (s State) Terminal() bool {
	return s == Completed || s == Stopped || s == Failed || s == Refused
}

// An Input is something observed of a unit, or decided about it.
type Input interface{ input() }

type (
	ProcessStarted     struct{}                 // a fact: the process is up
	ProcessExited      struct{ Status int }     // a fact: the process ended, with this status
	LaunchFailed       struct{ Reason string }  // a fact: the launch never produced a process
	StopAsked          struct{}                 // a decision: the unit is being stopped
	AdmissionAccepted  struct{}                 // a decision: admitted
	AdmissionDeclined  struct{}                 // a decision: declined at the gate
	AdmissionWithdrawn struct{}                 // a decision: an admission withdrawn afterwards
	SessionOpened      struct{}                 // the Session's first envelope arrived
	SessionEnded       struct{ Withdrawn bool } // the Session ended: withdrawn by decision, or lost
	WindowElapsed      struct{}                 // the startup window ran out
	Reported           struct {                 // the unit's own report: a view, a grade and a line
		State State
		Grade int
		Line  string
	}
)

func (ProcessStarted) input()     {}
func (ProcessExited) input()      {}
func (LaunchFailed) input()       {}
func (StopAsked) input()          {}
func (AdmissionAccepted) input()  {}
func (AdmissionDeclined) input()  {}
func (AdmissionWithdrawn) input() {}
func (SessionOpened) input()      {}
func (SessionEnded) input()       {}
func (WindowElapsed) input()      {}
func (Reported) input()           {}

// A Transition is where the unit came from and where it went.
type Transition struct{ From, To State }

// A Condition is the unit's own account of how well it is: an attribute beside the state, never a position.
type Condition struct {
	Grade int
	Line  string
}

// Machine is one incarnation's machine.
type Machine struct {
	kind      Kind
	state     State
	asked     bool
	condition *Condition
}

// NewMachine is a machine for a unit just launched.
func NewMachine(kind Kind) *Machine { return &Machine{kind: kind, state: Starting} }

func (m *Machine) State() State { return m.state }

// Condition is the unit's last report, if it ever made one: silence is not a claim.
func (m *Machine) Condition() (Condition, bool) {
	if m.condition == nil {
		return Condition{}, false
	}
	return *m.condition, true
}

// Apply weighs one input, returning the transition it produced, if any.
func (m *Machine) Apply(in Input) (Transition, bool) {
	if m.state.Terminal() {
		return Transition{}, false
	}
	from, plugin := m.state, m.kind == Plugin
	switch v := in.(type) {
	case ProcessStarted:
		if !plugin && m.state == Starting {
			m.state = Running
		}
	case ProcessExited:
		switch {
		case m.asked:
			m.state = Stopped
		case m.kind == Oneshot && m.state == Running && v.Status == 0:
			m.state = Completed
		default:
			m.state = Failed
		}
	case LaunchFailed, WindowElapsed:
		if m.state == Starting {
			m.state = Failed
		}
	case StopAsked:
		m.asked = true
	case AdmissionAccepted:
		if plugin && m.state == Starting {
			m.state = Admitted
		}
	case AdmissionDeclined:
		if plugin && m.state == Starting {
			m.state = Refused
		}
	case AdmissionWithdrawn:
		if plugin && (m.state == Admitted || m.state == Running) {
			m.state = Refused
		}
	case SessionOpened:
		if plugin && m.state == Admitted {
			m.state = Running
		}
	case SessionEnded:
		if plugin && m.state == Running {
			switch {
			case m.asked:
				m.state = Stopped
			case v.Withdrawn:
				m.state = Refused
			default:
				m.state = Failed
			}
		}
	case Reported:
		m.condition = &Condition{Grade: v.Grade, Line: v.Line}
	}
	return Transition{From: from, To: m.state}, m.state != from
}

// Restarts says whether a unit of kind that reached state is launched again. Only Failed ever restarts,
// and a unit that runs to completion only when its declaration says it is safe to run twice.
func Restarts(kind Kind, state State, onFailure bool) bool {
	return state == Failed && (kind != Oneshot || onFailure)
}
