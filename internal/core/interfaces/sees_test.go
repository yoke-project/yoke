package interfaces_test

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

func readOf(kind, identity string) *interfacev1.Request {
	return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Read{Read: &interfacev1.Read{Kind: kind, Identity: identity}}}
}

// answerTo calls and returns the frame answering it, skipping the standing subscription's.
func (a *attached) answerTo(t *testing.T, call string, r *interfacev1.Request) *interfacev1.CoreFrame {
	t.Helper()
	a.call(t, call, r)
	for {
		if f := a.next(t); f.GetCall() == call {
			return f
		}
	}
}

func twoChannels(t *testing.T) (*world, *attached) {
	t.Helper()
	w := newWorld()
	b := w.served(t, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}, {Name: "remote", Transport: "local", Clients: "single"}}, nil)
	a, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	return w, a
}

// std: yoke:what-a-channel-sees.01
func TestAReadAnswersWhatTheChannelObservesAndRefusesWhatItCannotName(t *testing.T) {
	_, a := twoChannels(t)
	a.next(t)
	count := func(f *interfacev1.CoreFrame) int { return len(f.GetAnswer().GetRead().GetRecords()) }
	if f := a.answerTo(t, "i", readOf("instance", "")); count(f) != 1 || f.GetAnswer().GetRead().GetRecords()[0].GetInstance() == nil {
		t.Errorf("the instance read as %v", f)
	}
	if f := a.answerTo(t, "u", readOf("unit", "")); count(f) != 2 {
		t.Errorf("the units read as %v", f)
	}
	if f := a.answerTo(t, "u1", readOf("unit", "acquire")); count(f) != 1 || f.GetAnswer().GetRead().GetRecords()[0].GetUnit().GetDeclared().GetIdentity() != "acquire" {
		t.Errorf("one unit read as %v", f)
	}
	for call, r := range map[string]*interfacev1.Request{"c": readOf("channel", ""), "c1": readOf("channel", "panel")} {
		f := a.answerTo(t, call, r)
		if count(f) != 1 || f.GetAnswer().GetRead().GetRecords()[0].GetChannel().GetDeclared().GetName() != "panel" {
			t.Errorf("%v read as %v", r, f)
		}
	}
	for call, r := range map[string]*interfacev1.Request{"other": readOf("channel", "remote"), "nobody": readOf("unit", "nobody")} {
		f := a.answerTo(t, call, r)
		ref := f.GetRefusal()
		if ref.GetCode() != "subject.unknown" || ref.GetSubject().GetKind() != r.GetRead().GetKind() || ref.GetSubject().GetIdentity() != r.GetRead().GetIdentity() {
			t.Errorf("%v was answered %v", r, f)
		}
	}
	if f := a.answerTo(t, "p", readOf("plugin", "")); f.GetAnswer().GetRead() == nil || count(f) != 0 {
		t.Errorf("the plugin kind read as %v", f)
	}
	if f := a.answerTo(t, "x", readOf("gadget", "")); f.GetRefusal().GetCode() != "operation.malformed" {
		t.Errorf("a kind that is none read as %v", f)
	}
}

// std: yoke:what-a-channel-sees.02
func TestAReadAndTheOpeningPictureAreOneRecord(t *testing.T) {
	_, a := twoChannels(t)
	pictured := map[string]*interfacev1.UnitRecord{}
	for _, r := range a.next(t).GetOpening().GetPicture().GetRecords() {
		if u := r.GetUnit(); u != nil {
			pictured[u.GetDeclared().GetIdentity()] = u
		}
	}
	for _, r := range a.answerTo(t, "u", readOf("unit", "")).GetAnswer().GetRead().GetRecords() {
		u := r.GetUnit()
		if !proto.Equal(u, pictured[u.GetDeclared().GetIdentity()]) {
			t.Errorf("%s reads as %v, and the picture carried %v", u.GetDeclared().GetIdentity(), u, pictured[u.GetDeclared().GetIdentity()])
		}
	}
	if len(pictured) != 2 {
		t.Errorf("the picture carried %d units", len(pictured))
	}
}

// std: yoke:what-a-channel-sees.03
func TestASubscriptionOpensWithWhatItSelectsThenItsEvents(t *testing.T) {
	w, a := twoChannels(t)
	a.next(t)
	sub := func(f *interfacev1.Filter) *interfacev1.Request {
		return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Subscribe{Subscribe: &interfacev1.Subscribe{Filter: f}}}
	}
	// next is the next frame of a call, skipping the standing subscription's.
	next := func(call string) *interfacev1.CoreFrame {
		for {
			if f := a.next(t); f.GetCall() == call {
				return f
			}
		}
	}
	a.call(t, "one", sub(&interfacev1.Filter{SubjectKind: "unit", SubjectIdentity: "acquire"}))
	snap := next("one").GetAnswer().GetSubscribe().GetSnapshot()
	if len(snap.GetRecords()) != 1 || snap.GetRecords()[0].GetUnit().GetDeclared().GetIdentity() != "acquire" {
		t.Fatalf("the first subscription opened with %v", snap)
	}
	w.bus.Publish(event.StateChanged("calibrate-once", 1, unit.Running, unit.Completed))
	w.bus.Publish(event.PolicyChanged("com.example.station", event.Actor{Class: event.ByOperator}, nil, []string{"stream.data.publish"}, nil))
	w.bus.Publish(event.StateChanged("acquire", 2, unit.Running, unit.Stopped))
	f := next("one")
	if e := f.GetAnswer().GetSubscribe().GetEvent(); e.GetSubject().GetIdentity() != "acquire" || e.GetSeq() <= snap.GetAt() {
		t.Errorf("the first subscription carried %v", f)
	}
	a.call(t, "plugins", sub(&interfacev1.Filter{SubjectKind: "plugin"}))
	if snap := next("plugins").GetAnswer().GetSubscribe().GetSnapshot(); snap == nil || len(snap.GetRecords()) != 0 {
		t.Errorf("the plugin subscription opened with %v", snap)
	}
	w.bus.Publish(event.PolicyChanged("com.example.station", event.Actor{Class: event.ByOperator}, nil, nil, []string{"stream.data.publish"}))
	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case f := <-a.frames:
			if f.GetCall() == "plugins" || (f.GetCall() == "one" && f.GetAnswer().GetSubscribe().GetEvent().GetSubject().GetKind() != "unit") {
				t.Errorf("a subscription carried %v", f)
			}
		case <-deadline:
			return
		}
	}
}
