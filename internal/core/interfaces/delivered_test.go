package interfaces_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/ring"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// delivering is a deployment whose bench unit acquire is granted station.spectra and declares
// station.preview without its grant, with a transport service the test drives as a unit would.
type delivering struct {
	offers     streams.Offers
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
	return deliveringAll(t, queue, streams.Offers{}, channels...)
}

// deliveringAll is deliveringWith, every unit's library having offered offers.
func deliveringAll(t *testing.T, queue int, offers streams.Offers, channels ...gate.Channel) *delivering {
	t.Helper()
	d := &delivering{queue: queue, offers: offers, w: newWorld(), root: root(t)}
	d.transports = streams.New(streams.Config{Root: d.root, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Publish: d.w.publish})
	t.Cleanup(func() { d.transports.CloseAll("acquire", streams.UnitExited) })
	manifest := &gate.Manifest{ID: "com.example.station", Streams: []gate.Stream{{ID: "station.spectra"}, {ID: "station.preview"}, {ID: "station.raw", ToleratesLoss: true}}}
	b, err := interfaces.BindServing(d.root, "", mode, channels, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{
			Channel: ch, Root: d.root, Queue: d.queue, Bus: d.w.bus, Publish: d.w.publish, Units: bench{}, Sessions: &fakeSessions{offers: d.offers}, Transports: d.transports,
			Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
			Granted: func(id string) ([]string, []string, []string) {
				if id == "acquire" {
					return []string{"station.spectra", "station.raw"}, nil, nil
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
	_, address, err := d.transports.Open("acquire", 3, "station.spectra", streams.Tolerances{}, streams.Offers{})
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

// flowSized is flow, with the unit's socket able to send a message of size bytes, as a library sending
// large frames sets it.
func (d *delivering) flowSized(t *testing.T, size int) func(sequence uint64, payload []byte) {
	t.Helper()
	_, address, err := d.transports.Open("acquire", 3, "station.spectra", streams.Tolerances{}, streams.Offers{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unixpacket", address)
	if err != nil {
		t.Fatal(err)
	}
	conn.(*net.UnixConn).SetWriteBuffer(size)
	t.Cleanup(func() { conn.Close() })
	return func(sequence uint64, payload []byte) {
		raw, _ := proto.Marshal(&pluginv1.Envelope{MessageId: "d", SentAtUnixNano: int64(1000 + sequence),
			Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: sequence, Payload: payload}}})
		if _, err := conn.Write(raw); err != nil {
			t.Errorf("the unit could not send %d bytes: %v", len(raw), err)
		}
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
	if last == 200 {
		t.Errorf("the client read all %d, and the delivery was not released", last)
	}
	ref := a.answerTo(t, "u", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamUnsubscribe{StreamUnsubscribe: &interfacev1.StreamRelease{Delivery: got.GetDelivery()}}}).GetRefusal()
	if ref == nil {
		t.Error("the released delivery could still be unsubscribed")
	}
}

// std: yoke:streams-delivered.08
func TestAFrameUpToTheBoundReachesAClientOnASocket(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	got := a.answerTo(t, "s", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe()
	conn, err := net.Dial("unixpacket", got.GetSocket())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.(*net.UnixConn).SetReadBuffer(4 << 20)
	time.Sleep(50 * time.Millisecond)
	write := d.flowSized(t, 2<<20)
	sizes := []int{600 << 10, 1 << 20}
	for i, n := range sizes {
		write(uint64(i+1), bytes.Repeat([]byte{byte(i + 1)}, n))
	}
	buf := make([]byte, 16+(2<<20))
	for i, n := range sizes {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		got, err := conn.Read(buf)
		if err != nil {
			t.Fatalf("frame %d of %d bytes did not arrive: %v", i+1, n, err)
		}
		if got != 16+n || binary.LittleEndian.Uint64(buf[0:8]) != uint64(i+1) || buf[16] != byte(i+1) {
			t.Fatalf("frame %d: read %d bytes, sequence %d; want %d bytes, sequence %d", i+1, got, binary.LittleEndian.Uint64(buf[0:8]), 16+n, i+1)
		}
	}
}

// std: yoke:streams-delivered.09
func TestADeliveryTheCoreCannotWriteToIsReleasedAndSaid(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	got := a.answerTo(t, "s", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe()
	conn, err := net.Dial("unixpacket", got.GetSocket())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	conn.Close()
	write := d.flow(t)
	deadline := time.Now().Add(5 * time.Second)
	for i := uint64(1); len(d.w.of("channel.delivery.failed")) == 0; i++ {
		if time.Now().After(deadline) {
			t.Fatal("no channel.delivery.failed was published for a client that closed its socket")
		}
		write(i, []byte("x"))
		time.Sleep(10 * time.Millisecond)
	}
	e := d.w.of("channel.delivery.failed")[0]
	var detail map[string]string
	json.Unmarshal(e.Detail, &detail)
	if e.Subject.ID != "panel" || detail["unit"] != "acquire" || detail["stream"] != "station.spectra" ||
		detail["delivery"] != got.GetDelivery() || detail["reason"] == "" {
		t.Errorf("published %+v", e)
	}
	ref := a.answerTo(t, "u", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamUnsubscribe{StreamUnsubscribe: &interfacev1.StreamRelease{Delivery: got.GetDelivery()}}}).GetRefusal()
	if ref == nil {
		t.Error("the failed delivery could still be unsubscribed")
	}
}

// std: yoke:streams-delivered.10
func TestAClientThatReadsAStreamSocketIsDeliveredOnOne(t *testing.T) {
	d := deliveringOn(t, gate.Channel{Name: "panel", Transport: "local", Clients: "multiple"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	reads := subscribeTo("station.spectra")
	reads.GetStreamSubscribe().Reads = []interfacev1.UnitStream_Reads{interfacev1.UnitStream_READS_STREAM_SOCKET}
	streamed := a.answerTo(t, "s1", reads).GetAnswer().GetStreamSubscribe()
	packeted := a.answerTo(t, "s2", subscribeTo("station.spectra")).GetAnswer().GetStreamSubscribe()
	if streamed.GetStreamSocket() == "" || packeted.GetSocket() == "" {
		t.Fatalf("answered %v and %v", streamed.GetArrives(), packeted.GetArrives())
	}
	conn, err := net.Dial("unix", streamed.GetStreamSocket())
	if err != nil {
		t.Fatalf("the stream socket does not answer: %v", err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)
	write := d.flowSized(t, 2<<20)
	write(1, bytes.Repeat([]byte{9}, 1<<20))
	write(2, []byte("x"))
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i, want := range []int{1 << 20, 1} {
		var length [4]byte
		if _, err := io.ReadFull(conn, length[:]); err != nil {
			t.Fatalf("frame %d: no length: %v", i+1, err)
		}
		n := int(binary.LittleEndian.Uint32(length[:]))
		body := make([]byte, n)
		if _, err := io.ReadFull(conn, body); err != nil {
			t.Fatalf("frame %d: %v", i+1, err)
		}
		if n != 16+want || binary.LittleEndian.Uint64(body[0:8]) != uint64(i+1) {
			t.Errorf("frame %d: %d bytes, sequence %d", i+1, n, binary.LittleEndian.Uint64(body[0:8]))
		}
	}
}

// lifeline reads one message of a delivery's lifeline: its length, and the descriptor it carries if any.
func lifeline(t *testing.T, conn *net.UnixConn) (int, *os.File) {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	head, oob := make([]byte, 4), make([]byte, unix.CmsgSpace(4))
	n, oobn, _, _, err := conn.ReadMsgUnix(head, oob)
	if err != nil || n != 4 {
		t.Fatalf("no message on the lifeline: %d bytes, %v", n, err)
	}
	length := int(binary.LittleEndian.Uint32(head))
	var f *os.File
	if oobn > 0 {
		messages, _ := unix.ParseSocketControlMessage(oob[:oobn])
		if len(messages) == 1 {
			if fds, err := unix.ParseUnixRights(&messages[0]); err == nil && len(fds) == 1 {
				f = os.NewFile(uintptr(fds[0]), "ring")
				t.Cleanup(func() { f.Close() })
			}
		}
	}
	if length > 0 {
		body := make([]byte, length)
		if _, err := io.ReadFull(conn, body); err != nil {
			t.Fatalf("the lifeline's message of %d bytes: %v", length, err)
		}
	}
	return length, f
}

// producing opens the stream on the ring as an activation would, and writes one frame into it as its unit.
func (d *delivering) producing(t *testing.T, sequence uint64, payload []byte) {
	t.Helper()
	_, address, err := d.transports.Open("acquire", 3, "station.raw", streams.Tolerances{Loss: true}, streams.Offers{Shared: true})
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: address, Net: "unixpacket"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf, oob := make([]byte, 1), make([]byte, unix.CmsgSpace(4))
	_, oobn, _, _, err := c.ReadMsgUnix(buf, oob)
	if err != nil {
		t.Fatal(err)
	}
	messages, _ := unix.ParseSocketControlMessage(oob[:oobn])
	fds, _ := unix.ParseUnixRights(&messages[0])
	f := os.NewFile(uintptr(fds[0]), "ring")
	defer f.Close()
	w, err := ring.OpenWriter(f)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := w.Publish(1000+sequence, payload); err != nil {
		t.Fatal(err)
	}
}

// read is the frames a client reading the ring finds in it, from the first.
func readRing(t *testing.T, f *os.File) []ring.Frame {
	t.Helper()
	r, err := ring.OpenReader(f)
	if err != nil {
		t.Fatalf("the descriptor is not a ring a reader opens: %v", err)
	}
	defer r.Close()
	var got []ring.Frame
	for {
		f, _, err := r.Next(100 * time.Millisecond)
		if err != nil {
			return got
		}
		got = append(got, f)
	}
}

// std: yoke:streams-delivered.11
func TestAClientThatReadsTheSharedObjectIsHandedEachRingAndAStop(t *testing.T) {
	d := deliveringAll(t, 0, streams.Offers{Shared: true}, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	reads := subscribeTo("station.raw")
	reads.GetStreamSubscribe().Reads = []interfacev1.UnitStream_Reads{interfacev1.UnitStream_READS_SHARED_OBJECT}
	got := a.answerTo(t, "s", reads).GetAnswer().GetStreamSubscribe()
	if got.GetShared() == "" {
		t.Fatalf("answered %v, not the lifeline", got.GetArrives())
	}
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: got.GetShared(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

	d.producing(t, 1, []byte("first"))
	n, f := lifeline(t, conn)
	if n != 1 || f == nil {
		t.Fatalf("at the activation the lifeline carried %d bytes and descriptor %v", n, f)
	}
	if frames := readRing(t, f); len(frames) != 1 || string(frames[0].Payload) != "first" {
		t.Errorf("the first ring holds %+v", frames)
	}
	if _, err := f.WriteAt([]byte{1}, 0); err == nil {
		t.Error("the client's descriptor of the ring can write")
	}

	d.transports.Close("acquire", "station.raw", streams.Asked)
	if n, f := lifeline(t, conn); n != 0 || f != nil {
		t.Errorf("at the stop the lifeline carried %d bytes and descriptor %v", n, f)
	}

	d.producing(t, 1, []byte("second"))
	n, g := lifeline(t, conn)
	if n != 1 || g == nil {
		t.Fatalf("at the second activation the lifeline carried %d bytes and descriptor %v", n, g)
	}
	if frames := readRing(t, g); len(frames) != 1 || string(frames[0].Payload) != "second" {
		t.Errorf("the second ring holds %+v", frames)
	}

	a.answerTo(t, "u", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamUnsubscribe{StreamUnsubscribe: &interfacev1.StreamRelease{Delivery: got.GetDelivery()}}})
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, err := conn.Read(make([]byte, 4)); err == nil {
		t.Errorf("after the release the lifeline carried %d more bytes", n)
	}
}

// std: yoke:streams-delivered.12
func TestTheRingIsWokenAfterEveryStopAndEndItsLifelineCarries(t *testing.T) {
	d := deliveringAll(t, 0, streams.Offers{Shared: true}, gate.Channel{Name: "panel", Transport: "local", Clients: "single"})
	a, _ := attachTo(t, d.bound.Address("panel"))
	a.next(t)
	reads := subscribeTo("station.raw")
	reads.GetStreamSubscribe().Reads = []interfacev1.UnitStream_Reads{interfacev1.UnitStream_READS_SHARED_OBJECT}
	got := a.answerTo(t, "s", reads).GetAnswer().GetStreamSubscribe()
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: got.GetShared(), Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)

	// woken waits for the ring's word to move from what it was, as a reader waiting on it with no bound
	// is woken, and then finds the lifeline's message already there.
	woken := func(what string, r *ring.Reader, was uint32) {
		t.Helper()
		for deadline := time.Now().Add(2 * time.Second); r.Word() == was; time.Sleep(time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("the ring was not woken at %s", what)
			}
		}
	}

	d.producing(t, 1, []byte("first"))
	_, f := lifeline(t, conn)
	r, err := ring.OpenReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	was := r.Word()
	d.transports.Close("acquire", "station.raw", streams.Asked)
	woken("the stop", r, was)
	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if n, _ := lifeline(t, conn); n != 0 {
		t.Errorf("once woken at the stop, the lifeline carried %d bytes", n)
	}

	d.producing(t, 1, []byte("second"))
	_, g := lifeline(t, conn)
	r2, err := ring.OpenReader(g)
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	was = r2.Word()
	a.answerTo(t, "u", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamUnsubscribe{StreamUnsubscribe: &interfacev1.StreamRelease{Delivery: got.GetDelivery()}}})
	woken("the release", r2, was)
	conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	if n, err := conn.Read(make([]byte, 4)); err == nil {
		t.Errorf("once woken at the release, the lifeline carried %d more bytes", n)
	}
}
