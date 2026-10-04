package streams_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/streams"
)

type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuffer) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// bench is a transport service over a short instance root, recording what it reads and publishes.
type bench struct {
	svc       *streams.Service
	root      string
	log       *syncBuffer
	mu        sync.Mutex
	frames    []streams.Frame
	published []event.Event
}

func newBench(t *testing.T) *bench {
	t.Helper()
	root, _ := os.MkdirTemp("", "yt")
	t.Cleanup(func() { os.RemoveAll(root) })
	b := &bench{root: root, log: &syncBuffer{}}
	b.svc = streams.New(streams.Config{
		Root: root,
		Log:  slog.New(slog.NewTextHandler(b.log, nil)),
		Deliver: func(f streams.Frame) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.frames = append(b.frames, f)
		},
		Publish: func(e event.Event) {
			b.mu.Lock()
			defer b.mu.Unlock()
			b.published = append(b.published, e)
		},
	})
	t.Cleanup(func() { b.svc.CloseAll("acquire", streams.UnitExited); b.svc.CloseAll("archive", streams.UnitExited) })
	return b
}

// read waits for n frames to have been read, and returns them.
func (b *bench) read(t *testing.T, n int) []streams.Frame {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		b.mu.Lock()
		got := slices.Clone(b.frames)
		b.mu.Unlock()
		if len(got) >= n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (b *bench) events() []event.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return slices.Clone(b.published)
}

func socketType(t *testing.T, path string) string {
	t.Helper()
	for _, network := range []string{"unixpacket", "unixgram"} {
		if network == "unixgram" {
			c, err := net.DialUnix(network, nil, &net.UnixAddr{Name: path, Net: network})
			if err == nil {
				c.Close()
				return network
			}
			continue
		}
		if c, err := net.Dial(network, path); err == nil {
			c.Close()
			return network
		}
	}
	return "none"
}

// std: yoke:a-streams-transport.01
func TestTheTransportIsChosenByTheTwoTolerances(t *testing.T) {
	b := newBench(t)
	for _, c := range []struct {
		stream    string
		tolerates streams.Tolerances
		transport streams.Transport
		network   string
	}{
		{"station.spectra", streams.Tolerances{}, streams.Ordered, "unixpacket"},
		{"station.preview", streams.Tolerances{Loss: true}, streams.Framed, "unixgram"},
		{"station.events", streams.Tolerances{Reorder: true}, streams.Framed, "unixgram"},
	} {
		transport, address, err := b.svc.Open("acquire", 1, c.stream, c.tolerates)
		if err != nil {
			t.Fatalf("%s did not open: %v", c.stream, err)
		}
		want := filepath.Join(b.root, "plugins", "acquire", "streams", c.stream+".sock")
		if transport != c.transport || address != want {
			t.Errorf("%s opened as %s at %s, want %s at %s", c.stream, transport, address, c.transport, want)
		}
		if got := socketType(t, address); got != c.network {
			t.Errorf("%s is a %s socket, want %s", c.stream, got, c.network)
		}
	}
	if _, _, err := b.svc.Open("acquire", 1, "station.spectra", streams.Tolerances{}); err == nil {
		t.Error("an open stream opened again")
	}
	if got := b.svc.Active("acquire"); !slices.Equal(got, []string{"station.events", "station.preview", "station.spectra"}) {
		t.Errorf("the unit's open streams are %v", got)
	}
}

func envelope(t *testing.T, e *pluginv1.Envelope) []byte {
	t.Helper()
	raw, err := proto.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// std: yoke:a-streams-transport.02
func TestTheOrderedPathCarriesOneDataEnvelopePerPacket(t *testing.T) {
	b := newBench(t)
	_, address, err := b.svc.Open("acquire", 1, "station.spectra", streams.Tolerances{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("unixpacket", address)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i := uint64(1); i <= 3; i++ {
		conn.Write(envelope(t, &pluginv1.Envelope{MessageId: "u-1", SentAtUnixNano: int64(1000 * i),
			Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: i, Payload: []byte{byte(i), 0xff}}}}))
	}
	got := b.read(t, 3)
	if len(got) != 3 {
		t.Fatalf("%d frames were read: %+v", len(got), got)
	}
	for i, f := range got {
		n := uint64(i + 1)
		if f.Unit != "acquire" || f.Stream != "station.spectra" || f.Sequence != n || f.SentAt != 1000*n || !bytes.Equal(f.Payload, []byte{byte(n), 0xff}) {
			t.Errorf("frame %d is %+v", i, f)
		}
	}
	second, err := net.Dial("unixpacket", address)
	if err == nil {
		second.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := second.Read(make([]byte, 1)); err == nil {
			t.Error("a second connection was kept open")
		}
		second.Close()
	}
	conn.Write(envelope(t, &pluginv1.Envelope{MessageId: "u-2", Payload: &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: 10}}}))
	conn.Write(envelope(t, &pluginv1.Envelope{MessageId: "u-3", Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: 4}}}))
	if got := b.read(t, 4); len(got) != 4 || got[3].Sequence != 4 {
		t.Errorf("after the second connection and a packet that is not data, the first carried %+v", got)
	}
	if log := b.log.String(); !strings.Contains(log, "level=WARN") || !strings.Contains(log, "stream=station.spectra") {
		t.Errorf("the packet that is not data is not recorded:\n%s", log)
	}
}

func frame(sequence, clock uint64, payload []byte) []byte {
	out := make([]byte, 16, 16+len(payload))
	binary.LittleEndian.PutUint64(out[0:8], sequence)
	binary.LittleEndian.PutUint64(out[8:16], clock)
	return append(out, payload...)
}

// std: yoke:a-streams-transport.03
func TestTheFramedPathReadsTheHeaderAndAGapIsAFaultOnlyWhereLossIsNotTolerated(t *testing.T) {
	b := newBench(t)
	for _, c := range []struct {
		stream    string
		tolerates streams.Tolerances
	}{{"station.preview", streams.Tolerances{Loss: true}}, {"station.events", streams.Tolerances{Reorder: true}}} {
		_, address, err := b.svc.Open("acquire", 1, c.stream, c.tolerates)
		if err != nil {
			t.Fatal(err)
		}
		conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: address, Net: "unixgram"})
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range []uint64{1, 2, 4} {
			conn.Write(frame(n, 7000+n, []byte("frame")))
		}
		conn.Write([]byte{1, 2, 3})
		conn.Close()
	}
	got := b.read(t, 6)
	if len(got) != 6 {
		t.Fatalf("%d frames were read: %+v", len(got), got)
	}
	for _, f := range got {
		if f.SentAt != 7000+f.Sequence || string(f.Payload) != "frame" {
			t.Errorf("a frame read as %+v", f)
		}
	}
	log := b.log.String()
	var faults, short []string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, "gap") {
			faults = append(faults, line)
		}
		if strings.Contains(line, "shorter") {
			short = append(short, line)
		}
	}
	if len(faults) != 1 || !strings.Contains(faults[0], "stream=station.events") {
		t.Errorf("the gaps recorded are %q, want one on the stream that does not tolerate loss", faults)
	}
	if len(short) != 2 {
		t.Errorf("the short datagrams recorded are %q", short)
	}
}

// std: yoke:a-streams-transport.04
func TestEachRouteThatEndsAStreamRemovesItsTransport(t *testing.T) {
	b := newBench(t)
	open := func(unitID, stream string, incarnation uint64) string {
		_, address, err := b.svc.Open(unitID, incarnation, stream, streams.Tolerances{})
		if err != nil {
			t.Fatal(err)
		}
		return address
	}
	spectra, diagnostics := open("acquire", "station.spectra", 2), open("acquire", "station.diagnostics", 2)
	archive := open("archive", "archive.index", 5)
	if !b.svc.Close("acquire", "station.spectra", streams.Asked) {
		t.Error("closing an open stream says it was not open")
	}
	b.svc.CloseAll("acquire", streams.SessionEnded)
	b.svc.CloseAll("archive", streams.UnitExited)
	if b.svc.Close("acquire", "station.spectra", streams.Asked) {
		t.Error("closing a stream that is not open says it was")
	}
	for _, address := range []string{spectra, diagnostics, archive} {
		if _, err := os.Stat(address); err == nil {
			t.Errorf("%s is still there", address)
		}
		if c, err := net.Dial("unixpacket", address); err == nil {
			c.Close()
			t.Errorf("%s accepted a connection", address)
		}
	}
	type stopped struct {
		Unit, Stream, Reason string
		Severity             int
		Incarnation          uint64
	}
	var got []stopped
	for _, e := range b.events() {
		var d struct{ Stream, Reason string }
		json.Unmarshal(e.Detail, &d)
		if e.Type != "unit.stream.stopped" {
			t.Errorf("published %s", e.Type)
			continue
		}
		got = append(got, stopped{e.Subject.ID, d.Stream, d.Reason, e.Severity, e.Subject.Incarnation})
	}
	want := []stopped{
		{"acquire", "station.spectra", "asked", 10, 2},
		{"acquire", "station.diagnostics", "session ended", 30, 2},
		{"archive", "archive.index", "unit exited", 30, 5},
	}
	if !slices.Equal(got, want) {
		t.Errorf("published %+v, want %+v", got, want)
	}
	if active := b.svc.Active("acquire"); len(active) != 0 {
		t.Errorf("the unit still has %v open", active)
	}
}
