// Package streams holds a stream's own transport: created and listened on by the Core before the stream
// is activated, chosen by the two tolerances the stream declares, read for as long as the stream flows,
// and removed on each of the three routes that end a stream. A stream that was never activated has no
// transport, so a unit that was never told to emit has nowhere to emit into.
package streams

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/ring"
)

// Transport is what carries a stream's data.
type Transport string

const (
	// Ordered is a packet socket carrying one data envelope per packet, for a stream that tolerates nothing.
	Ordered Transport = "ordered"
	// Framed is a datagram socket carrying one frame per datagram, for a stream that tolerates loss or
	// reorder.
	Framed Transport = "framed"
	// Shared is the shared ring, for a stream that tolerates loss where the unit's library offers it.
	Shared Transport = "shared"
	// OrderedStream is the ordered transport on a stream socket, each envelope after its length, where
	// the unit's library offers it.
	OrderedStream Transport = "ordered-stream"
	// FramedStream is the framed transport on a stream socket, each frame after its length, where the
	// unit's library offers it.
	FramedStream Transport = "framed-stream"
)

// Offers are the transports a unit's library offered at its registration beyond the ordered and the
// framed, which every library speaks. The Core uses one only for a library that offered it.
type Offers struct {
	Shared, OrderedStream, FramedStream bool
}

// OffersOf reads what a registration offered.
func OffersOf(offered []pluginv1.Control_Activate_Transport) Offers {
	var o Offers
	for _, t := range offered {
		switch t {
		case pluginv1.Control_Activate_TRANSPORT_SHARED_OBJECT:
			o.Shared = true
		case pluginv1.Control_Activate_TRANSPORT_ORDERED_STREAM:
			o.OrderedStream = true
		case pluginv1.Control_Activate_TRANSPORT_FRAMED_STREAM:
			o.FramedStream = true
		}
	}
	return o
}

// Activation is the transport as an activation names it to the unit.
func (t Transport) Activation() pluginv1.Control_Activate_Transport {
	switch t {
	case Framed:
		return pluginv1.Control_Activate_TRANSPORT_FRAMED
	case Shared:
		return pluginv1.Control_Activate_TRANSPORT_SHARED_OBJECT
	case OrderedStream:
		return pluginv1.Control_Activate_TRANSPORT_ORDERED_STREAM
	case FramedStream:
		return pluginv1.Control_Activate_TRANSPORT_FRAMED_STREAM
	}
	return pluginv1.Control_Activate_TRANSPORT_ORDERED
}

// Tolerances are what a stream's data tolerates, as its Manifest declares them.
type Tolerances struct {
	Loss, Reorder bool
	// MaxFrame is the largest payload the stream declares it emits; zero is the default, 1 MiB.
	MaxFrame int64
}

// largest is the stream's largest payload, the default where it declares none.
func (t Tolerances) largest() int64 {
	if t.MaxFrame <= 0 {
		return 1 << 20
	}
	return t.MaxFrame
}

// The reasons a stream stops.
const (
	Asked        = "asked"
	SessionEnded = "session ended"
	UnitExited   = "unit exited"
)

// header is a frame's: the sequence and the sender's clock, little-endian.
const header = 16

// packet is the largest packet or datagram read: a data message bounded at 1 MiB, with room for its
// envelope.
const packet = 1<<20 + 4096

// Frame is one message read from a stream's transport.
type Frame struct {
	Unit, Stream     string
	Sequence, SentAt uint64
	Payload          []byte
}

// Config is where the transports live and whom the service tells.
type Config struct {
	// Root is the instance root: every transport is at plugins/<unit>/streams/<stream>.sock under it.
	Root string
	// Instance is the instance's name, which a ring's label derives from.
	Instance string
	Log      *slog.Logger
	// Deliver is handed every message read. Optional.
	Deliver func(Frame)
	// Publish is told that a stream stopped, and why. Optional.
	Publish func(event.Event)
}

// Service holds every stream's transport.
type Service struct {
	cfg      Config
	mu       sync.Mutex
	open     map[key]*flow
	feeds    map[*feed]bool
	counters map[key]uint64
}

// feed is one consumer of a unit's stream, across its stops and starts, until its Session ends.
type feed struct {
	key
	to       func(Frame)
	released func()
	// opened and stopped make a feed of the ring itself rather than of its frames: handed a descriptor
	// of each activation's ring, opened for reading alone, and told of each stop. Such a feed is given
	// no frame.
	opened  func(*os.File)
	stopped func()
}

type key struct{ unit, stream string }

// flow is one stream's transport, while the stream may flow.
type flow struct {
	key
	incarnation uint64
	transport   Transport
	tolerates   Tolerances
	address     string
	closer      func()
	read        atomic.Uint64
	// relayed counts the frames among them that were copied for something to take; on a ring the rest
	// were passed unread.
	relayed atomic.Uint64
	// ring is the stream's ring, where it is carried on one.
	ring *ring.Ring
}

// New is a service with no transport open.
func New(cfg Config) *Service {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{cfg: cfg, open: map[key]*flow{}, feeds: map[*feed]bool{}, counters: map[key]uint64{}}
}

// Path is where a unit's stream's transport lives.
func (s *Service) Path(unitID, stream string) string {
	return filepath.Join(s.cfg.Root, "plugins", unitID, "streams", stream+".sock")
}

// Open creates a stream's transport and listens on it, returning what it is and where. A stream already
// open is refused: activation is once per flow.
func (s *Service) Open(unitID string, incarnation uint64, stream string, t Tolerances, offers Offers) (Transport, string, error) {
	k := key{unitID, stream}
	// What a feed of the ring is told runs once the lock is let go, so a feed may write to its client.
	var after []func()
	defer func() {
		for _, run := range after {
			run()
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, open := s.open[k]; open {
		return "", "", fmt.Errorf("the stream %s of %s is already open", stream, unitID)
	}
	address := s.Path(unitID, stream)
	if err := os.MkdirAll(filepath.Dir(address), 0o750); err != nil {
		return "", "", err
	}
	os.Remove(address)
	f := &flow{key: k, incarnation: incarnation, tolerates: t, address: address}
	if t.Loss && offers.Shared {
		if err := s.openShared(f); err != nil {
			return "", "", err
		}
	} else if (t.Loss || t.Reorder) && offers.FramedStream {
		f.transport = FramedStream
		if err := s.openStream(f, s.readFramedStream); err != nil {
			return "", "", err
		}
	} else if !t.Loss && !t.Reorder && offers.OrderedStream {
		f.transport = OrderedStream
		if err := s.openStream(f, s.readOrderedStream); err != nil {
			return "", "", err
		}
	} else if t.Loss || t.Reorder {
		f.transport = Framed
		conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: address, Net: "unixgram"})
		if err != nil {
			return "", "", err
		}
		f.closer = func() { conn.Close() }
		go s.readFrames(f, conn)
	} else {
		f.transport = Ordered
		listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: address, Net: "unixpacket"})
		if err != nil {
			return "", "", err
		}
		var mu sync.Mutex
		var held net.Conn
		f.closer = func() {
			listener.Close()
			mu.Lock()
			if held != nil {
				held.Close()
			}
			mu.Unlock()
		}
		go s.acceptOrdered(f, listener, func(c net.Conn) bool {
			mu.Lock()
			defer mu.Unlock()
			if held != nil {
				return false
			}
			held = c
			return true
		})
	}
	s.open[k] = f
	if f.ring != nil {
		for fd := range s.feeds {
			if fd.key == k && fd.opened != nil {
				if readable, err := f.ring.ReaderFile(); err == nil {
					opened := fd.opened
					after = append(after, func() { opened(readable) })
				}
			}
		}
	}
	s.cfg.Log.Info("stream", "unit", unitID, "stream", stream, "event", "opened", "transport", string(f.transport))
	return f.transport, address, nil
}

// openShared carries a stream on a shared ring: the Core creates the ring and a packet socket at the
// stream's path, hands the ring's descriptor that writes to the first connection — one byte carrying it —
// and keeps that connection for the life of the stream; it reads the ring itself and delivers what it
// reads. Closing the socket ends the stream, and the memory goes when the last descriptor on it closes.
func (s *Service) openShared(f *flow) error {
	slots, capacity := ring.Geometry(f.tolerates.largest())
	r, err := ring.CreateWith(fmt.Sprintf("yoke-%s-%s-%s", s.cfg.Instance, f.unit, f.stream), slots, capacity)
	if err != nil {
		return err
	}
	readable, err := r.ReaderFile()
	if err != nil {
		r.Close()
		return err
	}
	reader, err := ring.OpenReader(readable)
	readable.Close()
	if err != nil {
		r.Close()
		return err
	}
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: f.address, Net: "unixpacket"})
	if err != nil {
		reader.Close()
		r.Close()
		return err
	}
	f.transport = Shared
	f.ring = r
	done := make(chan struct{})
	var mu sync.Mutex
	var held net.Conn
	var once sync.Once
	f.closer = func() {
		once.Do(func() {
			close(done)
			listener.Close()
			mu.Lock()
			if held != nil {
				held.Close()
			}
			mu.Unlock()
			r.Close()
		})
	}
	go func() {
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			mu.Lock()
			first := held == nil
			if first {
				held = conn
			}
			mu.Unlock()
			if !first {
				s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "refused", "a second connection")
				conn.Close()
				continue
			}
			writable, err := r.ProducerFile()
			if err != nil {
				conn.Close()
				continue
			}
			_, _, err = conn.WriteMsgUnix([]byte{0}, unix.UnixRights(int(writable.Fd())), nil)
			writable.Close()
			if err != nil {
				s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "fault", "the ring's descriptor was not handed over", "error", err)
			}
		}
	}()
	go s.readRing(f, reader, done)
	return nil
}

// readRing reads a ring frame by frame until the stream ends, delivering each frame read; a gap is what a
// stream that tolerates loss tolerates. While nothing is relayed from the stream — its every consumer
// reads the ring itself, or it has none — the frames are passed and not read: the Core copies a frame
// for a delivery it carries, and for nothing else.
func (s *Service) readRing(f *flow, r *ring.Reader, done <-chan struct{}) {
	defer r.Close()
	for {
		select {
		case <-done:
			return
		default:
		}
		if !s.relays(f) {
			if passed, err := r.Pass(ringInterval); err == nil {
				f.read.Add(passed)
			}
			continue
		}
		frame, _, err := r.Next(ringInterval)
		if err != nil {
			continue
		}
		s.deliver(f, frame.Sequence, frame.Clock, frame.Payload)
	}
}

// relays says whether a frame read from a stream would be handed to anything.
func (s *Service) relays(f *flow) bool {
	if s.cfg.Deliver != nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for fd := range s.feeds {
		if fd.key == f.key && fd.to != nil {
			return true
		}
	}
	return false
}

// ringInterval bounds a wait on a ring's wake word, so that a stream's end is noticed.
const ringInterval = 100 * time.Millisecond

// acceptOrdered reads the first connection to an ordered transport and closes every later one: one
// stream has one producer, and a second connection would be a second claim to it.
func (s *Service) acceptOrdered(f *flow, listener *net.UnixListener, first func(net.Conn) bool) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		if !first(conn) {
			s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "refused", "a second connection")
			conn.Close()
			continue
		}
		go s.readOrdered(f, conn)
	}
}

// openStream listens on a stream socket at the stream's path, keeps the first connection for the life of the
// stream and refuses any other, and reads it with read.
func (s *Service) openStream(f *flow, read func(*flow, net.Conn)) error {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: f.address, Net: "unix"})
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var held net.Conn
	f.closer = func() {
		listener.Close()
		mu.Lock()
		if held != nil {
			held.Close()
		}
		mu.Unlock()
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			first := held == nil
			if first {
				held = conn
			}
			mu.Unlock()
			if !first {
				s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "refused", "a second connection")
				conn.Close()
				continue
			}
			go read(f, conn)
		}
	}()
	return nil
}

// message reads one message from a stream socket: its length, a u32 little-endian, then that many bytes.
// A length beyond the bound is not read: the connection is closed, since nothing after it can be found.
func (s *Service) message(f *flow, conn net.Conn, buf []byte) ([]byte, bool) {
	var length [4]byte
	if _, err := io.ReadFull(conn, length[:]); err != nil {
		return nil, false
	}
	n := int(binary.LittleEndian.Uint32(length[:]))
	if n > len(buf) {
		s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "fault", "a message longer than the bound", "bytes", n, "bound", len(buf))
		conn.Close()
		return nil, false
	}
	if _, err := io.ReadFull(conn, buf[:n]); err != nil {
		return nil, false
	}
	return buf[:n], true
}

// readOrderedStream reads one data envelope per message, in order.
func (s *Service) readOrderedStream(f *flow, conn net.Conn) {
	buf := make([]byte, f.tolerates.largest()+4096)
	for {
		m, ok := s.message(f, conn, buf)
		if !ok {
			return
		}
		e := &pluginv1.Envelope{}
		if err := proto.Unmarshal(m, e); err != nil || e.GetData() == nil {
			s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "refused", "a message that is not a data envelope", "message", e.GetMessageId())
			continue
		}
		s.deliver(f, e.GetData().GetSequence(), uint64(e.GetSentAtUnixNano()), slices.Clone(e.GetData().GetPayload()))
	}
}

// readFramedStream reads one frame per message, as readFrames does per datagram.
func (s *Service) readFramedStream(f *flow, conn net.Conn) {
	buf := make([]byte, f.tolerates.largest()+4096)
	var last uint64
	for {
		m, ok := s.message(f, conn, buf)
		if !ok {
			return
		}
		if len(m) < header {
			s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "refused", "a message shorter than a frame's header", "bytes", len(m))
			continue
		}
		sequence := binary.LittleEndian.Uint64(m[0:8])
		if !f.tolerates.Loss && last != 0 && sequence > last+1 {
			s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "fault", "a gap in the sequence of a stream that does not tolerate loss",
				"after", last, "sequence", sequence)
		}
		last = max(last, sequence)
		s.deliver(f, sequence, binary.LittleEndian.Uint64(m[8:16]), slices.Clone(m[header:]))
	}
}

// readOrdered reads one data envelope per packet, in order.
func (s *Service) readOrdered(f *flow, conn net.Conn) {
	buf := make([]byte, packet)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		e := &pluginv1.Envelope{}
		if err := proto.Unmarshal(buf[:n], e); err != nil || e.GetData() == nil {
			s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "refused", "a packet that is not a data envelope", "message", e.GetMessageId())
			continue
		}
		s.deliver(f, e.GetData().GetSequence(), uint64(e.GetSentAtUnixNano()), slices.Clone(e.GetData().GetPayload()))
	}
}

// readFrames reads one frame per datagram. A gap is a fault where the stream does not tolerate loss; a
// lower sequence is reorder, which a stream on this transport declared it tolerates.
func (s *Service) readFrames(f *flow, conn *net.UnixConn) {
	buf := make([]byte, packet)
	var last uint64
	for {
		n, _, err := conn.ReadFromUnix(buf)
		if err != nil {
			return
		}
		if n < header {
			s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "refused", "a datagram shorter than a frame's header", "bytes", n)
			continue
		}
		sequence := binary.LittleEndian.Uint64(buf[0:8])
		if !f.tolerates.Loss && last != 0 && sequence > last+1 {
			s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "fault", "a gap in the sequence of a stream that does not tolerate loss",
				"after", last, "sequence", sequence)
		}
		last = max(last, sequence)
		s.deliver(f, sequence, binary.LittleEndian.Uint64(buf[8:16]), slices.Clone(buf[header:n]))
	}
}

func (s *Service) deliver(f *flow, sequence, sentAt uint64, payload []byte) {
	f.read.Add(1)
	f.relayed.Add(1)
	frame := Frame{Unit: f.unit, Stream: f.stream, Sequence: sequence, SentAt: sentAt, Payload: payload}
	if s.cfg.Deliver != nil {
		s.cfg.Deliver(frame)
	}
	s.mu.Lock()
	var to []func(Frame)
	for fd := range s.feeds {
		if fd.key == f.key && fd.to != nil {
			to = append(to, fd.to)
		}
	}
	s.mu.Unlock()
	for _, send := range to {
		send(frame)
	}
}

// Close removes a stream's transport and says why, returning whether it was open.
func (s *Service) Close(unitID, stream, reason string) bool {
	s.mu.Lock()
	f, open := s.open[key{unitID, stream}]
	delete(s.open, key{unitID, stream})
	s.mu.Unlock()
	if !open {
		return false
	}
	s.remove(f, reason)
	return true
}

// Discard removes a transport through which nothing ever flowed, publishing nothing: an activation the
// unit refused or never received.
func (s *Service) Discard(unitID, stream string) {
	s.mu.Lock()
	f, open := s.open[key{unitID, stream}]
	delete(s.open, key{unitID, stream})
	s.mu.Unlock()
	if open {
		f.closer()
		os.Remove(f.address)
		s.cfg.Log.Info("stream", "unit", unitID, "stream", stream, "event", "discarded")
	}
}

// CloseAll removes every transport of a unit, for the reason given.
func (s *Service) CloseAll(unitID, reason string) {
	s.mu.Lock()
	var closing []*flow
	for k, f := range s.open {
		if k.unit == unitID {
			closing = append(closing, f)
			delete(s.open, k)
		}
	}
	// A Session that ends, or a life, takes every delivery of the unit's streams with it; a stream stopped
	// when asked keeps them, carrying nothing.
	var released []*feed
	if reason != Asked {
		for fd := range s.feeds {
			if fd.unit == unitID {
				released = append(released, fd)
				delete(s.feeds, fd)
			}
		}
	}
	s.mu.Unlock()
	slices.SortFunc(closing, func(a, b *flow) int { return cmp.Compare(a.stream, b.stream) })
	for _, f := range closing {
		s.remove(f, reason)
	}
	for _, fd := range released {
		if fd.released != nil {
			fd.released()
		}
	}
}

// remove ends a flow: the socket goes first, so nothing more arrives, then the stop is published.
func (s *Service) remove(f *flow, reason string) {
	s.mu.Lock()
	var stopped []func()
	for fd := range s.feeds {
		if fd.key == f.key && fd.stopped != nil {
			stopped = append(stopped, fd.stopped)
		}
	}
	s.mu.Unlock()
	for _, stop := range stopped {
		stop()
	}
	f.closer()
	if err := os.Remove(f.address); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "error", err.Error())
	}
	s.cfg.Log.Info("stream", "unit", f.unit, "stream", f.stream, "event", "stopped", "reason", reason, "read", f.read.Load(), "relayed", f.relayed.Load())
	if s.cfg.Publish != nil {
		s.cfg.Publish(event.StreamStopped(f.unit, f.incarnation, f.stream, reason, reason == Asked))
	}
}

// Active are a unit's streams whose transport is open, in the order of their identities.
func (s *Service) Active(unitID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.open {
		if k.unit == unitID {
			out = append(out, k.stream)
		}
	}
	slices.Sort(out)
	return out
}

// Feed hands every message read from a unit's stream to the function given, from now on and across the
// stream stopping and starting again, until the stop it returns is called; released is told when the
// unit's Session ends or the unit exits, which ends the feed.
func (s *Service) Feed(unitID, stream string, to func(Frame), released func()) (stop func()) {
	fd := &feed{key: key{unitID, stream}, to: to, released: released}
	s.mu.Lock()
	s.feeds[fd] = true
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.feeds, fd)
		s.mu.Unlock()
	}
}

// FeedRing is a consumer of a unit's stream that reads its ring itself: handed a descriptor of each
// activation's ring opened for reading alone — at once, where the stream flows on one — and told of each
// stop, across stops and starts, until the unit's Session ends, when it is released. It is given no frame:
// the Core carries nothing to it.
func (s *Service) FeedRing(unitID, stream string, opened func(*os.File), stopped func(), released func()) (stop func()) {
	fd := &feed{key: key{unitID, stream}, released: released, opened: opened, stopped: stopped}
	s.mu.Lock()
	s.feeds[fd] = true
	var readable *os.File
	if f, open := s.open[fd.key]; open && f.ring != nil {
		readable, _ = f.ring.ReaderFile()
	}
	s.mu.Unlock()
	if readable != nil {
		opened(readable)
	}
	return func() {
		s.mu.Lock()
		delete(s.feeds, fd)
		s.mu.Unlock()
	}
}

// NextSubscriber is the next subscriber's identity for a unit's stream: a counter, decimal and
// zero-padded to eight digits, never reused within the life of the instance.
func (s *Service) NextSubscriber(unitID, stream string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[key{unitID, stream}]++
	return fmt.Sprintf("%08d", s.counters[key{unitID, stream}])
}

// Address is where an open stream's transport is, and empty for a stream not open.
func (s *Service) Address(unitID, stream string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, open := s.open[key{unitID, stream}]; open {
		return f.address
	}
	return ""
}
