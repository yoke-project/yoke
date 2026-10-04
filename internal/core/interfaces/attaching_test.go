package interfaces_test

import (
	"context"
	"encoding/json"
	"os"
	"os/user"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// fakeUnits is a running Plugin unit and a unit that runs to completion.
type fakeUnits struct{}

func (fakeUnits) IDs() []string { return []string{"acquire", "calibrate-once"} }
func (fakeUnits) Kind(id string) unit.Kind {
	if id == "acquire" {
		return unit.Plugin
	}
	return unit.Oneshot
}
func (fakeUnits) Status(id string) supervisor.Status {
	if id == "acquire" {
		return supervisor.Status{State: unit.Running, Incarnation: 2, Since: time.Unix(1000, 0)}
	}
	return supervisor.Status{State: unit.Completed, Incarnation: 1, Since: time.Unix(900, 0)}
}

// world is a bus and what the surfaces of a deployment read and publish.
type world struct {
	bus *bus.Bus
	mu  sync.Mutex
	pub []event.Event
}

func newWorld() *world { return &world{bus: bus.New()} }

func (w *world) publish(e event.Event) {
	w.mu.Lock()
	w.pub = append(w.pub, e)
	w.mu.Unlock()
	w.bus.Publish(e)
}

func (w *world) of(typ string) []event.Event {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []event.Event
	for _, e := range w.pub {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// served binds the channels given, each with a surface reading the world.
func (w *world) served(t *testing.T, channels []gate.Channel, confirm *interfaces.Confirmation) *interfaces.Bound {
	t.Helper()
	b, err := interfaces.BindServing(root(t), mode, channels, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{
			Channel: ch, Bus: w.bus, Publish: w.publish, Confirm: confirm,
			Instance: func() *interfacev1.InstanceRecord { return &interfacev1.InstanceRecord{Ready: true} },
			Units:    fakeUnits{},
			Granted: func(id string) (streams, commands, queries []string) {
				if id == "acquire" {
					return []string{"station.spectra"}, []string{"calibrate"}, []string{"head-status"}
				}
				return nil, nil, nil
			},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

// attachTo attaches to a bound channel, returning the client and its frames, or the refusal.
func attachTo(t *testing.T, address string) (*attached, error) {
	t.Helper()
	target := "unix://" + address
	if address[0] != '/' {
		target = address
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err != nil {
		return nil, err
	}
	first, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	a := &attached{stream: stream, frames: make(chan *interfacev1.CoreFrame, 64)}
	a.frames <- first
	go func() {
		defer close(a.frames)
		for {
			f, err := stream.Recv()
			if err != nil {
				return
			}
			a.frames <- f
		}
	}()
	return a, nil
}

func channelRecord(picture *interfacev1.Snapshot) []*interfacev1.ChannelRecord {
	var out []*interfacev1.ChannelRecord
	for _, r := range picture.GetRecords() {
		if c := r.GetChannel(); c != nil {
			out = append(out, c)
		}
	}
	return out
}

func clientOf(e event.Event) string {
	var d struct{ Client string }
	json.Unmarshal(e.Detail, &d)
	return d.Client
}

// std: yoke:attaching.01
func TestTheClientIsEstablishedByTheClassOfTheAddress(t *testing.T) {
	w := newWorld()
	b := w.served(t, []gate.Channel{
		{Name: "panel", Transport: "local", Clients: "multiple"},
		{Name: "remote", Transport: "local", Clients: "multiple", Address: &gate.Address{Class: "loopback", Port: freePort(t)}},
	}, nil)
	me, _ := user.LookupId(strconv.Itoa(os.Getuid()))
	for name, want := range map[string]string{"panel": me.Username, "remote": "unestablished"} {
		a, err := attachTo(t, b.Address(name))
		if err != nil {
			t.Fatalf("attaching to %s: %v", name, err)
		}
		records := channelRecord(a.next(t).GetOpening().GetPicture())
		if len(records) != 1 || records[0].GetDeclared().GetName() != name || !records[0].GetObserved().GetAttached() || records[0].GetObserved().GetClient() != want {
			t.Errorf("%s's opening carries %v, want it attached by %s", name, records, want)
		}
	}
	got := map[string]string{}
	for _, e := range w.of("channel.attached") {
		got[e.Subject.ID] = clientOf(e)
	}
	if got["panel"] != me.Username || got["remote"] != "unestablished" {
		t.Errorf("channel.attached carried %v", got)
	}
}

// std: yoke:attaching.02
func TestASingleChannelRefusesASecondClient(t *testing.T) {
	w := newWorld()
	b := w.served(t, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}, {Name: "bench", Transport: "local", Clients: "multiple"}}, nil)
	first, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	first.next(t)
	_, err = attachTo(t, b.Address("panel"))
	ref := refusalIn(err)
	if ref.GetCode() != "channel.in_use" || ref.GetDetail() != nil {
		t.Errorf("the second attachment was answered %v", err)
	}
	first.call(t, "r", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Confirm{Confirm: &interfacev1.Confirm{Subscription: interfaces.Standing}}})
	if f := first.next(t); f.GetCall() != "r" {
		t.Errorf("the first client, after the refusal, was answered %v", f)
	}
	for i := 0; i < 2; i++ {
		a, err := attachTo(t, b.Address("bench"))
		if err != nil || a.next(t).GetOpening() == nil {
			t.Errorf("attachment %d to the multiple channel: %v", i, err)
		}
	}
}

func refusalIn(err error) *interfacev1.Refusal {
	st, _ := status.FromError(err)
	for _, d := range st.Details() {
		if r, ok := d.(*interfacev1.Refusal); ok {
			return r
		}
	}
	return nil
}

// std: yoke:attaching.03
func TestTheOpeningPictureIsWhatTheChannelMayAddressAndObserve(t *testing.T) {
	w := newWorld()
	w.publish(event.InstanceReady("bench"))
	last, _ := w.bus.Publish(event.StateChanged("acquire", 2, unit.Admitted, unit.Running))
	b := w.served(t, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}, {Name: "remote", Transport: "local", Clients: "single"}}, nil)
	a, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	o := a.next(t).GetOpening()
	if o.GetSubscription() != interfaces.Standing || o.GetVersion() != 1 || o.GetPicture().GetAt() < last.Seq {
		t.Errorf("the opening is %v, its picture at %d, want at least %d", o, o.GetPicture().GetAt(), last.Seq)
	}
	var instance bool
	units := map[string]*interfacev1.UnitRecord{}
	for _, r := range o.GetPicture().GetRecords() {
		instance = instance || r.GetInstance().GetReady()
		if u := r.GetUnit(); u != nil {
			units[u.GetDeclared().GetIdentity()] = u
		}
	}
	if !instance {
		t.Error("the picture holds no instance")
	}
	acquire, once := units["acquire"], units["calibrate-once"]
	if acquire.GetObserved().GetState() != "Running" || acquire.GetObserved().GetIncarnation() != 2 || acquire.GetDeclared().GetKind() != "plugin" ||
		!slices.Equal(acquire.GetAddressed().GetStreams(), []string{"station.spectra"}) || !slices.Equal(acquire.GetAddressed().GetCommands(), []string{"calibrate"}) ||
		!slices.Equal(acquire.GetAddressed().GetQueries(), []string{"head-status"}) {
		t.Errorf("the Plugin unit's record is %v", acquire)
	}
	if once.GetObserved().GetState() != "Completed" || len(once.GetAddressed().GetStreams())+len(once.GetAddressed().GetCommands())+len(once.GetAddressed().GetQueries()) != 0 {
		t.Errorf("the other unit's record is %v", once)
	}
	if records := channelRecord(o.GetPicture()); len(records) != 1 || records[0].GetDeclared().GetName() != "panel" || records[0].GetDeclared().GetClients() != "single" {
		t.Errorf("the picture's channel records are %v", records)
	}
}

// std: yoke:attaching.04
func TestTheStandingSubscriptionCarriesWhatTheChannelObserves(t *testing.T) {
	w := newWorld()
	b := w.served(t, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}, {Name: "remote", Transport: "local", Clients: "single"}}, nil)
	a, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	at := a.next(t).GetOpening().GetPicture().GetAt()
	w.bus.Publish(event.StateChanged("acquire", 2, unit.Running, unit.Stopped))
	w.bus.Publish(event.InstanceStopping("bench"))
	w.bus.Publish(event.PolicyChanged("com.example.station", event.Actor{Class: event.ByOperator}, nil, []string{"stream.data.publish"}, nil))
	w.bus.Publish(event.ConnectionOpened("c-1", "shell", event.Actor{Class: event.ByOperator}))
	w.bus.Publish(event.ChannelAttached("remote", "someone"))
	w.bus.Publish(event.ChannelAttached("panel", "someone"))
	var got []string
	for len(got) < 3 {
		f := a.next(t)
		e := f.GetEvent()
		if f.GetCall() != interfaces.Standing || e == nil || e.GetSeq() <= at {
			t.Fatalf("the standing subscription carried %v", f)
		}
		got = append(got, e.GetType()+" "+e.GetSubject().GetIdentity())
	}
	want := []string{"unit.state.changed acquire", "instance.stopping bench", "channel.attached panel"}
	if !slices.Equal(got, want) {
		t.Errorf("the standing subscription carried %q, want %q", got, want)
	}
	select {
	case f := <-a.frames:
		t.Errorf("it also carried %v", f)
	case <-time.After(200 * time.Millisecond):
	}
}

// std: yoke:attaching.05
func TestAnAttachmentThatEndsPublishesDetachedAndLeavesNothing(t *testing.T) {
	w := newWorld()
	b := w.served(t, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}}, nil)
	first, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	first.next(t)
	first.stream.CloseSend()
	deadline := time.Now().Add(2 * time.Second)
	for len(w.of("channel.detached")) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	detached := w.of("channel.detached")
	var d struct{ Client, Reason string }
	if len(detached) == 1 {
		json.Unmarshal(detached[0].Detail, &d)
	}
	if len(detached) != 1 || detached[0].Subject.ID != "panel" || d.Reason != "closed" || d.Client == "" {
		t.Fatalf("published %+v", detached)
	}
	second, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatalf("the second client was refused: %v", err)
	}
	if records := channelRecord(second.next(t).GetOpening().GetPicture()); len(records) != 1 || !records[0].GetObserved().GetAttached() {
		t.Errorf("the second opening's channel record is %v", records)
	}
}

// std: yoke:attaching.06
func TestOnAChannelNamedInARuleTheSubscriptionGoesStaleWhenNotConfirmed(t *testing.T) {
	w := newWorld()
	confirm := &interfaces.Confirmation{Every: 100 * time.Millisecond, Tolerance: 3, Required: map[string]bool{"panel": true}}
	b := w.served(t, []gate.Channel{{Name: "panel", Transport: "local", Clients: "single"}, {Name: "bench", Transport: "local", Clients: "single"}}, confirm)
	panel, err := attachTo(t, b.Address("panel"))
	if err != nil {
		t.Fatal(err)
	}
	at := panel.next(t).GetOpening().GetPicture().GetAt()
	bench, err := attachTo(t, b.Address("bench"))
	if err != nil {
		t.Fatal(err)
	}
	bench.next(t)
	for i := 0; i < 5; i++ {
		panel.call(t, "c", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Confirm{Confirm: &interfacev1.Confirm{Subscription: interfaces.Standing, Sequence: at}}})
		for {
			f := panel.next(t)
			if f.GetCall() == "c" {
				if f.GetAnswer().GetConfirm() == nil {
					t.Fatalf("a confirmation was answered %v", f)
				}
				break
			}
		}
		if n := len(w.of("channel.subscription.stale")); n != 0 {
			t.Fatalf("the channel went stale while confirming")
		}
		time.Sleep(100 * time.Millisecond)
	}
	last := time.Now()
	deadline := last.Add(time.Second)
	for len(w.of("channel.subscription.stale")) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	stale := w.of("channel.subscription.stale")
	if len(stale) != 1 || stale[0].Subject.ID != "panel" || time.Since(last) > 600*time.Millisecond {
		t.Errorf("after %v published %+v", time.Since(last), stale)
	}
	var d struct{ Since time.Time }
	if len(stale) == 1 {
		json.Unmarshal(stale[0].Detail, &d)
		if d.Since.IsZero() || d.Since.After(last) {
			t.Errorf("the stale event says it last confirmed at %v", d.Since)
		}
	}
}
