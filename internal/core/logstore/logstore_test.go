package logstore_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// reports keeps what a store reported.
type reports struct {
	mu   sync.Mutex
	said []error
}

func (r *reports) report(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.said = append(r.said, err)
}

func (r *reports) all() []error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]error(nil), r.said...)
}

func open(t *testing.T) (*logstore.Store, string, *reports) {
	t.Helper()
	path := filepath.Join(t.TempDir(), logstore.File)
	r := &reports{}
	s, err := logstore.Open(path, r.report)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path, r
}

// stored waits for n entries, and returns them.
func stored(t *testing.T, s *logstore.Store, n int) []logstore.Entry {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		got, err := s.Entries(0)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) >= n || time.Now().After(deadline) {
			if len(got) != n {
				t.Fatalf("%d entries were stored, want %d", len(got), n)
			}
			return got
		}
	}
}

func line(unitID string, incarnation uint64, message string) logstore.Entry {
	return logstore.Entry{At: time.Now(), Unit: unitID, Incarnation: incarnation, Source: logstore.Stdout, Severity: 10, Message: message}
}

// std: yoke:the-log-store.01
func TestAnEntryBelongsToAUnitAndALife(t *testing.T) {
	s, _, _ := open(t)
	for _, e := range []logstore.Entry{line("acquire", 2, "second life"), line("acquire", 0, "started by hand"), line("", 0, "the Core")} {
		if err := s.Append(e); err != nil {
			t.Fatalf("%s was refused: %v", e.Message, err)
		}
	}
	if err := s.Append(line("", 3, "a number with nothing to number")); err == nil {
		t.Error("an entry with a life and no unit was accepted")
	}
	got := stored(t, s, 3)
	for i, want := range []struct {
		unit        string
		incarnation uint64
	}{{"acquire", 2}, {"acquire", 0}, {"", 0}} {
		if got[i].Unit != want.unit || got[i].Incarnation != want.incarnation {
			t.Errorf("entry %d belongs to %q, life %d; want %q, life %d", i+1, got[i].Unit, got[i].Incarnation, want.unit, want.incarnation)
		}
	}
}

// std: yoke:the-log-store.02
func TestEntriesAreNumberedAsTheyAreStored(t *testing.T) {
	s, _, _ := open(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	times := []time.Time{now, now.Add(-time.Hour), now.Add(-2 * time.Hour)}
	for i, at := range times {
		e := line("acquire", 1, []string{"first", "second", "third"}[i])
		e.At = at
		s.Append(e)
	}
	got := stored(t, s, 3)
	for i, e := range got {
		if e.Seq != uint64(i+1) || !e.At.Equal(times[i]) || e.Message != []string{"first", "second", "third"}[i] {
			t.Errorf("entry %d is %d %v %q", i+1, e.Seq, e.At, e.Message)
		}
	}
}

// std: yoke:the-log-store.03
func TestEntriesAreAppendedInBatches(t *testing.T) {
	s, path, _ := open(t)
	other, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	count := func() int {
		var n int
		other.QueryRow("SELECT count(*) FROM entry").Scan(&n)
		return n
	}
	start := time.Now()
	for i := range 64 {
		s.Append(line("acquire", 1, strings.Repeat("x", i)))
	}
	for count() < 64 {
		if time.Since(start) > 100*time.Millisecond {
			t.Fatalf("after a tenth of a second the other connection reads %d of 64", count())
		}
		time.Sleep(5 * time.Millisecond)
	}
	var lengths []int
	rows, _ := other.Query("SELECT length(message) FROM entry ORDER BY seq")
	for rows.Next() {
		var n int
		rows.Scan(&n)
		lengths = append(lengths, n)
	}
	rows.Close()
	if !slices.IsSorted(lengths) || len(lengths) != 64 {
		t.Errorf("the batch was stored out of order: %v", lengths)
	}
	start = time.Now()
	s.Append(line("acquire", 1, "the last"))
	for count() < 65 {
		if time.Since(start) > 250*time.Millisecond {
			t.Fatal("the last entry was not durable within a quarter of a second")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// std: yoke:the-log-store.04
func TestALifeIsCountedOncePerLaunchAndTheCountSurvives(t *testing.T) {
	path := filepath.Join(t.TempDir(), logstore.File)
	s, err := logstore.Open(path, func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		unit string
		n    uint64
	}{{"acquire", 1}, {"acquire", 2}, {"archive", 1}} {
		if got, err := s.Next(want.unit); err != nil || got != want.n {
			t.Errorf("a launch of %s was counted %d (%v), want %d", want.unit, got, err, want.n)
		}
	}
	s.Close()
	s, err = logstore.Open(path, func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if got, _ := s.Next("acquire"); got != 3 {
		t.Errorf("after reopening, acquire's next life is %d, want 3", got)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var counted []uint64
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := s.Next("probe")
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			counted = append(counted, n)
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.Sort(counted)
	for i, n := range counted {
		if n != uint64(i+1) {
			t.Fatalf("twenty launches at once were counted %v", counted)
		}
	}
}

// std: yoke:the-log-store.05
func TestTheCoresEventsReachTheStoreAsTheirCounterparts(t *testing.T) {
	s, _, _ := open(t)
	b := bus.New()
	publish := func(e event.Event) event.Event {
		t.Helper()
		published, err := b.Publish(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Keep(published); err != nil {
			t.Fatal(err)
		}
		return published
	}
	earlier := publish(event.StateChanged("acquire", 1, unit.Admitted, unit.Running))
	failed := event.StateChanged("acquire", 1, unit.Running, unit.Failed)
	failed.Cause = earlier.Seq
	publish(failed)
	publish(event.InstanceReady("bench"))
	publish(event.OccurrenceReported("acquire", 1, &pluginv1.Event{Occurrence: "calibration.drift", Severity: 97, Line: "drifting", Detail: []byte{0xff, 1}}))
	got := stored(t, s, 4)[1:]

	change := got[0]
	var detail map[string]any
	json.Unmarshal(change.Detail, &detail)
	if change.Source != logstore.FromCore || change.Unit != "acquire" || change.Incarnation != 1 || change.Type != "unit.state.changed" ||
		change.SubjectKind != "unit" || change.SubjectID != "acquire" || change.Actor != "core" || change.Severity != 50 ||
		change.Cause != earlier.Seq || detail["to"] != "Failed" || change.Message == "" {
		t.Errorf("the state change was stored as %+v", change)
	}
	ready := got[1]
	if ready.Source != logstore.FromCore || ready.Unit != "" || ready.Incarnation != 0 || ready.Type != "instance.ready" ||
		ready.SubjectKind != "instance" || ready.SubjectID != "bench" {
		t.Errorf("the readiness was stored as %+v", ready)
	}
	report := got[2]
	if report.Source != logstore.Reported || report.Unit != "acquire" || report.Severity != 97 || report.Actor != "unit" ||
		report.Type != "unit.occurrence.reported" || report.Message != "drifting" || !bytes.Equal(report.Detail, []byte{0xff, 1}) {
		t.Errorf("the report was stored as %+v", report)
	}
}

// std: yoke:the-log-store.06
func TestAWriteThatFailsIsReportedAndNotFatal(t *testing.T) {
	s, _, r := open(t)
	logstore.Break(s)
	done := make(chan error, 1)
	go func() { done <- s.Append(line("acquire", 1, "lost")) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("appending did not return")
	}
	for deadline := time.Now().Add(time.Second); len(r.all()) == 0; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the failed write was not reported")
		}
	}
	if err := r.all()[0]; err == nil || !strings.Contains(err.Error(), "log store") {
		t.Errorf("the report does not name the store: %v", err)
	}
	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("the store could not be closed")
	}
}

// std: yoke:names-and-filtering.05
func TestEveryDeclaredTypeIsKeptAsAnEntry(t *testing.T) {
	one := map[string]event.Event{
		"instance.ready":             event.InstanceReady("bench"),
		"instance.stopping":          event.InstanceStopping("bench"),
		"unit.state.changed":         event.StateChanged("acquire", 2, unit.Starting, unit.Running),
		"unit.condition.changed":     event.ConditionChanged("acquire", 2, nil, 90, "warm"),
		"unit.occurrence.reported":   event.OccurrenceReported("acquire", 2, &pluginv1.Event{Occurrence: "calibration.drift", Severity: 40}),
		"unit.stream.activated":      event.StreamActivated("acquire", 2, "station.spectra"),
		"unit.stream.stopped":        event.StreamStopped("acquire", 2, "station.spectra", "asked", true),
		"channel.attached":           event.ChannelAttached("panel", "davide"),
		"channel.suspended":          event.ChannelSuspended("panel", "displaced", "bench", "read-only"),
		"channel.resumed":            event.ChannelResumed("panel"),
		"channel.detached":           event.ChannelDetached("panel", "davide", "closed"),
		"channel.subscription.stale": event.SubscriptionStale("panel", time.Unix(1000, 0)),
		"document.resolved":          event.DocumentResolved("/etc/yoke/bench.yaml", "bench", "sha256:00"),
		"document.rejected":          event.DocumentRejected("/etc/yoke/bad.yaml", "yaml.syntax", "line 3"),
		"connection.opened":          event.ConnectionOpened("c-1", "shell", event.Actor{Class: event.ByOperator, Person: "ada"}),
		"connection.closed":          event.ConnectionClosed("c-1", "shell", "cancelled", event.Actor{Class: event.ByOperator, Person: "ada"}),
	}
	for _, name := range event.Names() {
		if name == event.InRegistry {
			// Its counterpart is the Registry's decision row, and nothing is kept here.
			continue
		}
		e, made := one[name]
		if !made {
			t.Errorf("%s is declared, and this case makes none", name)
			continue
		}
		entry := logstore.Counterpart(e)
		wantSource := logstore.FromCore
		if name == "unit.occurrence.reported" {
			wantSource = logstore.Reported
		}
		unitLife := e.Subject.Kind == event.Unit
		if entry.Type != name || entry.SubjectKind != string(e.Subject.Kind) || entry.SubjectID != e.Subject.ID || entry.Source != wantSource ||
			(unitLife && (entry.Unit != "acquire" || entry.Incarnation != 2)) || (!unitLife && entry.Unit != "") {
			t.Errorf("%s is kept as %+v", name, entry)
		}
	}
}
