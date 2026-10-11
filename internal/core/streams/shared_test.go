package streams_test

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"strings"
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
		transport, _, err := b.svc.Open("acquire", 1, c.stream, c.tolerates, true)
		if err != nil || transport != c.want {
			t.Errorf("%s opened as %s (%v), want %s", c.stream, transport, err, c.want)
		}
	}
	if transport, _, _ := b.svc.Open("archive", 1, "archive.preview", streams.Tolerances{Loss: true}, false); transport != streams.Framed {
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
	_, address, err := b.svc.Open("acquire", 3, "station.preview", streams.Tolerances{Loss: true}, true)
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
