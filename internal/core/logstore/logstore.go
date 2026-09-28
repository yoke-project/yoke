// Package logstore is the store of evidence: what a unit printed, what the Core concluded, and what a
// unit reported, each entry belonging to a unit and one of its lives or to no unit at all.
//
// The store numbers an entry when it stores it, and that number is the one order it keeps: a line
// printed just before a crash may be stored just after it, and it still names the life that printed it.
// Entries are appended in batches. The counter that numbers a unit's lives lives here too, and survives
// the Core. Nothing reads this store to make a decision.
package logstore

import (
	"time"

	"github.com/yoke-project/yoke/internal/core/event"
)

// File is the log store's name in the instance's state directory.
const File = "logs.db"

// Source is where an entry came from.
type Source string

const (
	Stdout   Source = "stdout"
	Stderr   Source = "stderr"
	FromCore Source = "core"
	Reported Source = "reported"
)

// Entry is one record. A zero Incarnation is a life the Core did not count; an empty Unit is an entry
// at Core level, which belongs to no life either.
type Entry struct {
	Seq         uint64
	At          time.Time
	Unit        string
	Incarnation uint64
	Source      Source
	Severity    int
	Type        string
	SubjectKind string
	SubjectID   string
	Actor       string
	Cause       uint64
	Message     string
	Detail      []byte
}

// Store is one instance's log store. The Core is its one writer.
type Store struct{}

// Open opens the log store at path, creating it if absent. A write that fails once it is open is handed
// to report, and costs evidence and nothing else.
func Open(path string, report func(error)) (*Store, error) { return &Store{}, nil }

// Append queues an entry for the next batch. An entry naming a life and no unit is refused.
func (s *Store) Append(e Entry) error { return nil }

// Entries is every entry stored after the one numbered after, in the store's order.
func (s *Store) Entries(after uint64) ([]Entry, error) { return nil, nil }

// Next counts a launch of the unit, and is the number of the life it begins.
func (s *Store) Next(unit string) (uint64, error) { return 0, nil }

// Keep appends an event's durable counterpart. It is called where the event is published and not by a
// subscriber, since a subscriber may be told of an overflow and a record may not be lost.
func (s *Store) Keep(e event.Event) error { return nil }

// Close writes what is queued, and closes the store.
func (s *Store) Close() error { return nil }

// Counterpart is an event's durable counterpart.
func Counterpart(e event.Event) Entry { return Entry{} }

// brk closes the store's connection underneath it, for a test of a write that fails.
func (s *Store) brk() {}
