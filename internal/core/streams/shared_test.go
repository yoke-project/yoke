package streams_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/yoke-project/yoke/internal/core/ring"
	"github.com/yoke-project/yoke/internal/core/streams"
)

// produce connects to a stream's ring socket as its producer and returns the connection and the
// descriptor it was handed, or none.
func produce(t *testing.T, address string) (*net.UnixConn, *os.File) {
	t.Helper()
	c, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: address, Net: "unixpacket"})
	if err != nil {
		t.Fatalf("the producer did not connect to %s: %v", address, err)
	}
	t.Cleanup(func() { c.Close() })
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf, oob := make([]byte, 16), make([]byte, unix.CmsgSpace(4))
	n, oobn, _, _, err := c.ReadMsgUnix(buf, oob)
	if err != nil || n != 1 {
		return c, nil
	}
	messages, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(messages) != 1 {
		return c, nil
	}
	fds, err := unix.ParseUnixRights(&messages[0])
	if err != nil || len(fds) != 1 {
		return c, nil
	}
	f := os.NewFile(uintptr(fds[0]), "ring")
	t.Cleanup(func() { f.Close() })
	return c, f
}

// std: yoke:a-streams-transport.09
func TestAStreamThatToleratesLossIsCarriedOnTheRingWhereOffered(t *testing.T) {
	b := newBench(t)
	for _, c := range []struct {
		stream    string
		tolerates streams.Tolerances
		want      streams.Transport
	}{
		{"station.preview", streams.Tolerances{Loss: true}, streams.Shared},
		{"station.events", streams.Tolerances{Reorder: true}, streams.Framed},
		{"station.spectra", streams.Tolerances{}, streams.Ordered},
	} {
		transport, _, err := b.svc.Open("acquire", 1, c.stream, c.tolerates, streams.Offers{Shared: true})
		if err != nil || transport != c.want {
			t.Errorf("%s opened as %s (%v), want %s", c.stream, transport, err, c.want)
		}
	}
	if transport, _, _ := b.svc.Open("archive", 1, "archive.preview", streams.Tolerances{Loss: true}, streams.Offers{}); transport != streams.Framed {
		t.Errorf("for a library that offers nothing, a stream that tolerates loss opened as %s", transport)
	}
	_, f := produce(t, b.svc.Address("acquire", "station.preview"))
	if f == nil {
		t.Fatal("the producer was handed no descriptor")
	}
	if link, _ := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(f.Fd()))); !strings.Contains(link, "yoke-bench-acquire-station.preview") {
		t.Errorf("the ring is labelled %q", link)
	}
	w, err := ring.OpenWriter(f)
	if err != nil {
		t.Fatalf("the descriptor does not map for writing: %v", err)
	}
	defer w.Close()
	for i, p := range []string{"one", "two", "three"} {
		if err := w.Publish(uint64(100+i), []byte(p)); err != nil {
			t.Fatal(err)
		}
	}
	got := b.read(t, 3)
	if len(got) != 3 {
		t.Fatalf("%d frames delivered, want 3", len(got))
	}
	for i, p := range []string{"one", "two", "three"} {
		if f := got[i]; f.Unit != "acquire" || f.Stream != "station.preview" || f.Sequence != uint64(i+1) || f.SentAt != uint64(100+i) || !bytes.Equal(f.Payload, []byte(p)) {
			t.Errorf("frame %d delivered as %+v", i+1, f)
		}
	}
}

// std: yoke:a-streams-transport.10
func TestTheRingsDescriptorGoesToOneProducerAndItsSocketEndsTheStream(t *testing.T) {
	b := newBench(t)
	_, address, err := b.svc.Open("acquire", 3, "station.preview", streams.Tolerances{Loss: true}, streams.Offers{Shared: true})
	if err != nil {
		t.Fatal(err)
	}
	first, f := produce(t, address)
	if f == nil {
		t.Fatal("the first producer was handed no descriptor")
	}
	if _, second := produce(t, address); second != nil {
		t.Error("a second connection was handed a descriptor")
	}
	b.svc.Close("acquire", "station.preview", streams.Asked)
	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	if n, err := first.Read(make([]byte, 1)); err == nil {
		t.Errorf("the producer's connection is still open: read %d bytes", n)
	}
	if _, err := os.Stat(address); err == nil {
		t.Error("the ring's socket is still there")
	}
	events := b.events()
	if len(events) != 1 || events[0].Type != "unit.stream.stopped" {
		t.Fatalf("published %v", events)
	}
	var d struct{ Stream, Reason string }
	json.Unmarshal(events[0].Detail, &d)
	if d.Stream != "station.preview" || d.Reason != "asked" {
		t.Errorf("the stop says %+v", d)
	}
}

// std: yoke:a-streams-transport.15
func TestTheCoreReadsARingsFrameForADeliveryItRelaysAlone(t *testing.T) {
	root, _ := os.MkdirTemp("", "yt")
	t.Cleanup(func() { os.RemoveAll(root) })
	log := &syncBuffer{}
	// No Deliver: nothing but a feed takes what the Core reads.
	svc := streams.New(streams.Config{Root: root, Instance: "bench", Log: slog.New(slog.NewTextHandler(log, nil))})
	t.Cleanup(func() { svc.CloseAll("acquire", streams.UnitExited) })
	direct := svc.FeedRing("acquire", "station.preview", func(f *os.File) { f.Close() }, func() {}, func() {})
	defer direct()

	activation := func(incarnation uint64) {
		t.Helper()
		_, address, err := svc.Open("acquire", incarnation, "station.preview", streams.Tolerances{Loss: true}, streams.Offers{Shared: true})
		if err != nil {
			t.Fatal(err)
		}
		_, f := produce(t, address)
		if f == nil {
			t.Fatal("the producer was handed no descriptor")
		}
		w, err := ring.OpenWriter(f)
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		for i := range 3 {
			if err := w.Publish(uint64(100+i), []byte{byte(i)}); err != nil {
				t.Fatal(err)
			}
			// Apart, so that a reader that reads meets each frame.
			time.Sleep(20 * time.Millisecond)
		}
	}

	activation(1)
	time.Sleep(300 * time.Millisecond)
	svc.Close("acquire", "station.preview", streams.Asked)
	if said := log.String(); !strings.Contains(said, "event=stopped") || !strings.Contains(said, "read=3 relayed=0") {
		t.Fatalf("nothing was relayed, and the first stop says: %s", said)
	}

	var mu sync.Mutex
	var relayed []streams.Frame
	relay := svc.Feed("acquire", "station.preview", func(f streams.Frame) {
		mu.Lock()
		defer mu.Unlock()
		relayed = append(relayed, f)
	}, func() {})
	defer relay()
	activation(2)
	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(relayed)
		mu.Unlock()
		if n >= 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	svc.Close("acquire", "station.preview", streams.Asked)
	mu.Lock()
	defer mu.Unlock()
	if len(relayed) != 3 || relayed[0].Sequence != 1 || relayed[2].Sequence != 3 || !bytes.Equal(relayed[2].Payload, []byte{2}) {
		t.Errorf("the relayed consumer was given %+v", relayed)
	}
	if said := log.String(); !strings.Contains(said, "read=3 relayed=3") {
		t.Errorf("three frames were relayed, and the second stop says: %s", said)
	}
}
