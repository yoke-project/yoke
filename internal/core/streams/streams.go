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
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Transport is what carries a stream's data.
type Transport string

const (
	// Ordered is a packet socket carrying one data envelope per packet, for a stream that tolerates nothing.
	Ordered Transport = "ordered"
	// Framed is a datagram socket carrying one frame per datagram, for a stream that tolerates loss or
	// reorder.
	Framed Transport = "framed"
)

// Tolerances are what a stream's data tolerates, as its Manifest declares them.
type Tolerances struct{ Loss, Reorder bool }

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
	Log  *slog.Logger
	// Deliver is handed every message read. Optional.
	Deliver func(Frame)
	// Publish is told that a stream stopped, and why. Optional.
	Publish func(event.Event)
}

// Service holds every stream's transport.
type Service struct {
	cfg  Config
	mu   sync.Mutex
	open map[key]*flow
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
}

// New is a service with no transport open.
func New(cfg Config) *Service {
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	return &Service{cfg: cfg, open: map[key]*flow{}}
}

// Path is where a unit's stream's transport lives.
func (s *Service) Path(unitID, stream string) string {
	return filepath.Join(s.cfg.Root, "plugins", unitID, "streams", stream+".sock")
}

// Open creates a stream's transport and listens on it, returning what it is and where. A stream already
// open is refused: activation is once per flow.
func (s *Service) Open(unitID string, incarnation uint64, stream string, t Tolerances) (Transport, string, error) {
	k := key{unitID, stream}
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
	if t.Loss || t.Reorder {
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
	s.cfg.Log.Info("stream", "unit", unitID, "stream", stream, "event", "opened", "transport", string(f.transport))
	return f.transport, address, nil
}

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
	if s.cfg.Deliver != nil {
		s.cfg.Deliver(Frame{Unit: f.unit, Stream: f.stream, Sequence: sequence, SentAt: sentAt, Payload: payload})
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
	s.mu.Unlock()
	slices.SortFunc(closing, func(a, b *flow) int { return cmp.Compare(a.stream, b.stream) })
	for _, f := range closing {
		s.remove(f, reason)
	}
}

// remove ends a flow: the socket goes first, so nothing more arrives, then the stop is published.
func (s *Service) remove(f *flow, reason string) {
	f.closer()
	if err := os.Remove(f.address); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.cfg.Log.Warn("stream", "unit", f.unit, "stream", f.stream, "error", err.Error())
	}
	s.cfg.Log.Info("stream", "unit", f.unit, "stream", f.stream, "event", "stopped", "reason", reason, "read", f.read.Load())
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

// Address is where an open stream's transport is, and empty for a stream not open.
func (s *Service) Address(unitID, stream string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, open := s.open[key{unitID, stream}]; open {
		return f.address
	}
	return ""
}
