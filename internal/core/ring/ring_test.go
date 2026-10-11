package ring

import (
	"bytes"
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// pair is a ring with its producer's writer and the Core's reader.
func pair(t *testing.T, slots, capacity uint32) (*Ring, *Writer, *Reader) {
	t.Helper()
	r, err := CreateWith("yoke-test", slots, capacity)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	pf, err := r.ProducerFile()
	if err != nil {
		t.Fatal(err)
	}
	w, err := OpenWriter(pf)
	pf.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Close)
	rf, err := r.ReaderFile()
	if err != nil {
		t.Fatal(err)
	}
	rd, err := OpenReader(rf)
	rf.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rd.Close)
	return r, w, rd
}

// std: yoke:the-shared-ring.01
func TestTheRingIsCreatedSizedAndLabelled(t *testing.T) {
	r, err := Create("yoke-bench-acquire-station.spectra")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	f, err := r.ReaderFile()
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, _ := f.Stat()
	if want := int64(64 + 32*(32+1<<20)); info.Size() != want {
		t.Errorf("the ring is %d bytes, want %d", info.Size(), want)
	}
	if link, _ := os.Readlink("/proc/self/fd/" + strconv.Itoa(int(f.Fd()))); !strings.Contains(link, "yoke-bench-acquire-station.spectra") {
		t.Errorf("the memory file is %q", link)
	}
	head := make([]byte, 64)
	f.ReadAt(head, 0)
	le := binary.LittleEndian
	if string(head[:8]) != "YOKERING" || le.Uint32(head[8:]) != 1 || le.Uint32(head[12:]) != 32 || le.Uint32(head[16:]) != 1<<20 {
		t.Errorf("the header reads %q %d %d %d", head[:8], le.Uint32(head[8:]), le.Uint32(head[12:]), le.Uint32(head[16:]))
	}
	if le.Uint64(head[24:]) != 0 || le.Uint32(head[32:]) != 0 || !bytes.Equal(head[20:24], make([]byte, 4)) || !bytes.Equal(head[36:], make([]byte, 28)) {
		t.Errorf("the counters or the reserved bytes are not zero: %x", head)
	}
}

// std: yoke:the-shared-ring.02
func TestAFramePublishedIsReadInOrder(t *testing.T) {
	_, w, rd := pair(t, 4, 64)
	payloads := [][]byte{[]byte("a"), []byte("bbbb"), bytes.Repeat([]byte("c"), 64)}
	for i, p := range payloads {
		if err := w.Publish(uint64(100+i), p); err != nil {
			t.Fatal(err)
		}
	}
	for i, p := range payloads {
		f, gap, err := rd.Next(time.Second)
		if err != nil || gap != 0 {
			t.Fatalf("frame %d: %v, gap %d", i+1, err, gap)
		}
		if f.Sequence != uint64(i+1) || f.Clock != uint64(100+i) || !bytes.Equal(f.Payload, p) {
			t.Errorf("frame %d read as %d %d %q", i+1, f.Sequence, f.Clock, f.Payload)
		}
	}
}

// std: yoke:the-shared-ring.03
func TestAReaderTooFarBehindMovesToTheOldestFrameHeld(t *testing.T) {
	_, w, rd := pair(t, 32, 16)
	for i := 1; i <= 40; i++ {
		w.Publish(uint64(i), []byte{byte(i)})
	}
	f, gap, err := rd.Next(time.Second)
	if err != nil || f.Sequence != 9 || gap != 8 {
		t.Fatalf("first read: frame %d, gap %d, %v", f.Sequence, gap, err)
	}
	for want := uint64(10); want <= 40; want++ {
		f, gap, err := rd.Next(time.Second)
		if err != nil || f.Sequence != want || gap != 0 {
			t.Fatalf("read frame %d, gap %d, %v; want %d", f.Sequence, gap, err, want)
		}
	}
}

// std: yoke:the-shared-ring.04
func TestASlotOverwrittenWhileReadIsAGap(t *testing.T) {
	_, w, rd := pair(t, 32, 16)
	w.Publish(1, []byte("first"))
	rd.copied = func() {
		for i := 2; i <= 33; i++ {
			w.Publish(uint64(i), []byte("lap"))
		}
		rd.copied = nil
	}
	f, gap, err := rd.Next(time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if f.Sequence == 1 {
		t.Fatalf("an overwritten frame was delivered: %q", f.Payload)
	}
	if gap == 0 {
		t.Errorf("no gap was reported; read frame %d", f.Sequence)
	}
}

// std: yoke:the-shared-ring.05
func TestAFrameLargerThanASlotIsRefused(t *testing.T) {
	r, w, _ := pair(t, 4, 1<<20)
	err := w.Publish(1, make([]byte, 1<<20+1))
	if err == nil || !strings.Contains(err.Error(), "1048576") {
		t.Errorf("a frame of 1 MiB and a byte: %v", err)
	}
	if r.Published() != 0 {
		t.Errorf("%d frames published", r.Published())
	}
}

// std: yoke:the-shared-ring.06
func TestAWaitingReaderIsWokenByAPublication(t *testing.T) {
	_, w, rd := pair(t, 4, 16)
	interval := 5 * time.Second
	go func() {
		time.Sleep(50 * time.Millisecond)
		w.Publish(1, []byte("now"))
	}()
	start := time.Now()
	f, _, err := rd.Next(interval)
	if err != nil || f.Sequence != 1 {
		t.Fatalf("read frame %d: %v", f.Sequence, err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("the reader took %v to wake", took)
	}
}

// std: yoke:the-shared-ring.07
func TestTheProducersDescriptorWritesAndAReadersCannot(t *testing.T) {
	r, err := CreateWith("yoke-test", 4, 16)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	pf, _ := r.ProducerFile()
	defer pf.Close()
	rf, _ := r.ReaderFile()
	defer rf.Close()
	size := Size(4, 16)
	if m, err := unix.Mmap(int(pf.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED); err != nil {
		t.Errorf("the producer's descriptor does not map for writing: %v", err)
	} else {
		unix.Munmap(m)
	}
	if m, err := unix.Mmap(int(rf.Fd()), 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED); err == nil {
		unix.Munmap(m)
		t.Error("a reader's descriptor maps for writing")
	}
}
