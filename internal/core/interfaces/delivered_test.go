package interfaces_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// delivering is a deployment whose bench unit acquire is granted station.spectra and declares
// station.preview without its grant, with a transport service the test drives as a unit would.
type delivering struct {
	queue      int
	w          *world
	root       string
	transports *streams.Service
	bound      *interfaces.Bound
}

func deliveringOn(t *testing.T, channels ...gate.Channel) *delivering {
	t.Helper()
	return deliveringWith(t, 0, channels...)
}

// deliveringWith is deliveringOn with each delivery holding at most queue frames, the default where 0.
func deliveringWith(t *testing.T, queue int, channels ...gate.Channel) *delivering {
	t.Helper()
	d := &delivering{queue: queue, w: newWorld(), root: root(t)}
	d.transports = streams.New(streams.Config{Root: d.root, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Publish: d.w.publish})
	t.Cleanup(func() { d.transports.CloseAll("acquire", streams.UnitExited) })
	manifest := &gate.Manifest{ID: "com.example.station", Streams: []gate.Stream{{ID: "station.spectra"}, {ID: "station.preview"}}}
	b, err := interfaces.BindServing(d.root, mode, channels, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{
			Channel: ch, Root: d.root, Queue: d.queue, Bus: d.w.bus, Publish: d.w.publish, Units: bench{}, Sessions: &fakeSessions{}, Transports: d.transports,
			Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
			Granted: func(id string) ([]string, []string, []string) {
				if id == "acquire" {
					return []string{"station.spectra"}, nil, nil
				}
				return nil, nil, nil
			},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	d.bound = b
	return d
}

// flow opens the stream's transport as an activation would, and returns what writes to it as its unit.
func (d *delivering) flow(t *testing.T) func(sequence uint64, payload []byte) {
	t.Helper()
	_, address, err := d.transports.Open("acquire", 3, "station.spectra", streams.Tolerances{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unixpacket", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return func(sequence uint64, payload []byte) {
		raw, _ := proto.Marshal(&pluginv1.Envelope{MessageId: "d", SentAtUnixNano: int64(1000 + sequence),
			Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: sequence, Payload: payload}}})
		conn.Write(raw)
	}
}

func subscribeTo(name string) *interfacev1.Request {
	return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamSubscribe{StreamSubscribe: &interfacev1.UnitStream{Unit: "acquire", Stream: name}}}
}

// reader connects to a subscriber socket and returns what reads its packets.
func reader(t *testing.T, path string) func() []byte {
	t.Helper()
	conn, err := net.Dial("unixpacket", path)
	if err != nil {
		t.Fatalf("the subscriber socket %s does not answer: %v", path, err)
	}
	t.Cleanup(func() { conn.Close() })
	return func() []byte {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 1<<16)
		n, err := conn.Read(buf)
		if err != nil {
			return nil
		}
		return bytes.Clone(buf[:n])
	}
}

func packet(sequence uint64, payload []byte) []byte {
	out := make([]byte, 16)
	binary.LittleEndian.PutUint64(out[0:8], sequence)
	binary.LittleEndian.PutUint64(out[8:16], 1000+sequence)
	return append(out, payload...)
}

var subscriber = regexp.MustCompile(`/plugins/acquire/subscribers/station\.spectra/(\d{8})\.sock$`)

// std: yoke:streams-delivered.01
func TestOnALocalChannelTheAnswerNamesASocketUnderACountedIdentity(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	var last string
	for _, call := range []string{"s1", "s2"} {
		got := a.answerTo(t, call, subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe()
		m := subscriber.FindStringSubmatch(got.GetSocket())
		if m == nil || got.GetDelivery() != m[1] || got.GetFlowing() || !filepath.IsAbs(got.GetSocket()) {
			t.Fatalf("%s was answered %v", call, got)
		}
		if _, err := os.Stat(got.GetSocket()); err != nil {
			t.Errorf("%s's socket does not exist: %v", call, err)
		}
		if m[1] <= last {
			t.Errorf("the subscriber %s follows %s", m[1], last)
		}
		last = m[1]
	}
}

// std: yoke:streams-delivered.02
func TestWhatIsReadReachesEverySubscriberUnchanged(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	first := reader(t, a.answerTo(t, "s1", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe().GetSocket())
	second := reader(t, a.answerTo(t, "s2", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe().GetSocket())
	time.Sleep(50 * time.Millisecond)
	write := d.flow(t)
	for i := uint64(1); i <= 3; i++ {
		write(i, []byte{byte(i), 0xab})
	}
	for name, read := range map[string]func() []byte{"first": first, "second": second} {
		for i := uint64(1); i <= 3; i++ {
			if got := read(); !bytes.Equal(got, packet(i, []byte{byte(i), 0xab})) {
				t.Errorf("the %s subscriber read %x for message %d", name, got, i)
			}
		}
	}
}

// std: yoke:streams-delivered.03
func TestOnAnAddressTheDeliveryArrivesOnTheConnection(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "remote", Transport: "local", Clients: "single", Address: &gate.Address{Class: "loopback", Port: freePort(t)}})
	a, _ := attachTo(t, d.bound.Address("remote"))
	a.next(t)
	got := a.answerTo(t, "s", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe()
	if !got.GetConnection() || got.GetDelivery() == "" {
		t.Fatalf("the subscription was answered %v", got)
	}
	write := d.flow(t)
	write(1, []byte("one"))
	write(2, []byte("two"))
	for i, want := range []string{"one", "two"} {
		var delivery *interfacev1.StreamDelivery
		for delivery == nil {
			delivery = a.next(t).GetDelivery()
		}
		if delivery.GetDelivery() != got.GetDelivery() || delivery.GetSequence() != uint64(i+1) || delivery.GetSentAt() != uint64(1001+i) || string(delivery.GetPayload()) != want {
			t.Errorf("delivery %d is %v", i, delivery)
		}
	}
}

// std: yoke:streams-delivered.04
func TestADeliveryIsReleasedOrKeptAsTheContractSays(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "panel", Transport: "local", Clients: "single"}, gate.Channel{Name: "bench", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	var sockets []string
	for _, call := range []string{"s1", "s2", "s3"} {
		sockets = append(sockets, a.answerTo(t, call, subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe().GetSocket())
	}
	other, _ := attachTo(t, d.bound.Address("bench"))
	other.next(t)
	otherSocket := other.answerTo(t, "o", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe().GetSocket()

	first := a.answerTo(t, "u", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamUnsubscribe{StreamUnsubscribe: &interfacev1.StreamRelease{
		Delivery: subscriber.FindStringSubmatch(sockets[0])[1]}}})
	if f := first.GetAnswer().GetStreamUnsubscribe(); f == nil {
		t.Errorf("the unsubscribe was answered %v", first)
	}
	if _, err := os.Stat(sockets[0]); err == nil {
		t.Error("the unsubscribed socket is still there")
	}
	read := reader(t, sockets[1])
	d.flow(t)
	d.transports.Close("acquire", "station.spectra", streams.Asked)
	if _, err := os.Stat(sockets[1]); err != nil {
		t.Errorf("a delivery did not survive the stream stopping: %v", err)
	}
	write := d.flow(t)
	write(1, []byte("again"))
	if got := read(); !bytes.Equal(got, packet(1, []byte("again"))) {
		t.Errorf("after the restart the delivery read %x", got)
	}
	d.transports.CloseAll("acquire", streams.SessionEnded)
	deadline := time.Now().Add(2 * time.Second)
	for _, s := range sockets[1:] {
		for {
			if _, err := os.Stat(s); err != nil {
				break
			}
			if time.Now().After(deadline) {
				t.Errorf("%s survived the Session ending", s)
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	other.stream.CloseSend()
	detached := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(otherSocket); err != nil {
			break
		}
		if time.Now().After(detached) {
			t.Error("the other attachment's delivery survived its detaching")
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// std: yoke:streams-delivered.05
func TestAStreamTheChannelMayNotAddressIsRefused(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	nobody := &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamSubscribe{StreamSubscribe: &interfacev1.UnitStream{Unit: "nobody", Stream: "station.spectra"}}}
	if ref := a.answerTo(t, "n", nobody).GetRefusal(); ref.GetCode() != "subject.unknown" || ref.GetSubject().GetIdentity() != "nobody" {
		t.Errorf("a unit nobody declared was refused %v", ref)
	}
	for call, c := range map[string][2]string{"u": {"station.nothing", "scope.undeclared"}, "w": {"station.preview", "scope.withheld"}} {
		if ref := a.answerTo(t, call, subscribeTo(c[0])).GetRefusal(); ref.GetCode() != c[1] || ref.GetItem() != c[0] {
			t.Errorf("%s was refused %v, want %s", c[0], ref, c[1])
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(d.root, "plugins", "acquire", "subscribers")); len(entries) != 0 {
		t.Errorf("a refusal left %v", entries)
	}
}

// std: yoke:streams-delivered.07
func TestADeliveryWhoseClientFallsBehindIsReleased(t *testing.T) {
	d := deliveringWith(t, 4, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	got := a.answerTo(t, "s", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe()
	conn, err := net.Dial("unixpacket", got.GetSocket())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)
	write := d.flow(t)
	big := make([]byte, 32<<10)
	for i := uint64(1); i <= 200; i++ {
		write(i, big)
	}
	var last uint64
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 64<<10)
	for {
		n, err := conn.Read(buf)
		if err != nil || n == 0 {
			break
		}
		sequence := binary.LittleEndian.Uint64(buf[0:8])
		if sequence != last+1 {
			t.Fatalf("after %d the client read %d: a gap", last, sequence)
		}
		last = sequence
	}
	if last == 0 || last == 200 {
		t.Errorf("the client read up to %d, and the delivery was not released", last)
	}
	ref := a.answerTo(t, "u", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamUnsubscribe{StreamUnsubscribe: &interfacev1.StreamRelease{Delivery: got.GetDelivery()}}}).GetRefusal()
	if ref == nil {
		t.Error("the released delivery could still be unsubscribed")
	}
}
