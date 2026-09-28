// Package event is what the Core publishes when it observed something or decided something: eight
// fields, the same on every event, with a subject that is a kind and an identity, an actor the Core
// establishes, the Core's own clock, and a severity on one ordered scale.
//
// The Core grades its own conclusions; a unit's report carries the grade the unit declared, and nobody
// restates it. Every type is declared with its class: a level announces that a current value changed,
// and an edge an occurrence that leaves nothing current behind.
package event

import (
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

// ClassOf is the class a type was declared with, and false for a type nobody declared.
func ClassOf(typ string) (Class, bool) { return "", false }

// Check refuses an event that does not fit its eight fields, naming the field.
func (e Event) Check() error { return nil }

// UnitSubject is a unit's subject, in one of its lives.
func UnitSubject(id string, incarnation uint64) Subject { return Subject{} }

// StateChanged is the Core's conclusion that a unit's life moved between two states.
func StateChanged(unitID string, incarnation uint64, from, to unit.State) Event { return Event{} }

// InstanceReady is the trunk's conclusion that the instance became observable.
func InstanceReady(name string) Event { return Event{} }

// OccurrenceReported is a unit's report of an occurrence in its own domain, as the Core carries it.
func OccurrenceReported(unitID string, incarnation uint64, r *pluginv1.Event) Event { return Event{} }

// ConditionChanged is a unit's report of how well it is, as the Core carries it; from is nil where the
// unit had not reported before.
func ConditionChanged(unitID string, incarnation uint64, from *int, to int, line string) Event {
	return Event{}
}
