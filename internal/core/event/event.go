// Package event is what the Core publishes when it observed something or decided something: eight
// fields, the same on every event, with a subject that is a kind and an identity, an actor the Core
// establishes, the Core's own clock, and a severity on one ordered scale.
//
// The Core grades its own conclusions; a unit's report carries the grade the unit declared, and nobody
// restates it. Every type is declared with its class: a level announces that a current value changed,
// and an edge an occurrence that leaves nothing current behind.
package event

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/unit"
)

// Kind is what a subject is: one of six, closed.
type Kind string

const (
	Instance   Kind = "instance"
	Unit       Kind = "unit"
	Plugin     Kind = "plugin"
	Channel    Kind = "channel"
	Document   Kind = "document"
	Connection Kind = "connection"
)

// Subject is what an event is about. A unit's names one of its lives; zero is a life the Core did not
// launch, whose number it does not know.
type Subject struct {
	Kind        Kind
	ID          string
	Incarnation uint64
}

// ActorClass is who caused an event: one of four, closed, and established by the Core.
type ActorClass string

const (
	ByCore     ActorClass = "core"
	ByOperator ActorClass = "operator"
	ByUnit     ActorClass = "unit"
	ByHost     ActorClass = "host"
)

// Actor is who caused an event; an operator's carries the person the surface established.
type Actor struct {
	Class  ActorClass
	Person string
}

// The four anchors of the scale, placed apart so that values can be inserted between them.
const (
	Routine  = 10
	Notable  = 30
	Serious  = 50
	Critical = 70
)

// Class is how an event is recovered: a level by resynchronising, an edge by reading.
type Class string

const (
	Level Class = "level"
	Edge  Class = "edge"
)

// Event is one conclusion the Core reached.
type Event struct {
	// Seq numbers the event within one life of the instance. It is assigned at publication.
	Seq        uint64
	Type       string
	Subject    Subject
	Time       time.Time
	Actor      Actor
	Severity   int
	Occurrence string
	// Cause is the sequence of the event this one follows from, zero where there is none.
	Cause  uint64
	Detail []byte
}

// declared is every type a producer in this Core emits, with its class and, where the Core grades its
// own conclusion, nothing more: the grading is the constructor's.
var declared = map[string]Class{
	"instance.ready":           Level,
	"instance.stopping":        Level,
	"unit.state.changed":       Level,
	"unit.condition.changed":   Level,
	"unit.occurrence.reported": Edge,
	"document.resolved":        Level,
	"document.rejected":        Edge,
}

// occurrenceType is the one type that carries an occurrence.
const occurrenceType = "unit.occurrence.reported"

// ClassOf is the class a type was declared with, and false for a type nobody declared.
func ClassOf(typ string) (Class, bool) {
	c, ok := declared[typ]
	return c, ok
}

// Check refuses an event that does not fit its eight fields, naming the field.
func (e Event) Check() error {
	switch {
	case e.Type == "":
		return fmt.Errorf("an event has a type")
	case !declaredType(e.Type):
		return fmt.Errorf("the type %s is declared by no producer", e.Type)
	case !slices.Contains([]Kind{Instance, Unit, Plugin, Channel, Document, Connection}, e.Subject.Kind):
		return fmt.Errorf("the subject is of the kind %q, which is none of the six", e.Subject.Kind)
	case e.Subject.ID == "":
		return fmt.Errorf("the subject has an identity")
	case e.Subject.Kind != Unit && e.Subject.Incarnation != 0:
		return fmt.Errorf("the subject is a %s, and only a unit's identity has an incarnation", e.Subject.Kind)
	case !slices.Contains([]ActorClass{ByCore, ByOperator, ByUnit, ByHost}, e.Actor.Class):
		return fmt.Errorf("the actor is of the class %q, which is none of the four", e.Actor.Class)
	case e.Severity < 0 || e.Severity > 99:
		return fmt.Errorf("the severity %d is outside 0–99", e.Severity)
	case e.Time.IsZero():
		return fmt.Errorf("the time is the Core's, and was not taken")
	case e.Type == occurrenceType && e.Occurrence == "":
		return fmt.Errorf("an occurrence reported names its occurrence")
	case e.Type != occurrenceType && e.Occurrence != "":
		return fmt.Errorf("only %s carries an occurrence", occurrenceType)
	}
	return nil
}

func declaredType(typ string) bool {
	_, ok := declared[typ]
	return ok
}

// UnitSubject is a unit's subject, in one of its lives.
func UnitSubject(id string, incarnation uint64) Subject {
	return Subject{Kind: Unit, ID: id, Incarnation: incarnation}
}

// concluded is an event the Core reached on its own, stamped now.
func concluded(typ string, subject Subject, severity int, detail map[string]any) Event {
	e := Event{Type: typ, Subject: subject, Time: time.Now(), Actor: Actor{Class: ByCore}, Severity: severity}
	if detail != nil {
		e.Detail, _ = json.Marshal(detail)
	}
	return e
}

// StateChanged is the Core's conclusion that a unit's life moved between two states. It grades the
// change by where it went: into Failed serious, into Refused notable, and anything else routine.
func StateChanged(unitID string, incarnation uint64, from, to unit.State) Event {
	severity := Routine
	switch to {
	case unit.Failed:
		severity = Serious
	case unit.Refused:
		severity = Notable
	}
	return concluded("unit.state.changed", UnitSubject(unitID, incarnation), severity, map[string]any{"from": from, "to": to})
}

// InstanceReady is the trunk's conclusion that the instance became observable.
func InstanceReady(name string) Event {
	return concluded("instance.ready", Subject{Kind: Instance, ID: name}, Routine, nil)
}

// InstanceStopping is the trunk's conclusion that the instance began to stop.
func InstanceStopping(name string) Event {
	return concluded("instance.stopping", Subject{Kind: Instance, ID: name}, Routine, nil)
}

// OccurrenceReported is a unit's report of an occurrence in its own domain, as the Core carries it: at
// the grade the unit declared, with the class in its own field and the detail as it arrived, stamped
// when the Core received it.
func OccurrenceReported(unitID string, incarnation uint64, r *pluginv1.Event) Event {
	return Event{Type: occurrenceType, Subject: UnitSubject(unitID, incarnation), Time: time.Now(),
		Actor: Actor{Class: ByUnit}, Severity: int(r.GetSeverity()), Occurrence: r.GetOccurrence(), Detail: r.GetDetail()}
}

// ConditionChanged is a unit's report of how well it is, as the Core carries it: at the grade the unit
// reported. from is nil where the unit had not reported before.
func ConditionChanged(unitID string, incarnation uint64, from *int, to int, line string) Event {
	detail := map[string]any{"to": to, "message": line}
	if from != nil {
		detail["from"] = *from
	}
	e := concluded("unit.condition.changed", UnitSubject(unitID, incarnation), to, detail)
	e.Actor = Actor{Class: ByUnit}
	return e
}
