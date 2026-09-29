// Package logstore is the store of evidence: what a unit printed, what the Core concluded, and what a
// unit reported, each entry belonging to a unit and one of its lives or to no unit at all.
//
// The store numbers an entry when it stores it, and that number is the one order it keeps: a line
// printed just before a crash may be stored just after it, and it still names the life that printed it.
// Entries are appended in batches. The counter that numbers a unit's lives lives here too, and survives
// the Core. Nothing reads this store to make a decision.
package logstore

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yoke-project/yoke/internal/core/event"
)

// File is the log store's name in the instance's state directory.
const File = "logs.db"

const driver = "sqlite"

// A batch is written when this many entries have accumulated, or when its first has waited this long.
const (
	batchEntries = 64
	batchWait    = 100 * time.Millisecond
)

// Times are stored at one width, so that their text sorts as they do.
const stamp = "2006-01-02T15:04:05.000000000Z"

// steps lead to the schema this Core implements; a file's number is how many it has had applied.
var steps = []string{
	`CREATE TABLE entry (
		seq          INTEGER PRIMARY KEY,
		at           TEXT NOT NULL,
		unit         TEXT,
		incarnation  INTEGER,
		source       TEXT NOT NULL CHECK (source IN ('stdout', 'stderr', 'core', 'reported')),
		severity     INTEGER NOT NULL CHECK (severity BETWEEN 0 AND 99),
		type         TEXT,
		subject_kind TEXT,
		subject_id   TEXT,
		actor        TEXT,
		cause        INTEGER,
		message      TEXT NOT NULL,
		detail       BLOB,
		CHECK (unit IS NOT NULL OR incarnation IS NULL)
	);
	CREATE INDEX entry_by_life ON entry (unit, incarnation, seq);
	CREATE INDEX entry_by_time ON entry (at);
	CREATE TABLE incarnation (
		unit TEXT PRIMARY KEY,
		last INTEGER NOT NULL
	);
	CREATE TABLE retention_policy (
		unit    TEXT PRIMARY KEY,
		age     INTEGER,
		bytes   INTEGER,
		entries INTEGER
	);`,
}

// Schema is the number of the schema this Core implements.
func Schema() int { return len(steps) }

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
type Store struct {
	db     *sql.DB
	path   string
	report func(error)

	mu      sync.Mutex
	closed  bool
	queue   chan Entry
	written chan struct{}
}

// Open opens the log store at path, creating it if absent, and migrates it forward. A file written by a
// newer Core is refused and left untouched. A write that fails once it is open is handed to report, and
// costs evidence and nothing else.
func Open(path string, report func(error)) (*Store, error) {
	if err := readable(path, len(steps)); err != nil {
		return nil, err
	}
	// Evidence is worth a power cut's loss of the last batch, and a reader never blocks the writer.
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	// One connection: the writer, the counter and the reads take turns rather than contend.
	db.SetMaxOpenConns(1)
	if err := migrate(db, path); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, path: path, report: report, queue: make(chan Entry, 4096), written: make(chan struct{})}
	go s.write()
	return s, nil
}

func migrate(db *sql.DB, path string) error {
	var at int
	if err := db.QueryRow("PRAGMA user_version").Scan(&at); err != nil {
		return fmt.Errorf("the log store %s cannot be read: %w", path, err)
	}
	if at > len(steps) {
		return newer(path, at)
	}
	for n := at; n < len(steps); n++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(steps[n]); err != nil {
			tx.Rollback()
			return fmt.Errorf("the log store %s: step %d: %w", path, n+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", n+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// readable refuses a file already at a number above this Core's, reading it without writing.
func readable(path string, implements int) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	db, err := sql.Open(driver, "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var at int
	if err := db.QueryRow("PRAGMA user_version").Scan(&at); err != nil {
		return fmt.Errorf("the log store %s cannot be read: %w", path, err)
	}
	if at > implements {
		return newer(path, at)
	}
	return nil
}

func newer(path string, at int) error {
	return fmt.Errorf("the log store %s is at schema %d, and this Core implements %d: a store written by a newer Core is not read", path, at, len(steps))
}

// ErrClosed is what Append answers once the store is closed.
var ErrClosed = errors.New("the log store is closed")

// Append queues an entry for the next batch. An entry naming a life and no unit is refused: a number
// with nothing to number.
func (s *Store) Append(e Entry) error {
	if e.Unit == "" && e.Incarnation != 0 {
		return fmt.Errorf("an entry names the life %d of no unit", e.Incarnation)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	s.queue <- e
	return nil
}

// write takes entries off the queue in batches, each written in one transaction in the order queued.
func (s *Store) write() {
	defer close(s.written)
	for first := range s.queue {
		batch := []Entry{first}
		wait := time.NewTimer(batchWait)
	gather:
		for len(batch) < batchEntries {
			select {
			case e, open := <-s.queue:
				if !open {
					break gather
				}
				batch = append(batch, e)
			case <-wait.C:
				break gather
			}
		}
		wait.Stop()
		if err := s.store(batch); err != nil {
			s.report(fmt.Errorf("the log store %s lost %d entries: %w", s.path, len(batch), err))
		}
	}
}

func (s *Store) store(batch []Entry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	for _, e := range batch {
		if _, err := tx.Exec(`INSERT INTO entry (at, unit, incarnation, source, severity, type, subject_kind, subject_id, actor, cause, message, detail)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.At.UTC().Format(stamp), null(e.Unit), nullNumber(e.Incarnation), string(e.Source), e.Severity, null(e.Type),
			null(e.SubjectKind), null(e.SubjectID), null(e.Actor), nullNumber(e.Cause), e.Message, e.Detail); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func null(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullNumber(n uint64) any {
	if n == 0 {
		return nil
	}
	return int64(n)
}

// Entries is every entry stored after the one numbered after, in the store's order.
func (s *Store) Entries(after uint64) ([]Entry, error) {
	rows, err := s.db.Query(`SELECT seq, at, coalesce(unit, ''), coalesce(incarnation, 0), source, severity, coalesce(type, ''),
		coalesce(subject_kind, ''), coalesce(subject_id, ''), coalesce(actor, ''), coalesce(cause, 0), message, detail
		FROM entry WHERE seq > ? ORDER BY seq`, int64(after))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []Entry
	for rows.Next() {
		var e Entry
		var at, source string
		if err := rows.Scan(&e.Seq, &at, &e.Unit, &e.Incarnation, &source, &e.Severity, &e.Type, &e.SubjectKind, &e.SubjectID,
			&e.Actor, &e.Cause, &e.Message, &e.Detail); err != nil {
			return nil, err
		}
		e.At, _ = time.Parse(stamp, at)
		e.Source = Source(source)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Next counts a launch of the unit, in one statement, and is the number of the life it begins.
func (s *Store) Next(unit string) (uint64, error) {
	var last uint64
	err := s.db.QueryRow(`INSERT INTO incarnation (unit, last) VALUES (?, 1)
		ON CONFLICT (unit) DO UPDATE SET last = last + 1 RETURNING last`, unit).Scan(&last)
	return last, err
}

// Keep appends an event's durable counterpart. It is called where the event is published and not by a
// subscriber, since a subscriber may be told of an overflow and a record may not be lost.
func (s *Store) Keep(e event.Event) error { return s.Append(Counterpart(e)) }

// Close writes what is queued, and closes the store.
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.queue)
	s.mu.Unlock()
	<-s.written
	return s.db.Close()
}

// Retention is a unit's retention override: each limit absent is unconstrained.
type Retention struct {
	Age     time.Duration // zero is unconstrained
	Bytes   *uint64
	Entries *uint64
}

// Override is a unit's retention override, and false where it has none.
func (s *Store) Override(unit string) (Retention, bool, error) { return Retention{}, false, nil }

// Counterpart is an event's durable counterpart: attributed to the unit it is about, or to no unit; a
// unit's report under the source that says so.
func Counterpart(e event.Event) Entry {
	entry := Entry{At: e.Time, Source: FromCore, Severity: e.Severity, Type: e.Type, SubjectKind: string(e.Subject.Kind),
		SubjectID: e.Subject.ID, Actor: actor(e.Actor), Cause: e.Cause, Detail: e.Detail, Message: sentence(e)}
	if e.Subject.Kind == event.Unit {
		entry.Unit, entry.Incarnation = e.Subject.ID, e.Subject.Incarnation
	}
	if e.Occurrence != "" {
		entry.Source = Reported
	}
	return entry
}

// actor is who caused an event as the column keeps it: the class, and the person where the channel
// established one.
func actor(a event.Actor) string {
	if a.Person != "" {
		return string(a.Class) + ":" + a.Person
	}
	return string(a.Class)
}

// sentence is what the Core concluded, for a person: a unit's report is its own line, and anything else
// the type and what it is about.
func sentence(e event.Event) string {
	if e.Line != "" {
		return e.Line
	}
	return fmt.Sprintf("%s %s:%s", e.Type, e.Subject.Kind, e.Subject.ID)
}

// brk closes the store's connection underneath it, for a test of a write that fails.
func (s *Store) brk() { s.db.Close() }
