package admin_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"

	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/unit"
)

func readOf(kind, identity string) *administrativev1.Request {
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_Read{Read: &administrativev1.Read{Kind: kind, Identity: identity}}})
}

func query(q *administrativev1.LogQuery) *administrativev1.Request {
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_LogQuery{LogQuery: q}})
}

// records reads, and fails the test on a refusal.
func (b *bench) records(t *testing.T, kind, identity string) []*administrativev1.Record {
	t.Helper()
	resp, ref := b.call(t, readOf(kind, identity))
	if ref != nil {
		t.Fatalf("reading %s %q was refused %v", kind, identity, ref)
	}
	return resp.GetRead().GetRecords()
}

// appended appends entries and waits for the store to hold n in all.
func appended(t *testing.T, logs *logstore.Store, n int, add ...logstore.Entry) {
	t.Helper()
	for _, e := range add {
		if e.At.IsZero() {
			e.At = time.Now()
		}
		if err := logs.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	entries(t, logs, func(all []logstore.Entry) bool { return len(all) >= n })
}

func many(n int, e logstore.Entry) []logstore.Entry {
	out := make([]logstore.Entry, n)
	for i := range out {
		out[i] = e
	}
	return out
}

// std: yoke:reads-and-the-log.01
func TestAReadNamesAKindAndListsIt(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"archive": running(station, 1), "acquire": running(station, 1)}, map[string]string{})
	var ids []string
	for _, r := range b.records(t, "unit", "") {
		ids = append(ids, r.GetUnit().GetDeclared().GetIdentity())
	}
	if !slices.Equal(ids, []string{"acquire", "archive"}) {
		t.Errorf("the kind unit answered %v, want acquire and archive", ids)
	}
	if got := b.records(t, "unit", "acquire"); len(got) != 1 || got[0].GetUnit().GetDeclared().GetIdentity() != "acquire" {
		t.Errorf("the unit acquire answered %v", got)
	}
	if _, ref := b.call(t, readOf("unit", "nobody")); ref.GetCode() != "subject.unknown" ||
		ref.GetSubject().GetKind() != "unit" || ref.GetSubject().GetIdentity() != "nobody" {
		t.Errorf("the unit nobody was answered %v", ref)
	}
	if _, ref := b.call(t, readOf("stream", "")); ref.GetCode() != "operation.malformed" {
		t.Errorf("the kind stream was answered %v", ref)
	}
	if got := b.records(t, "channel", ""); len(got) != 0 {
		t.Errorf("the kind channel answered %v, want no record", got)
	}
}

// std: yoke:reads-and-the-log.02
func TestAUnitsRecordIsItsDeclarationAndWhatIsObserved(t *testing.T) {
	since := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	b := newBench(t, map[string]*fakeUnit{"acquire": {plugin: station, state: unit.Running, incarnation: 3, since: since,
		condition: &unit.Condition{Grade: 80, Line: "warm"}}}, map[string]string{})
	got := b.records(t, "unit", "acquire")
	if len(got) != 1 {
		t.Fatalf("the read answered %v", got)
	}
	u := got[0].GetUnit()
	d, o := u.GetDeclared(), u.GetObserved()
	if d.GetIdentity() != "acquire" || d.GetKind() != "plugin" || d.GetBackend() != "host" || d.GetPlugin() != station {
		t.Errorf("the declared group is %v", d)
	}
	if o.GetState() != "Running" || o.GetIncarnation() != 3 || !o.GetSince().AsTime().Equal(since) ||
		o.GetCondition().GetGrade() != 80 || o.GetCondition().GetLine() != "warm" {
		t.Errorf("the observed group is %v", o)
	}
	if u.GetPlugin().GetDeclared().GetIdentity() != station {
		t.Errorf("the plugin's record beside it is %v", u.GetPlugin())
	}
}

// std: yoke:reads-and-the-log.03
func TestAPluginsRecordMarksItsThreeClasses(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	b.core.Registry.Record(station, registry.Registration{Version: "0.0.1", Language: "go", SDK: "yoke-sdk-go 0.2.0"})
	b.core.Registry.Grant(station, "stream.data.publish", "someone")
	b.core.Composed = func(id string) bool { return id == station }
	got := b.records(t, "plugin", station)
	if len(got) != 1 {
		t.Fatalf("the read answered %v", got)
	}
	p := got[0].GetPlugin()
	d, a, o := p.GetDeclared(), p.GetAuthorized(), p.GetObserved()
	if d.GetIdentity() != station || d.GetProtocol() != 1 || d.GetManifestDigest() != "sha256:00" || d.GetVersion() != "0.0.1" ||
		d.GetLanguage() != "go" || d.GetSdkLine() != "yoke-sdk-go 0.2.0" {
		t.Errorf("declared is %v", d)
	}
	if !a.GetEnabled() || !slices.Equal(a.GetGranted(), []string{"stream.data.publish"}) || a.GetCredentialMode() != "bootstrap" {
		t.Errorf("authorized is %v", a)
	}
	if !o.GetManifestPresent() || !o.GetComposed() || !slices.Equal(o.GetRunning(), []string{"acquire"}) {
		t.Errorf("observed is %v", o)
	}
}

// std: yoke:reads-and-the-log.04
func TestTheInstanceItsDocumentsAndConnectionsAreRead(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{}, map[string]string{})
	b.core.Instance = func() *administrativev1.InstanceRecord {
		return &administrativev1.InstanceRecord{Identity: "bench", Form: "service", Ready: true}
	}
	b.core.Documents = func() []*administrativev1.DocumentRecord {
		return []*administrativev1.DocumentRecord{{Path: "/etc/yoke/bench.yaml", Resolved: "2 units", Digest: "sha256:01"}}
	}
	if got := b.records(t, "instance", ""); len(got) != 1 || got[0].GetInstance().GetIdentity() != "bench" || !got[0].GetInstance().GetReady() {
		t.Errorf("the instance answered %v", got)
	}
	if got := b.records(t, "document", ""); len(got) != 1 || got[0].GetDocument().GetPath() != "/etc/yoke/bench.yaml" ||
		got[0].GetDocument().GetResolved() != "2 units" || got[0].GetDocument().GetDigest() != "sha256:01" {
		t.Errorf("the documents answered %v", got)
	}
	_, opening, _ := opened(t, b.shell)
	got := b.records(t, "connection", "")
	if len(got) != 1 {
		t.Fatalf("the connections answered %v, want the one open", got)
	}
	c := got[0].GetConnection()
	if c.GetIdentity() != opening.GetConnection() || c.GetProjection() != "shell" || c.GetActor().GetPerson() != me(t).Username || c.GetOpened() == nil {
		t.Errorf("the connection is %v", c)
	}
}

// std: yoke:reads-and-the-log.05
func TestLogQueryPagesByACursorAndResumes(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 2), "archive": running(station, 1)}, map[string]string{})
	var add []logstore.Entry
	add = append(add, many(600, logstore.Entry{Unit: "acquire", Incarnation: 1, Source: logstore.FromCore, Severity: 10, Message: "one"})...)
	add = append(add, many(5, logstore.Entry{Unit: "acquire", Incarnation: 2, Source: logstore.FromCore, Severity: 50, Message: "two"})...)
	add = append(add, many(3, logstore.Entry{Unit: "archive", Incarnation: 1, Source: logstore.FromCore, Severity: 10, Message: "other"})...)
	appended(t, b.core.Logs, 608, add...)

	page := func(q *administrativev1.LogQuery) *administrativev1.LogPage {
		t.Helper()
		resp, ref := b.call(t, query(q))
		if ref != nil {
			t.Fatalf("the query %v was refused %v", q, ref)
		}
		return resp.GetLogQuery()
	}
	first := page(&administrativev1.LogQuery{Unit: "acquire"})
	if n := len(first.GetEntries()); n != 500 || first.GetNext() != first.GetEntries()[499].GetSeq() {
		t.Fatalf("the first page holds %d entries and the cursor %d", n, first.GetNext())
	}
	for i := 1; i < 500; i++ {
		if first.GetEntries()[i].GetSeq() <= first.GetEntries()[i-1].GetSeq() || first.GetEntries()[i].GetUnit() != "acquire" {
			t.Fatalf("the page is out of order, or holds another unit's, at %d", i)
		}
	}
	if second := page(&administrativev1.LogQuery{Unit: "acquire", Cursor: first.GetNext()}); len(second.GetEntries()) != 105 {
		t.Errorf("the second page holds %d entries, want 105", len(second.GetEntries()))
	}
	if floor := page(&administrativev1.LogQuery{Unit: "acquire", Floor: 50}); len(floor.GetEntries()) != 5 {
		t.Errorf("the floor answered %d entries, want 5", len(floor.GetEntries()))
	}
	if life := page(&administrativev1.LogQuery{Unit: "acquire", Incarnation: 2}); len(life.GetEntries()) != 5 {
		t.Errorf("the life 2 answered %d entries, want 5", len(life.GetEntries()))
	}

	removed := first.GetEntries()[9].GetSeq()
	db, err := sql.Open("sqlite", filepath.Join(b.dir, logstore.File))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DELETE FROM entry WHERE seq = ?`, removed); err != nil {
		t.Fatal(err)
	}
	resumed := page(&administrativev1.LogQuery{Unit: "acquire", Cursor: removed})
	if !resumed.GetResumed() || len(resumed.GetEntries()) == 0 || resumed.GetEntries()[0].GetSeq() != removed+1 {
		t.Errorf("from the removed cursor the page resumed %v at %v", resumed.GetResumed(), resumed.GetEntries()[:1])
	}
}

// std: yoke:reads-and-the-log.06
func TestLogFollowPushesAndSaysWhereItFellBehind(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1), "archive": running(station, 1)}, map[string]string{})
	appended(t, b.core.Logs, 3, many(3, logstore.Entry{Unit: "acquire", Incarnation: 1, Source: logstore.FromCore, Severity: 10, Message: "before"})...)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	follow := v1(&administrativev1.Request{Operation: &administrativev1.Request_LogFollow{LogFollow: &administrativev1.LogFollow{Unit: "acquire"}}})
	stream, err := b.operator.Watch(ctx, follow)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	appended(t, b.core.Logs, 5,
		logstore.Entry{Unit: "acquire", Incarnation: 1, Source: logstore.FromCore, Severity: 10, Message: "now"},
		logstore.Entry{Unit: "archive", Incarnation: 1, Source: logstore.FromCore, Severity: 10, Message: "elsewhere"})
	got, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if got.GetLogFollow().GetEntry().GetMessage() != "now" {
		t.Fatalf("the follow first pushed %v, want the entry written after it began", got)
	}
	last := got.GetLogFollow().GetEntry().GetSeq()

	long := strings.Repeat("x", 1024)
	appended(t, b.core.Logs, 605, many(600, logstore.Entry{Unit: "acquire", Incarnation: 1, Source: logstore.FromCore, Severity: 10, Message: long})...)
	time.Sleep(500 * time.Millisecond)
	var pushed []uint64
	var behind uint64
	for behind == 0 {
		got, err := stream.Recv()
		if err != nil {
			t.Fatalf("the follow ended before it said it fell behind: %v, after %d entries", err, len(pushed))
		}
		if e := got.GetLogFollow().GetEntry(); e != nil {
			if e.GetMessage() == "elsewhere" {
				t.Error("the follow pushed another unit's entry")
			}
			pushed = append(pushed, e.GetSeq())
			continue
		}
		behind = got.GetLogFollow().GetBehindAt()
	}
	if len(pushed) == 0 || len(pushed) == 600 || behind < last {
		t.Fatalf("the follow pushed %d entries and fell behind at %d", len(pushed), behind)
	}
	for _, seq := range pushed {
		if seq > behind {
			t.Errorf("the follow pushed %d, after the sequence %d it fell behind at", seq, behind)
		}
	}
	rest, _, err := b.core.Logs.Query(logstore.Query{After: behind, Unit: "acquire"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pushed)+len(rest) != 600 {
		t.Errorf("the follow pushed %d and a query from %d answers %d, want 600 in all", len(pushed), behind, len(rest))
	}
}

// std: yoke:reads-and-the-log.07
func TestAReadAndAQueryLeaveNoTrace(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	appended(t, b.core.Logs, 1, logstore.Entry{Unit: "acquire", Incarnation: 1, Source: logstore.FromCore, Severity: 10, Message: "one"})
	b.records(t, "unit", "")
	if _, ref := b.call(t, query(&administrativev1.LogQuery{})); ref != nil {
		t.Fatal(ref)
	}
	time.Sleep(300 * time.Millisecond)
	if all, _ := b.core.Logs.Entries(0); len(all) != 1 {
		t.Errorf("the log store holds %d entries, want the 1 there before", len(all))
	}
	b.events.mu.Lock()
	defer b.events.mu.Unlock()
	if len(b.events.events) != 0 {
		t.Errorf("reading published %v", b.events.events)
	}
}
