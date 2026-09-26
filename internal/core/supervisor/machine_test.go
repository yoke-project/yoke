package supervisor_test

import (
	"testing"

	"github.com/yoke-project/yoke/internal/core/unit"
)

// through applies the inputs in order and returns the state reached.
func through(kind unit.Kind, inputs ...unit.Input) unit.State {
	m := unit.NewMachine(kind)
	for _, in := range inputs {
		m.Apply(in)
	}
	return m.State()
}

// std: yoke:the-supervisor.01
func TestAUnitsKindDecidesWhichStatesItCanReach(t *testing.T) {
	cases := []struct {
		kind   unit.Kind
		inputs []unit.Input
		want   unit.State
	}{
		{unit.Plugin, nil, unit.Starting},
		{unit.Plugin, []unit.Input{unit.ProcessStarted{}, unit.AdmissionAccepted{}}, unit.Admitted},
		{unit.Plugin, []unit.Input{unit.ProcessStarted{}, unit.AdmissionAccepted{}, unit.SessionOpened{}}, unit.Running},
		{unit.Plugin, []unit.Input{unit.ProcessStarted{}, unit.AdmissionDeclined{}}, unit.Refused},
		{unit.Plugin, []unit.Input{unit.ProcessStarted{}}, unit.Starting},
		{unit.Oneshot, []unit.Input{unit.ProcessStarted{}}, unit.Running},
		{unit.Oneshot, []unit.Input{unit.ProcessStarted{}, unit.ProcessExited{Status: 0}}, unit.Completed},
		{unit.Interface, []unit.Input{unit.ProcessStarted{}}, unit.Running},
		{unit.Oneshot, []unit.Input{unit.ProcessStarted{}, unit.AdmissionAccepted{}}, unit.Running},
		{unit.Interface, []unit.Input{unit.ProcessStarted{}, unit.AdmissionDeclined{}}, unit.Running},
		{unit.Plugin, []unit.Input{unit.ProcessStarted{}, unit.WindowElapsed{}}, unit.Failed},
	}
	for _, c := range cases {
		if got := through(c.kind, c.inputs...); got != c.want {
			t.Errorf("%s after %v is %s, want %s", c.kind, c.inputs, got, c.want)
		}
	}
}

// running is the inputs that bring a unit of kind to Running.
func running(kind unit.Kind) []unit.Input {
	if kind == unit.Plugin {
		return []unit.Input{unit.ProcessStarted{}, unit.AdmissionAccepted{}, unit.SessionOpened{}}
	}
	return []unit.Input{unit.ProcessStarted{}}
}

var kinds = []unit.Kind{unit.Plugin, unit.Oneshot, unit.Interface}

// std: yoke:the-supervisor.02
func TestAnExitIsCompletedOnlyForAUnitThatRunsToCompletion(t *testing.T) {
	for _, kind := range kinds {
		for status, want := range map[int]unit.State{0: unit.Failed, 1: unit.Failed} {
			if kind == unit.Oneshot && status == 0 {
				want = unit.Completed
			}
			if got := through(kind, append(running(kind), unit.ProcessExited{Status: status})...); got != want {
				t.Errorf("a running %s exiting %d is %s, want %s", kind, status, got, want)
			}
		}
	}
}

// std: yoke:the-supervisor.03
func TestAUnitAskedToStopHasStopped(t *testing.T) {
	for _, kind := range kinds {
		if got := through(kind, append(running(kind), unit.StopAsked{}, unit.ProcessExited{Status: 1})...); got != unit.Stopped {
			t.Errorf("a %s asked to stop is %s, want Stopped", kind, got)
		}
	}
}

// std: yoke:the-supervisor.04
func TestADeploymentsNoIsRefusedAndALostSessionIsFailed(t *testing.T) {
	started := []unit.Input{unit.ProcessStarted{}}
	admitted := []unit.Input{unit.ProcessStarted{}, unit.AdmissionAccepted{}}
	live := running(unit.Plugin)
	cases := []struct {
		name   string
		inputs []unit.Input
		want   unit.State
	}{
		{"declined at the gate", append(started, unit.AdmissionDeclined{}), unit.Refused},
		{"withdrawn while admitted", append(admitted, unit.AdmissionWithdrawn{}), unit.Refused},
		{"withdrawn while running", append(live, unit.AdmissionWithdrawn{}), unit.Refused},
		{"a Session ended by decision", append(live, unit.SessionEnded{Withdrawn: true}), unit.Refused},
		{"a Session lost", append(live, unit.SessionEnded{Withdrawn: false}), unit.Failed},
		{"dead while admitted", append(admitted, unit.ProcessExited{Status: 0}), unit.Failed},
	}
	for _, c := range cases {
		if got := through(unit.Plugin, c.inputs...); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// std: yoke:the-supervisor.05
func TestATerminalStateEndsTheIncarnation(t *testing.T) {
	for _, kind := range kinds {
		ended := append(running(kind), unit.ProcessExited{Status: 1})
		want := through(kind, ended...)
		later := append(ended, unit.Reported{State: unit.Running}, unit.AdmissionAccepted{}, unit.SessionOpened{})
		if got := through(kind, later...); got != want {
			t.Errorf("a %s that ended %s moved to %s", kind, want, got)
		}
	}
}

// std: yoke:the-supervisor.06
func TestAReportCarriesAConditionAndNeverMovesTheState(t *testing.T) {
	m := unit.NewMachine(unit.Plugin)
	for _, in := range running(unit.Plugin) {
		m.Apply(in)
	}
	if _, has := m.Condition(); has {
		t.Error("a unit that never reported has a condition")
	}
	m.Apply(unit.Reported{State: unit.Running, Grade: 90, Line: "the lamp is ageing"})
	if m.State() != unit.Running {
		t.Errorf("a report moved the unit to %s", m.State())
	}
	condition, has := m.Condition()
	if !has || condition.Grade != 90 || condition.Line != "the lamp is ageing" {
		t.Errorf("the condition is %+v (%v), want grade 90 and its line", condition, has)
	}
}

// std: yoke:the-supervisor.07
func TestTheRestartPolicyReadsTheTerminalStateAndTheKind(t *testing.T) {
	for _, kind := range kinds {
		for _, onFailure := range []bool{false, true} {
			for _, state := range []unit.State{unit.Failed, unit.Stopped, unit.Completed, unit.Refused} {
				want := state == unit.Failed && (kind != unit.Oneshot || onFailure)
				if got := unit.Restarts(kind, state, onFailure); got != want {
					t.Errorf("%s %s with on_failure %v: restarts %v, want %v", kind, state, onFailure, got, want)
				}
			}
		}
	}
}
