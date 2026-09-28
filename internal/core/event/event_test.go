package event_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
)

func detail(t *testing.T, e event.Event) map[string]any {
	t.Helper()
	var d map[string]any
	if err := json.Unmarshal(e.Detail, &d); err != nil {
		t.Fatalf("the detail of %s is not an object: %v", e.Type, err)
	}
	return d
}

// std: yoke:the-event-envelope.01
func TestAnEventCarriesEightFieldsAndNothingThatDoesNotFit(t *testing.T) {
	good := event.StateChanged("acquire", 1, unit.Starting, unit.Running)
	if err := good.Check(); err != nil {
		t.Fatalf("a well-formed event was refused: %v", err)
	}
	for field, spoil := range map[string]func(e *event.Event){
		"type":       func(e *event.Event) { e.Type = "" },
		"subject":    func(e *event.Event) { e.Subject.Kind = "stream" },
		"identity":   func(e *event.Event) { e.Subject.ID = "" },
		"actor":      func(e *event.Event) { e.Actor.Class = "plugin" },
		"severity":   func(e *event.Event) { e.Severity = 100 },
		"occurrence": func(e *event.Event) { e.Occurrence = "calibration.drift" },
	} {
		e := good
		spoil(&e)
		if err := e.Check(); err == nil {
			t.Errorf("an event with a spoilt %s was accepted", field)
		} else if !bytes.Contains([]byte(err.Error()), []byte(field)) {
			t.Errorf("the refusal of a spoilt %s does not name it: %v", field, err)
		}
	}
	reported := event.OccurrenceReported("acquire", 1, &pluginv1.Event{Occurrence: "calibration.drift", Severity: 40})
	reported.Occurrence = ""
	if err := reported.Check(); err == nil {
		t.Error("an occurrence reported with none was accepted")
	}
}

// std: yoke:the-event-envelope.02
func TestTheSubjectIsAKindAndAnIdentity(t *testing.T) {
	changed := event.StateChanged("station", 3, unit.Starting, unit.Running)
	if want := (event.Subject{Kind: event.Unit, ID: "station", Incarnation: 3}); changed.Subject != want || event.UnitSubject("station", 3) != want {
		t.Errorf("the unit's subject is %+v, want %+v", changed.Subject, want)
	}
	channel := event.Subject{Kind: event.Channel, ID: "station"}
	if channel == changed.Subject || channel.Incarnation != 0 {
		t.Errorf("a channel and a unit sharing a name merged: %+v", channel)
	}
}

// std: yoke:the-event-envelope.03
func TestTheClockIsTheCores(t *testing.T) {
	before := time.Now()
	e := event.OccurrenceReported("acquire", 1, &pluginv1.Event{Occurrence: "calibration.drift", Severity: 40})
	after := time.Now()
	if e.Time.Before(before) || e.Time.After(after) {
		t.Errorf("the event is stamped %v, outside the conclusion's %v–%v", e.Time, before, after)
	}
}

// std: yoke:the-event-envelope.04
func TestTheCoreGradesItsOwnConclusions(t *testing.T) {
	for to, want := range map[unit.State]int{unit.Running: 10, unit.Failed: 50, unit.Refused: 30, unit.Stopped: 10} {
		e := event.StateChanged("acquire", 1, unit.Admitted, to)
		d := detail(t, e)
		if e.Type != "unit.state.changed" || e.Severity != want || e.Actor.Class != event.ByCore || d["from"] != string(unit.Admitted) || d["to"] != string(to) {
			t.Errorf("into %s: %s at %d by %s, detail %v", to, e.Type, e.Severity, e.Actor.Class, d)
		}
	}
	ready := event.InstanceReady("bench")
	if ready.Type != "instance.ready" || ready.Severity != event.Routine || ready.Actor.Class != event.ByCore ||
		ready.Subject != (event.Subject{Kind: event.Instance, ID: "bench"}) {
		t.Errorf("the instance became ready as %+v", ready)
	}
}

// std: yoke:the-event-envelope.05
func TestAGradeAUnitDeclaredIsCarriedUnchanged(t *testing.T) {
	opaque := []byte{0xff, 0x00, 0xfe, 'x'}
	for _, severity := range []uint32{97, 0} {
		e := event.OccurrenceReported("acquire", 2, &pluginv1.Event{Occurrence: "calibration.drift", Severity: severity, Line: "drifting", Detail: opaque})
		if e.Type != "unit.occurrence.reported" || e.Occurrence != "calibration.drift" || e.Severity != int(severity) ||
			e.Actor.Class != event.ByUnit || !bytes.Equal(e.Detail, opaque) || e.Subject != event.UnitSubject("acquire", 2) {
			t.Errorf("an occurrence at %d became %+v", severity, e)
		}
		if err := e.Check(); err != nil {
			t.Errorf("an occurrence at %d was refused: %v", severity, err)
		}
	}
	condition := event.ConditionChanged("acquire", 2, nil, 42, "lamp warming")
	d := detail(t, condition)
	if condition.Type != "unit.condition.changed" || condition.Severity != 42 || condition.Actor.Class != event.ByUnit ||
		d["to"] != float64(42) || d["message"] != "lamp warming" {
		t.Errorf("a condition became %+v, detail %v", condition, d)
	}
	if _, reported := d["from"]; reported {
		t.Errorf("a unit that had not reported has a former grade: %v", d)
	}
}

// std: yoke:the-event-envelope.06
func TestEveryTypeIsDeclaredWithItsClass(t *testing.T) {
	for typ, want := range map[string]event.Class{
		"unit.state.changed": event.Level, "unit.condition.changed": event.Level, "instance.ready": event.Level,
		"instance.stopping": event.Level, "document.resolved": event.Level,
		"unit.occurrence.reported": event.Edge, "document.rejected": event.Edge,
	} {
		if got, declared := event.ClassOf(typ); !declared || got != want {
			t.Errorf("%s is %q, declared %v; want %s", typ, got, declared, want)
		}
	}
	if _, declared := event.ClassOf("unit.lamp.changed"); declared {
		t.Error("a type nobody declared has a class")
	}
	e := event.StateChanged("acquire", 1, unit.Starting, unit.Running)
	e.Type = "unit.lamp.changed"
	if err := e.Check(); err == nil {
		t.Error("an event of an undeclared type was accepted")
	}
}
