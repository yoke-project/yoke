package interfaces

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// sendBuffer is what the Core asks of each socket it writes a delivery on: room for the largest frame a
// data message may carry and its header. A packet larger than the socket's send buffer is refused, and a
// host's default is far below the bound; the kernel grants it up to net.core.wmem_max, and a frame that
// still does not fit is a write that fails. No system parameter is touched.
func sendBuffer(largest int64) int {
	if largest <= 0 {
		largest = 1 << 20
	}
	return 16 + int(largest) + 4096
}

// Queued is how many frames a delivery holds for a client that reads slower than the stream flows. Past
// it the delivery is released and the client told, rather than a frame dropped: a delivery never removes
// a guarantee the producing path gave.
const Queued = 1024

// deliveries are the deliveries one attachment holds, by their identity, each with what releases it.
type deliveries struct {
	mu      sync.Mutex
	release map[string]func()
}

func (d *deliveries) add(id string, release func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.release == nil {
		d.release = map[string]func(){}
	}
	d.release[id] = release
}

// drop releases one delivery, and says whether the attachment held it.
func (d *deliveries) drop(id string) bool {
	d.mu.Lock()
	release, held := d.release[id]
	delete(d.release, id)
	d.mu.Unlock()
	if held {
		release()
	}
	return held
}

// all releases every delivery the attachment holds: detaching leaves nothing behind.
func (d *deliveries) all() {
	d.mu.Lock()
	var ids []string
	for id := range d.release {
		ids = append(ids, id)
	}
	d.mu.Unlock()
	for _, id := range ids {
		d.drop(id)
	}
}

// frameOf is a frame as a delivery carries it: the sequence and the sender's clock, little-endian, then
// the payload.
func frameOf(f streams.Frame) []byte {
	out := make([]byte, 16, 16+len(f.Payload))
	binary.LittleEndian.PutUint64(out[0:8], f.Sequence)
	binary.LittleEndian.PutUint64(out[8:16], f.SentAt)
	return append(out, f.Payload...)
}

// lengthed is a frame as a stream socket carries it: its length, a u32 little-endian, then the frame.
func lengthed(frame []byte) []byte {
	out := make([]byte, 4, 4+len(frame))
	binary.LittleEndian.PutUint32(out, uint32(len(frame)))
	return append(out, frame...)
}

// deliverRing answers a delivery whose client reads the ring itself. Its per-subscriber socket is the
// delivery's lifeline, a stream socket each message of which follows its length: at each activation one
// byte carrying a descriptor of that activation's ring, opened for reading alone; at each stop, a message of
// length zero. Its closing ends the delivery. A write that fails ends it as a slow client's end does. After
// a stop or the end, the ring is woken: its reader waits on it with no bound, and reads its lifeline then.
func (s *Surface) deliverRing(id, stream, n string, answer *interfacev1.Delivering, held *deliveries, release func(), closer *func()) (*interfacev1.Response, *interfacev1.Refusal) {
	path := filepath.Join(s.cfg.Root, "plugins", id, "subscribers", stream, n+".sock")
	if len(path) > ceiling {
		return nil, refusal("operation.malformed", "the subscriber's socket path "+path+" is beyond what a socket path may have")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, aboutUnit("unit.no_session", id, err.Error())
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, aboutUnit("unit.no_session", id, err.Error())
	}
	listener.SetUnlinkOnClose(false)
	var mu sync.Mutex
	var conn *net.UnixConn
	var current *os.File
	failed := false
	// send writes one message on the lifeline, the lock held: the ring's descriptor, or a stop.
	send := func(ring *os.File) {
		if conn == nil || failed {
			return
		}
		message, rights := []byte{0, 0, 0, 0}, []byte(nil)
		if ring != nil {
			message, rights = []byte{1, 0, 0, 0, 0}, unix.UnixRights(int(ring.Fd()))
		}
		if _, _, err := conn.WriteMsgUnix(message, rights, nil); err != nil {
			failed = true
			s.publish(event.DeliveryFailed(s.cfg.Channel.Name, id, stream, n, err.Error()))
			go held.drop(n)
		}
	}
	opened := func(f *os.File) {
		mu.Lock()
		defer mu.Unlock()
		if current != nil {
			current.Close()
		}
		current = f
		send(f)
	}
	stopped := func() {
		mu.Lock()
		defer mu.Unlock()
		if current != nil {
			current.Close()
			current = nil
		}
		send(nil)
	}
	*closer = func() {
		listener.Close()
		mu.Lock()
		if conn != nil {
			conn.Close()
		}
		mapped := current != nil
		if current != nil {
			current.Close()
			current = nil
		}
		mu.Unlock()
		if mapped {
			s.cfg.Transports.WakeRing(id, stream)
		}
		os.Remove(path)
	}
	go func() {
		c, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		conn = c
		if current != nil {
			send(current)
		}
	}()
	answer.Arrives = &interfacev1.Delivering_Shared{Shared: path}
	stop := s.cfg.Transports.FeedRing(id, stream, opened, stopped, func() { held.drop(n) })
	held.add(n, func() { stop(); release() })
	return &interfacev1.Response{Answer: &interfacev1.Response_StreamSubscribe{StreamSubscribe: answer}}, nil
}

// subscribeStream grants a delivery of a unit's stream to this attachment and says where its data
// arrives: a socket in the instance's tree the client connects to, on a channel bound as a local socket;
// the attachment's own connection on one bound on an address. Subscribing to a stream not flowing is
// legal and carries nothing until it flows.
func (s *Surface) subscribeStream(r *interfacev1.Request, a *attachment) (*interfacev1.Response, *interfacev1.Refusal) {
	held, send := a.held, a.live
	id, stream := r.GetStreamSubscribe().GetUnit(), r.GetStreamSubscribe().GetStream()
	if s.cfg.Units == nil || s.cfg.Declared == nil || s.cfg.Transports == nil {
		return nil, refusal("operation.unknown", "this Core does not serve stream.subscribe")
	}
	if !slices.Contains(s.cfg.Units.IDs(), id) || s.cfg.Units.Kind(id) != unit.Plugin || (s.cfg.Use != nil && id != s.cfg.Use.Bound) {
		return nil, aboutUnit("subject.unknown", id, "nothing this channel may address is the unit "+id)
	}
	if s.cfg.Use != nil && !slices.Contains(s.cfg.Use.Streams, stream) {
		return nil, aboutItem("use.undeclared", stream, "the need "+s.cfg.Use.Need+" of "+s.cfg.Use.Unit+" does not use the stream "+stream)
	}
	m, ok := s.cfg.Declared(id)
	if !ok {
		return nil, aboutUnit("subject.unknown", id, "nothing this channel may address is the unit "+id)
	}
	at := slices.IndexFunc(m.Streams, func(d gate.Stream) bool { return d.ID == stream })
	if at < 0 {
		return nil, aboutItem("scope.undeclared", stream, stream+" was never declared by the unit's Plugin")
	}
	largest := m.Streams[at].MaxFrame
	var granted []string
	if s.cfg.Granted != nil {
		granted, _, _ = s.cfg.Granted(id)
	}
	if !slices.Contains(granted, stream) {
		return nil, aboutItem("scope.withheld", stream, stream+" is declared and not granted to that plugin")
	}
	n := s.cfg.Transports.NextSubscriber(id, stream)
	size := s.cfg.Queue
	if size == 0 {
		size = Queued
	}
	frames := make(chan streams.Frame, size)
	// Once behind, the delivery accepts nothing more: the release is asynchronous, and a frame accepted
	// while it lands would reach the client after the one dropped.
	var behind atomic.Bool
	carry := func(f streams.Frame) {
		if behind.Load() {
			return
		}
		// A channel suspended dark keeps its deliveries and they carry nothing.
		if s.cfg.Arbiter != nil {
			if suspended, grade, _, _ := s.cfg.Arbiter.State(s.cfg.Channel.Name); suspended && grade == "dark" {
				return
			}
		}
		select {
		case frames <- f:
		default:
			// Behind: released, so the client learns it rather than meeting a gap.
			behind.Store(true)
			go held.drop(n)
		}
	}
	done := make(chan struct{})
	var once sync.Once
	var closer func()
	release := func() {
		once.Do(func() {
			close(done)
			if closer != nil {
				closer()
			}
		})
	}
	answer := &interfacev1.Delivering{Delivery: n, Flowing: slices.Contains(s.cfg.Transports.Active(id), stream)}
	// A client on the host whose library reads the shared object, of a stream carried on the ring, reads
	// the ring itself: the Core hands it each activation's ring and says each stop, and carries nothing.
	local := s.cfg.Channel.Transport != "http+ws" && (s.cfg.Channel.Address == nil || s.cfg.Channel.Address.Class == "local")
	if local && slices.Contains(r.GetStreamSubscribe().GetReads(), interfacev1.UnitStream_READS_SHARED_OBJECT) &&
		m.Streams[at].ToleratesLoss && s.cfg.Sessions != nil && s.cfg.Sessions.Offers(id).Shared {
		return s.deliverRing(id, stream, n, answer, held, release, &closer)
	}
	if s.cfg.Channel.Transport == "http+ws" {
		// The browser projection's delivery is one binary WebSocket, at the path the answer names.
		path := "/v1/streams/" + id + "/" + stream + "/" + n
		s.web.mu.Lock()
		s.web.deliveries[path] = &webDelivery{frames: frames, done: done, owner: a}
		s.web.mu.Unlock()
		closer = func() {
			s.web.mu.Lock()
			delete(s.web.deliveries, path)
			s.web.mu.Unlock()
		}
		answer.Arrives = &interfacev1.Delivering_Path{Path: path}
	} else if s.cfg.Channel.Address == nil || s.cfg.Channel.Address.Class == "local" {
		path := filepath.Join(s.cfg.Root, "plugins", id, "subscribers", stream, n+".sock")
		if len(path) > ceiling {
			return nil, refusal("operation.malformed", "the subscriber's socket path "+path+" is beyond what a socket path may have")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, aboutUnit("unit.no_session", id, err.Error())
		}
		// A library that reads a stream socket is delivered on one, each frame after its length; any other
		// on the packet socket every library reads.
		streamed := slices.Contains(r.GetStreamSubscribe().GetReads(), interfacev1.UnitStream_READS_STREAM_SOCKET)
		network := "unixpacket"
		if streamed {
			network = "unix"
		}
		listener, err := net.ListenUnix(network, &net.UnixAddr{Name: path, Net: network})
		if err != nil {
			return nil, aboutUnit("unit.no_session", id, err.Error())
		}
		listener.SetUnlinkOnClose(false)
		var mu sync.Mutex
		var client net.Conn
		closer = func() {
			listener.Close()
			mu.Lock()
			if client != nil {
				client.Close()
			}
			mu.Unlock()
			os.Remove(path)
		}
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			if unix, ok := conn.(*net.UnixConn); ok {
				unix.SetWriteBuffer(sendBuffer(largest))
			}
			mu.Lock()
			client = conn
			mu.Unlock()
			for {
				select {
				case <-done:
					return
				case f := <-frames:
					out := frameOf(f)
					if streamed {
						out = lengthed(out)
					}
					if _, err := conn.Write(out); err != nil {
						// A delivery that cannot be written to ends as a slow client's does: released, its
						// socket closed, so the client learns it, and the reason said.
						behind.Store(true)
						s.publish(event.DeliveryFailed(s.cfg.Channel.Name, id, stream, n, err.Error()))
						go held.drop(n)
						return
					}
				}
			}
		}()
		answer.Arrives = &interfacev1.Delivering_Socket{Socket: path}
		if streamed {
			answer.Arrives = &interfacev1.Delivering_StreamSocket{StreamSocket: path}
		}
	} else {
		go func() {
			for {
				select {
				case <-done:
					// A delivery on the connection ends as a call does: completed by the Core.
					send(&interfacev1.CoreFrame{Call: n, Carries: &interfacev1.CoreFrame_Completion{Completion: &interfacev1.Completion{By: interfacev1.Completion_BY_CORE}}})
					return
				case f := <-frames:
					send(&interfacev1.CoreFrame{Call: n, Carries: &interfacev1.CoreFrame_Delivery{Delivery: &interfacev1.StreamDelivery{
						Delivery: n, Sequence: f.Sequence, SentAt: f.SentAt, Payload: f.Payload}}})
				}
			}
		}()
		answer.Arrives = &interfacev1.Delivering_Connection{Connection: true}
	}
	stop := s.cfg.Transports.Feed(id, stream, carry, func() { held.drop(n) })
	held.add(n, func() { stop(); release() })
	return &interfacev1.Response{Answer: &interfacev1.Response_StreamSubscribe{StreamSubscribe: answer}}, nil
}
