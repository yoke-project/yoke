package interfaces

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// queued is how many frames a delivery holds for a client that reads slower than the stream flows. Past
// it frames are dropped, which the client sees as a gap in the sequence.
const queued = 1024

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

// subscribeStream grants a delivery of a unit's stream to this attachment and says where its data
// arrives: a socket in the instance's tree the client connects to, on a channel bound as a local socket;
// the attachment's own connection on one bound on an address. Subscribing to a stream not flowing is
// legal and carries nothing until it flows.
func (s *Surface) subscribeStream(r *interfacev1.Request, held *deliveries, send func(*interfacev1.CoreFrame) error) (*interfacev1.Response, *interfacev1.Refusal) {
	id, stream := r.GetStreamSubscribe().GetUnit(), r.GetStreamSubscribe().GetStream()
	if s.cfg.Units == nil || s.cfg.Declared == nil || s.cfg.Transports == nil {
		return nil, refusal("operation.unknown", "this Core does not serve stream.subscribe")
	}
	if !slices.Contains(s.cfg.Units.IDs(), id) || s.cfg.Units.Kind(id) != unit.Plugin {
		return nil, aboutUnit("subject.unknown", id, "nothing this channel may address is the unit "+id)
	}
	m, ok := s.cfg.Declared(id)
	if !ok {
		return nil, aboutUnit("subject.unknown", id, "nothing this channel may address is the unit "+id)
	}
	if !slices.ContainsFunc(m.Streams, func(d gate.Stream) bool { return d.ID == stream }) {
		return nil, aboutItem("scope.undeclared", stream, stream+" was never declared by the unit's Plugin")
	}
	var granted []string
	if s.cfg.Granted != nil {
		granted, _, _ = s.cfg.Granted(id)
	}
	if !slices.Contains(granted, stream) {
		return nil, aboutItem("scope.withheld", stream, stream+" is declared and not granted to that plugin")
	}
	n := s.cfg.Transports.NextSubscriber(id, stream)
	frames := make(chan streams.Frame, queued)
	carry := func(f streams.Frame) {
		// A channel suspended dark keeps its deliveries and they carry nothing.
		if s.cfg.Arbiter != nil {
			if suspended, grade, _, _ := s.cfg.Arbiter.State(s.cfg.Channel.Name); suspended && grade == "dark" {
				return
			}
		}
		select {
		case frames <- f:
		default:
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
	if s.cfg.Channel.Address == nil || s.cfg.Channel.Address.Class == "local" {
		path := filepath.Join(s.cfg.Root, "plugins", id, "subscribers", stream, n+".sock")
		if len(path) > ceiling {
			return nil, refusal("operation.malformed", "the subscriber's socket path "+path+" is beyond what a socket path may have")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return nil, aboutUnit("unit.no_session", id, err.Error())
		}
		listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Name: path, Net: "unixpacket"})
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
			mu.Lock()
			client = conn
			mu.Unlock()
			for {
				select {
				case <-done:
					return
				case f := <-frames:
					if _, err := conn.Write(frameOf(f)); err != nil {
						return
					}
				}
			}
		}()
		answer.Arrives = &interfacev1.Delivering_Socket{Socket: path}
	} else {
		go func() {
			for {
				select {
				case <-done:
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
