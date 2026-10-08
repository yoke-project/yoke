package interfaces_test

import (
	"encoding/json"
	"testing"
	"time"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// arbitrated binds the channels given under the rules given, each surface reading the world, with the
// bench's units and Sessions so that operations can be withdrawn.
func (w *world) arbitrated(t *testing.T, channels []gate.Channel, rules []gate.Rule, confirm *interfaces.Confirmation) *interfaces.Bound {
	t.Helper()
	arbiter := interfaces.NewArbiter(channels, rules, w.publish)
	sessions := &fakeSessions{}
	manifest := &gate.Manifest{ID: "com.example.station", Commands: []string{"calibrate"}, Queries: []string{"head-status"}}
	b, err := interfaces.BindServing(root(t), mode, channels, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{
			Channel: ch, Bus: w.bus, Publish: w.publish, Confirm: confirm, Arbiter: arbiter, Units: bench{}, Sessions: sessions,
			Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func local(name string) gate.Channel {
	return gate.Channel{Name: name, Transport: "local", Clients: "multiple", OnSuspend: "read-only"}
}

// waitFor waits until the world has published n events of a type about a channel, and returns them.
// at is where the first event of typ about channel stands in what was published, or -1.
func (w *world) at(typ, channel string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	for i, e := range w.pub {
		if e.Type == typ && e.Subject.ID == channel {
			return i
		}
	}
	return -1
}

func (w *world) waitFor(t *testing.T, typ, channel string, n int) []event.Event {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		var got []event.Event
		for _, e := range w.of(typ) {
			if e.Subject.ID == channel {
				got = append(got, e)
			}
		}
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func detailOf(e event.Event) map[string]string {
	d := map[string]string{}
	json.Unmarshal(e.Detail, &d)
	return d
}

// std: yoke:arbitration.01
func TestAPrevailingChannelSuspendsTheOthersAndOnlyAChangeIsPublished(t *testing.T) {
	w := newWorld()
	b := w.arbitrated(t, []gate.Channel{local("bench"), local("remote"), local("third")}, []gate.Rule{{Prevails: "bench", Over: []string{"remote"}}}, nil)
	remote, _ := attachTo(t, b.Address("remote"))
	remote.next(t)
	bench, _ := attachTo(t, b.Address("bench"))
	bench.next(t)
	suspended := w.waitFor(t, "channel.suspended", "remote", 1)
	if len(suspended) != 1 || detailOf(suspended[0])["by"] != "bench" || detailOf(suspended[0])["retains"] != "read-only" || detailOf(suspended[0])["reason"] == "" {
		t.Fatalf("published %+v", suspended)
	}
	third, _ := attachTo(t, b.Address("third"))
	third.next(t)
	bench.stream.CloseSend()
	if resumed := w.waitFor(t, "channel.resumed", "remote", 1); len(resumed) != 1 {
		t.Errorf("remote was not resumed: %+v", resumed)
	}
	w.waitFor(t, "channel.detached", "bench", 1)
	if detached, resumed := w.at("channel.detached", "bench"), w.at("channel.resumed", "remote"); detached > resumed {
		t.Errorf("remote was resumed (%d) before bench's detachment (%d) was published", resumed, detached)
	}
	if n := len(w.of("channel.suspended")) + len(w.of("channel.resumed")); n != 2 {
		t.Errorf("%d arbitration events were published", n)
	}
}

// std: yoke:arbitration.02
func TestAChannelWhoseSubscriptionGoesStaleStopsHolding(t *testing.T) {
	w := newWorld()
	confirm := &interfaces.Confirmation{Every: 100 * time.Millisecond, Tolerance: 3, Required: map[string]bool{"bench": true, "remote": true}}
	b := w.arbitrated(t, []gate.Channel{local("bench"), local("remote")}, []gate.Rule{{Prevails: "bench", Over: []string{"remote"}}}, confirm)
	remote, _ := attachTo(t, b.Address("remote"))
	remote.next(t)
	bench, _ := attachTo(t, b.Address("bench"))
	bench.next(t)
	done := make(chan struct{})
	defer close(done)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(50 * time.Millisecond):
				remote.stream.Send(&interfacev1.ClientFrame{Call: "c", Carries: &interfacev1.ClientFrame_Request{Request: &interfacev1.Request{Version: 1,
					Operation: &interfacev1.Request_Confirm{Confirm: &interfacev1.Confirm{Subscription: interfaces.Standing}}}}})
			}
		}
	}()
	if len(w.waitFor(t, "channel.suspended", "remote", 1)) != 1 {
		t.Fatal("remote was never suspended")
	}
	if len(w.waitFor(t, "channel.resumed", "remote", 1)) != 1 {
		t.Fatal("remote was not resumed when bench went stale")
	}
	bench.call(t, "c", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Confirm{Confirm: &interfacev1.Confirm{Subscription: interfaces.Standing}}})
	if len(w.waitFor(t, "channel.suspended", "remote", 2)) != 2 {
		t.Error("remote was not suspended again when bench confirmed")
	}
}

// std: yoke:arbitration.03
func TestAGradeWithdrawsWhatItWithdraws(t *testing.T) {
	w := newWorld()
	darkChannel := local("dark")
	darkChannel.OnSuspend = "dark"
	b := w.arbitrated(t, []gate.Channel{local("bench"), local("remote"), darkChannel}, []gate.Rule{{Prevails: "bench", Over: []string{"remote", "dark"}}}, nil)
	remote, _ := attachTo(t, b.Address("remote"))
	remote.next(t)
	dark, _ := attachTo(t, b.Address("dark"))
	dark.next(t)
	bench, _ := attachTo(t, b.Address("bench"))
	bench.next(t)
	w.waitFor(t, "channel.suspended", "dark", 1)
	query := &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Query{Query: &interfacev1.Question{Unit: "acquire", Type: "head-status"}}}
	ref := remote.answerTo(t, "cmd", command("acquire", "calibrate", nil)).GetRefusal()
	if ref.GetCode() != "channel.suspended" || ref.GetSuspension().GetGrade() != "read-only" || ref.GetSuspension().GetBy() != "bench" {
		t.Errorf("a command on a read-only channel was refused %v", ref)
	}
	if f := remote.answerTo(t, "q", query); f.GetAnswer().GetQuery() == nil {
		t.Errorf("a question on a read-only channel was answered %v", f)
	}
	if f := remote.answerTo(t, "r", readOf("unit", "acquire")); f.GetAnswer().GetRead() == nil {
		t.Errorf("a read on a read-only channel was answered %v", f)
	}
	ref = dark.answerTo(t, "q", query).GetRefusal()
	if ref.GetCode() != "channel.suspended" || ref.GetSuspension().GetGrade() != "dark" || ref.GetSuspension().GetBy() != "bench" {
		t.Errorf("a question on a dark channel was refused %v", ref)
	}
	if f := dark.answerTo(t, "r", readOf("channel", "dark")); f.GetAnswer().GetRead() == nil {
		t.Errorf("a read of its own channel on a dark channel was answered %v", f)
	}
	w.bus.Publish(event.StateChanged("acquire", 3, unit.Running, unit.Stopped))
	w.bus.Publish(event.ChannelAttached("dark", "someone"))
	saw := func(a *attached, want string) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case f := <-a.frames:
				if e := f.GetEvent(); e != nil {
					got := e.GetType() + " " + e.GetSubject().GetIdentity()
					if got == want {
						return
					}
					if a == dark && e.GetSubject().GetKind() != "channel" {
						t.Errorf("the dark channel was carried %s", got)
					}
				}
			case <-deadline:
				t.Errorf("never carried %s", want)
				return
			}
		}
	}
	saw(remote, "unit.state.changed acquire")
	saw(dark, "channel.attached dark")
}

// std: yoke:arbitration.04
func TestALocalChannelMayAlwaysReclaim(t *testing.T) {
	w := newWorld()
	panel := local("panel")
	panel.OnSuspend = "dark"
	loop := local("bench")
	loop.Address = &gate.Address{Class: "loopback", Port: freePort(t)}
	b := w.arbitrated(t, []gate.Channel{loop, panel}, []gate.Rule{{Prevails: "bench", Over: []string{"panel"}}}, nil)
	p, _ := attachTo(t, b.Address("panel"))
	p.next(t)
	bench, _ := attachTo(t, b.Address("bench"))
	bench.next(t)
	w.waitFor(t, "channel.suspended", "panel", 1)
	reclaim := &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Reclaim{Reclaim: &interfacev1.Reclaim{}}}
	got := p.answerTo(t, "re", reclaim).GetAnswer().GetReclaim()
	if !got.GetChanged() || got.GetChannel().GetObserved().GetSuspended() {
		t.Errorf("the reclaim was answered %v", got)
	}
	if len(w.waitFor(t, "channel.resumed", "panel", 1)) != 1 {
		t.Error("panel was not resumed")
	}
	if s := w.waitFor(t, "channel.suspended", "bench", 1); len(s) != 1 || detailOf(s[0])["reason"] != "reclaimed" || detailOf(s[0])["by"] != "panel" {
		t.Errorf("bench's suspension is %+v", s)
	}
	if ref := bench.answerTo(t, "re", reclaim).GetRefusal(); ref.GetCode() != "channel.not_local" {
		t.Errorf("a reclaim on loopback was refused %v", ref)
	}
	p.stream.CloseSend()
	if len(w.waitFor(t, "channel.resumed", "bench", 1)) != 1 {
		t.Error("bench was not resumed when panel detached")
	}
}

// std: yoke:arbitration.05
func TestASuspensionIsAStateOfTheChannel(t *testing.T) {
	w := newWorld()
	b := w.arbitrated(t, []gate.Channel{local("bench"), local("remote")}, []gate.Rule{{Prevails: "bench", Over: []string{"remote"}}}, nil)
	remote, _ := attachTo(t, b.Address("remote"))
	remote.next(t)
	bench, _ := attachTo(t, b.Address("bench"))
	bench.next(t)
	w.waitFor(t, "channel.suspended", "remote", 1)
	check := func(what string, c *interfacev1.ChannelRecord) {
		t.Helper()
		o := c.GetObserved()
		if !o.GetSuspended() || o.GetGrade() != "read-only" || o.GetBy() != "bench" || o.GetReason() == "" {
			t.Errorf("%s says %v", what, o)
		}
	}
	check("the read", remote.answerTo(t, "r", readOf("channel", "remote")).GetAnswer().GetRead().GetRecords()[0].GetChannel())
	second, _ := attachTo(t, b.Address("remote"))
	check("the second opening", channelRecord(second.next(t).GetOpening().GetPicture())[0])
}
