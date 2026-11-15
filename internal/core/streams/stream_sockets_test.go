package streams_test

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/streams"
)

// prefixed is a message after its length, as a stream socket carries it.
func prefixed(message []byte) []byte {
	out := make([]byte, 4, 4+len(message))
	binary.LittleEndian.PutUint32(out, uint32(len(message)))
	return append(out, message...)
}

// std: yoke:a-streams-transport.12
func TestALibraryThatOffersTheStreamSocketHasItsStreamCarriedOnOne(t *testing.T) {
	b := newBench(t)
	offers := streams.Offers{OrderedStream: true, FramedStream: true}
	big := bytes.Repeat([]byte{7}, 600<<10)
	for _, c := range []struct {
		stream    string
		tolerates streams.Tolerances
		transport streams.Transport
		activate  pluginv1.Control_Activate_Transport
		message   func(sequence uint64, payload []byte) []byte
	}{
		{"station.spectra", streams.Tolerances{}, streams.OrderedStream, pluginv1.Control_Activate_TRANSPORT_ORDERED_STREAM,
			func(sequence uint64, payload []byte) []byte {
				return envelope(t, &pluginv1.Envelope{MessageId: "u", SentAtUnixNano: int64(1000 + sequence),
					Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: sequence, Payload: payload}}})
			}},
		{"station.preview", streams.Tolerances{Loss: true}, streams.FramedStream, pluginv1.Control_Activate_TRANSPORT_FRAMED_STREAM,
			func(sequence uint64, payload []byte) []byte { return frame(sequence, 1000+sequence, payload) }},
	} {
		before := len(b.read(t, 0))
		transport, address, err := b.svc.Open("acquire", 1, c.stream, c.tolerates, offers)
		if err != nil {
			t.Fatalf("%s did not open: %v", c.stream, err)
		}
		if transport != c.transport || transport.Activation() != c.activate {
			t.Errorf("%s opened as %s, activated as %v; want %s, %v", c.stream, transport, transport.Activation(), c.transport, c.activate)
		}
		conn, err := net.Dial("unix", address)
		if err != nil {
			t.Fatalf("%s is not a stream socket: %v", c.stream, err)
		}
		defer conn.Close()
		for i, payload := range [][]byte{big, {1}, {2}} {
			if _, err := conn.Write(prefixed(c.message(uint64(i+1), payload))); err != nil {
				t.Fatalf("%s: writing message %d: %v", c.stream, i+1, err)
			}
		}
		got := b.read(t, before+3)[before:]
		if len(got) != 3 {
			t.Fatalf("%s: %d messages were read", c.stream, len(got))
		}
		for i, f := range got {
			n := uint64(i + 1)
			if f.Stream != c.stream || f.Sequence != n || f.SentAt != 1000+n {
				t.Errorf("%s: message %d is sequence %d at %d", c.stream, n, f.Sequence, f.SentAt)
			}
		}
		if len(got[0].Payload) != len(big) || got[1].Payload[0] != 1 || got[2].Payload[0] != 2 {
			t.Errorf("%s: the payloads read are %d, %v, %v bytes", c.stream, len(got[0].Payload), got[1].Payload, got[2].Payload)
		}
		// A length beyond the bound ends the transport: nothing after it is read.
		huge := make([]byte, 4)
		binary.LittleEndian.PutUint32(huge, 1<<30)
		conn.Write(huge)
		conn.Write(prefixed(c.message(4, []byte{4})))
		time.Sleep(100 * time.Millisecond)
		if n := len(b.read(t, 0)); n != before+3 {
			t.Errorf("%s: after a length beyond the bound, %d more were read", c.stream, n-before-3)
		}
	}
}

// std: yoke:a-streams-transport.13
func TestALibraryThatOffersNoStreamSocketKeepsThePacketSockets(t *testing.T) {
	b := newBench(t)
	for _, c := range []struct {
		stream    string
		tolerates streams.Tolerances
		transport streams.Transport
		network   string
	}{
		{"station.spectra", streams.Tolerances{}, streams.Ordered, "unixpacket"},
		{"station.preview", streams.Tolerances{Loss: true}, streams.Framed, "unixgram"},
	} {
		transport, address, err := b.svc.Open("acquire", 1, c.stream, c.tolerates, streams.Offers{})
		if err != nil {
			t.Fatal(err)
		}
		if transport != c.transport || socketType(t, address) != c.network {
			t.Errorf("%s opened as %s on a %s socket, want %s on %s", c.stream, transport, socketType(t, address), c.transport, c.network)
		}
	}
}
