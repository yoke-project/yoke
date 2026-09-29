package admin_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
)

func subscribeTo(f *administrativev1.Filter) *administrativev1.Request {
	return v1(&administrativev1.Request{Operation: &administrativev1.Request_Subscribe{Subscribe: &administrativev1.Subscribe{Filter: f}}})
}

// quiet is the next frame within d, or nil where none came.
func quiet(stream administrativev1.Shell_ConnectClient, d time.Duration) *administrativev1.CoreFrame {
	got := make(chan *administrativev1.CoreFrame, 1)
	go func() {
		f, err := stream.Recv()
		if err == nil {
			got <- f
		}
	}()
	select {
	case f := <-got:
		return f
	case <-time.After(d):
		return nil
	}
}

func (b *bench) publish(t *testing.T, e event.Event) event.Event {
	t.Helper()
	published, err := b.bus.Publish(e)
	if err != nil {
		t.Fatal(err)
	}
	return published
}

// std: yoke:subscriptions.01
func TestASubscriptionOpensWithASnapshotOfRecords(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1), "archive": running(station, 1)}, map[string]string{})
	b.publish(t, event.InstanceReady("bench"))
	before := b.publish(t, event.StateChanged("archive", 1, unit.Starting, unit.Running))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := b.operator.Watch(ctx, subscribeTo(&administrativev1.Filter{SubjectKind: "unit"}))
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	snap := first.GetSubscribe().GetSnapshot()
	read, _ := b.call(t, readOf("unit", ""))
	if snap == nil || snap.GetAt() != before.Seq || !proto.Equal(&administrativev1.Records{Records: snap.GetRecords()}, read.GetRead()) {
		t.Fatalf("the subscription opened with %v, want the records a read answers at %d", first, before.Seq)
	}
	changed := b.publish(t, event.StateChanged("acquire", 1, unit.Running, unit.Failed))
	b.publish(t, event.InstanceStopping("bench"))
	next, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if e := next.GetSubscribe().GetEvent(); e.GetSeq() != changed.Seq || e.GetSubject().GetIdentity() != "acquire" || e.GetType() != "unit.state.changed" {
		t.Errorf("the next answer is %v, want acquire's change at %d", next, changed.Seq)
	}
}

// std: yoke:subscriptions.02
func TestAClientSelectsOnFourAxes(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := b.operator.Watch(ctx, subscribeTo(&administrativev1.Filter{Floor: 50, Type: "unit.state", TypePrefix: true}))
	if err != nil {
		t.Fatal(err)
	}
	stream.Recv()
	b.publish(t, event.StateChanged("acquire", 1, unit.Starting, unit.Running))
	failed := b.publish(t, event.StateChanged("acquire", 1, unit.Running, unit.Failed))
	b.publish(t, event.ConditionChanged("acquire", 1, nil, 70, "hot"))
	b.publish(t, event.StateChanged("acquire", 2, "", unit.Refused))
	next, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if next.GetSubscribe().GetEvent().GetSeq() != failed.Seq {
		t.Errorf("the first event delivered is %v, want the change at 50, %d", next, failed.Seq)
	}
}

// std: yoke:subscriptions.03
func TestAnOverflowIsAnnouncedWithAFreshSnapshot(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := b.operator.Watch(ctx, subscribeTo(&administrativev1.Filter{SubjectKind: "unit"}))
	if err != nil {
		t.Fatal(err)
	}
	stream.Recv()
	detail := bytes.Repeat([]byte("x"), 1024)
	for range 2000 {
		b.publish(t, event.OccurrenceReported("acquire", 1, &pluginv1.Event{Occurrence: "calibration.drift", Severity: 10, Detail: detail}))
	}
	time.Sleep(300 * time.Millisecond)
	var last uint64
	var overflow *administrativev1.Snapshot
	for overflow == nil {
		r, err := stream.Recv()
		if err != nil {
			t.Fatalf("the stream ended before an overflow: %v", err)
		}
		if e := r.GetSubscribe().GetEvent(); e != nil {
			last = e.GetSeq()
			continue
		}
		overflow = r.GetSubscribe().GetOverflow()
		if overflow == nil {
			t.Fatalf("the stream answered %v", r)
		}
	}
	if overflow.GetAt() <= last || len(overflow.GetRecords()) != 1 {
		t.Errorf("the overflow's snapshot is at %d with %d records, after the event %d", overflow.GetAt(), len(overflow.GetRecords()), last)
	}
	after := b.publish(t, event.StateChanged("acquire", 2, "", unit.Starting))
	for {
		r, err := stream.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if e := r.GetSubscribe().GetEvent(); e != nil {
			if e.GetSeq() <= overflow.GetAt() {
				t.Fatalf("after the overflow the event %d arrived, at or before its snapshot at %d", e.GetSeq(), overflow.GetAt())
			}
			if e.GetSeq() == after.Seq {
				break
			}
		}
	}
}

// std: yoke:subscriptions.04
func TestOnTheShellASubscriptionStandsFromTheStart(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	b.publish(t, event.InstanceReady("bench"))
	stream, opening, _ := opened(t, b.shell)
	standing := opening.GetSubscription()
	if standing == "" {
		t.Fatal("the opening names no standing subscription")
	}
	snap := nextAny(t, stream)
	if snap.GetCall() != standing || snap.GetAnswer().GetSubscribe().GetSnapshot() == nil {
		t.Fatalf("after the opening came %v, want the standing subscription's snapshot", snap)
	}
	kinds := map[string]bool{}
	for _, r := range snap.GetAnswer().GetSubscribe().GetSnapshot().GetRecords() {
		kinds[string(r.ProtoReflect().WhichOneof(r.ProtoReflect().Descriptor().Oneofs().ByName("subject")).Name())] = true
	}
	if !kinds["unit"] || !kinds["plugin"] || !kinds["connection"] {
		t.Errorf("the standing snapshot holds the kinds %v, want every subject", kinds)
	}
	changed := b.publish(t, event.StateChanged("acquire", 1, unit.Running, unit.Failed))
	for {
		f := nextAny(t, stream)
		if f.GetEvent().GetSeq() == changed.Seq {
			if f.GetCall() != standing {
				t.Errorf("the event carries %q, want the standing call %q", f.GetCall(), standing)
			}
			break
		}
	}
	cancel(t, stream, standing)
	for {
		f := nextAny(t, stream)
		if f.GetCompletion() != nil {
			if f.GetCall() != standing {
				t.Errorf("the completion carries %q", f.GetCall())
			}
			break
		}
	}
	b.publish(t, event.StateChanged("acquire", 2, "", unit.Starting))
	if f := quiet(stream, 300*time.Millisecond); f != nil {
		t.Errorf("after the cancellation the connection got %v", f)
	}
}

// std: yoke:subscriptions.05
func TestAShellConnectionHoldsAtMostEight(t *testing.T) {
	b := newBench(t, map[string]*fakeUnit{"acquire": running(station, 1)}, map[string]string{})
	stream, opening, _ := opened(t, b.shell)
	for i := range 7 {
		call := fmt.Sprintf("u%d", i)
		issue(t, stream, call, subscribeTo(&administrativev1.Filter{SubjectKind: "unit"}))
		if f := next(t, stream); f.GetCall() != call || f.GetAnswer().GetSubscribe().GetSnapshot() == nil {
			t.Fatalf("the subscription %s was answered %v", call, f)
		}
	}
	issue(t, stream, "u7", subscribeTo(&administrativev1.Filter{SubjectKind: "unit"}))
	if f := next(t, stream); f.GetCall() != "u7" || f.GetRefusal().GetCode() != "operation.malformed" || f.GetRefusal().GetMessage() == "" {
		t.Errorf("a ninth subscription was answered %v", f)
	}
	records := b.records(t, "connection", opening.GetConnection())
	subs := records[0].GetConnection().GetSubscriptions()
	if len(subs) != 8 {
		t.Fatalf("the connection holds %d subscriptions, want 8", len(subs))
	}
	units := 0
	for _, f := range subs {
		if f.GetSubjectKind() == "unit" {
			units++
		}
	}
	if units != 7 {
		t.Errorf("the connection's subscriptions are %v, want the standing one and seven naming unit", subs)
	}
}
