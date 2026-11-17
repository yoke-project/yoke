// Package ring is the shared ring: the one object that is not a socket, carrying a stream that tolerates
// loss from its producer to the Core in one write and one read.
//
// The Core creates an anonymous memory file, sizes it, labels it and writes its header; the producer is
// handed a descriptor that writes, the Core keeps one that only reads. A frame is published into the slot
// its sequence selects and every waiter is woken on one word. A reader checks a slot's sequence before
// and after it copies the slot, and a slot overwritten in between, or a reader the producer has lapped,
// is a gap — as a lost datagram is on the framed transport.
package ring

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The figures of layout 1.
const (
	Slots      = 32
	Capacity   = 1 << 20
	headerSize = 64
	slotHeader = 32
	magic      = "YOKERING"
	version    = 1
)

// Header offsets.
const (
	offVersion   = 8
	offSlots     = 12
	offCapacity  = 16
	offPublished = 24
	offWake      = 32
)

// ErrNoFrame is a wait that ended with nothing published.
var ErrNoFrame = errors.New("no frame was published in the interval")

// ErrStopped is a wait ended by its caller, with nothing published.
var ErrStopped = errors.New("the wait was stopped")

// Size is the bytes a ring of the given geometry occupies.
func Size(slots, capacity uint32) int {
	return headerSize + int(slots)*(slotHeader+int(capacity))
}

// Ring is a ring the Core created: it holds the memory file and a mapping of it.
type Ring struct {
	fd  int
	mem []byte
}

// Create creates a ring of the figures layout 1 sets, labelled as given.
func Create(label string) (*Ring, error) { return CreateWith(label, Slots, Capacity) }

// The memory a ring may take whole, beyond which it holds fewer slots, and the fewest it holds.
const (
	Budget   = 128 << 20
	MinSlots = 4
)

// Geometry is the ring a stream declaring maxFrame as its largest frame is carried on: slots of that
// capacity, Slots of them while they fit in Budget, otherwise as many as fit, never fewer than MinSlots.
func Geometry(maxFrame int64) (slots, capacity uint32) {
	slots = Slots
	if fit := Budget / maxFrame; fit < int64(slots) {
		slots = uint32(max(fit, MinSlots))
	}
	return slots, uint32(maxFrame)
}

// CreateWith creates a ring of the given geometry: an anonymous memory file, sized, labelled, its header
// written.
func CreateWith(label string, slots, capacity uint32) (*Ring, error) {
	fd, err := unix.MemfdCreate(label, unix.MFD_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("the ring %s cannot be created: %w", label, err)
	}
	size := Size(slots, capacity)
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		unix.Close(fd)
		return nil, err
	}
	mem, err := unix.Mmap(fd, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	copy(mem[:8], magic)
	le := binary.LittleEndian
	le.PutUint32(mem[offVersion:], version)
	le.PutUint32(mem[offSlots:], slots)
	le.PutUint32(mem[offCapacity:], capacity)
	return &Ring{fd: fd, mem: mem}, nil
}

// ProducerFile is a descriptor of the ring that writes, for the producer alone.
func (r *Ring) ProducerFile() (*os.File, error) {
	fd, err := unix.Dup(r.fd)
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	return os.NewFile(uintptr(fd), "ring"), nil
}

// ReaderFile is a descriptor of the ring that only reads.
func (r *Ring) ReaderFile() (*os.File, error) {
	return os.OpenFile("/proc/self/fd/"+strconv.Itoa(r.fd), os.O_RDONLY, 0)
}

// Published is the count of frames published.
func (r *Ring) Published() uint64 { return atomic.LoadUint64(u64(r.mem, offPublished)) }

// Wake changes the wake word and wakes every waiter on it, with nothing published: a reader waiting with no
// bound looks again, and finds what stopped it.
func (r *Ring) Wake() {
	atomic.AddUint32(u32(r.mem, offWake), 1)
	futexWake(u32(r.mem, offWake))
}

// Close lets the Core's hold on the ring go. The memory goes when the last descriptor on it closes.
func (r *Ring) Close() {
	if r.mem != nil {
		unix.Munmap(r.mem)
		r.mem = nil
	}
	unix.Close(r.fd)
}

func u64(mem []byte, off int) *uint64 { return (*uint64)(unsafe.Pointer(&mem[off])) }
func u32(mem []byte, off int) *uint32 { return (*uint32)(unsafe.Pointer(&mem[off])) }

// geometry is a mapped ring's, read from its header.
type geometry struct {
	mem      []byte
	slots    uint32
	capacity uint32
}

func mapRing(f *os.File, prot int) (geometry, error) {
	info, err := f.Stat()
	if err != nil {
		return geometry{}, err
	}
	if info.Size() < headerSize {
		return geometry{}, fmt.Errorf("the ring is %d bytes, shorter than its header", info.Size())
	}
	mem, err := unix.Mmap(int(f.Fd()), 0, int(info.Size()), prot, unix.MAP_SHARED)
	if err != nil {
		return geometry{}, err
	}
	le := binary.LittleEndian
	g := geometry{mem: mem, slots: le.Uint32(mem[offSlots:]), capacity: le.Uint32(mem[offCapacity:])}
	switch {
	case string(mem[:8]) != magic:
		err = errors.New("the memory is not a ring")
	case le.Uint32(mem[offVersion:]) != version:
		err = fmt.Errorf("the ring is of layout %d; this reads layout %d", le.Uint32(mem[offVersion:]), version)
	case g.slots == 0 || Size(g.slots, g.capacity) > len(mem):
		err = fmt.Errorf("the ring's geometry, %d slots of %d bytes, does not fit its %d bytes", g.slots, g.capacity, len(mem))
	}
	if err != nil {
		unix.Munmap(mem)
		return geometry{}, err
	}
	return g, nil
}

func (g geometry) slot(n uint64) int {
	return headerSize + int((n-1)%uint64(g.slots))*(slotHeader+int(g.capacity))
}

// Writer is the producer's side of a ring.
type Writer struct {
	geometry
	published uint64
}

// OpenWriter maps a ring the producer was handed, for writing.
func OpenWriter(f *os.File) (*Writer, error) {
	g, err := mapRing(f, unix.PROT_READ|unix.PROT_WRITE)
	if err != nil {
		return nil, err
	}
	return &Writer{geometry: g, published: atomic.LoadUint64(u64(g.mem, offPublished))}, nil
}

// Publish writes one frame into its slot and wakes every waiter. A frame larger than a slot is refused.
func (w *Writer) Publish(clock uint64, payload []byte) error {
	if len(payload) > int(w.capacity) {
		return fmt.Errorf("the frame is %d bytes, and a slot of this ring holds %d", len(payload), w.capacity)
	}
	n := w.published + 1
	at := w.slot(n)
	sequence := u64(w.mem, at)
	atomic.StoreUint64(sequence, 0)
	le := binary.LittleEndian
	le.PutUint64(w.mem[at+8:], clock)
	le.PutUint32(w.mem[at+16:], uint32(len(payload)))
	copy(w.mem[at+slotHeader:], payload)
	atomic.StoreUint64(sequence, n)
	atomic.StoreUint64(u64(w.mem, offPublished), n)
	atomic.AddUint32(u32(w.mem, offWake), 1)
	futexWake(u32(w.mem, offWake))
	w.published = n
	return nil
}

// Close lets the producer's mapping go.
func (w *Writer) Close() { unix.Munmap(w.mem) }

// Frame is one frame read from a ring.
type Frame struct {
	Sequence uint64
	Clock    uint64
	Payload  []byte
}

// Reader is one reader of a ring, at its own pace.
type Reader struct {
	geometry
	next uint64
	// copied, where set, runs between a slot's copy and its second sequence check.
	copied func()
}

// OpenReader maps a ring for reading only.
func OpenReader(f *os.File) (*Reader, error) {
	g, err := mapRing(f, unix.PROT_READ)
	if err != nil {
		return nil, err
	}
	return &Reader{geometry: g, next: 1}, nil
}

// Next returns the next frame the reader can still read, and how many frames before it were lost: a lap
// by the producer, or a slot overwritten while it was copied. It waits at most the interval given for a
// frame to be published.
func (r *Reader) Next(interval time.Duration) (Frame, uint64, error) {
	return r.read(time.Now().Add(interval), nil)
}

// NextUntil is Next with no bound: it waits until a frame is published or stop is closed, and the closer
// of stop then wakes the ring (Ring.Wake), so that a reader waiting at rest costs nothing.
func (r *Reader) NextUntil(stop <-chan struct{}) (Frame, uint64, error) {
	return r.read(time.Time{}, stop)
}

// waits waits on the wake word while it holds what was read before: until the deadline, where there is
// one, or until stop is closed. It says why it ended where that is no frame.
func (r *Reader) waits(wake uint32, deadline time.Time, stop <-chan struct{}) error {
	select {
	case <-stop:
		return ErrStopped
	default:
	}
	if deadline.IsZero() {
		futexWait(u32(r.mem, offWake), wake, 0)
		return nil
	}
	left := time.Until(deadline)
	if left <= 0 {
		return ErrNoFrame
	}
	futexWait(u32(r.mem, offWake), wake, left)
	return nil
}

func (r *Reader) read(deadline time.Time, stop <-chan struct{}) (Frame, uint64, error) {
	var gap uint64
	for {
		wake := atomic.LoadUint32(u32(r.mem, offWake))
		published := atomic.LoadUint64(u64(r.mem, offPublished))
		if published < r.next {
			if err := r.waits(wake, deadline, stop); err != nil {
				return Frame{}, gap, err
			}
			continue
		}
		if oldest := published - uint64(r.slots) + 1; published >= uint64(r.slots) && r.next < oldest {
			gap += oldest - r.next
			r.next = oldest
		}
		n := r.next
		r.next++
		at := r.slot(n)
		before := atomic.LoadUint64(u64(r.mem, at))
		le := binary.LittleEndian
		clock := le.Uint64(r.mem[at+8:])
		length := min(le.Uint32(r.mem[at+16:]), r.capacity)
		payload := append([]byte(nil), r.mem[at+slotHeader:at+slotHeader+int(length)]...)
		if r.copied != nil {
			r.copied()
		}
		after := atomic.LoadUint64(u64(r.mem, at))
		if before != n || after != n {
			gap++
			continue
		}
		return Frame{Sequence: n, Clock: clock, Payload: payload}, gap, nil
	}
}

// Pass moves the reader past every frame published, reading none of them, and returns how many it
// passed. It waits at most the interval given for a frame to be published.
func (r *Reader) Pass(interval time.Duration) (uint64, error) {
	return r.pass(time.Now().Add(interval), nil)
}

// PassUntil is Pass with no bound, as NextUntil is Next.
func (r *Reader) PassUntil(stop <-chan struct{}) (uint64, error) {
	return r.pass(time.Time{}, stop)
}

func (r *Reader) pass(deadline time.Time, stop <-chan struct{}) (uint64, error) {
	for {
		wake := atomic.LoadUint32(u32(r.mem, offWake))
		published := atomic.LoadUint64(u64(r.mem, offPublished))
		if published < r.next {
			if err := r.waits(wake, deadline, stop); err != nil {
				return 0, err
			}
			continue
		}
		passed := published - r.next + 1
		r.next = published + 1
		return passed, nil
	}
}

// Word is the ring's wake word as it stands: it changes at every frame published and every wake.
func (r *Reader) Word() uint32 { return atomic.LoadUint32(u32(r.mem, offWake)) }

// Close lets the reader's mapping go.
func (r *Reader) Close() { unix.Munmap(r.mem) }

// The futex operations, on a word of memory shared between processes: not private.
const (
	futexWaitOp = 0
	futexWakeOp = 1
)

// futexWait waits on a word of shared memory while it still holds val, for at most d; a d of zero is no
// bound.
func futexWait(addr *uint32, val uint32, d time.Duration) {
	var bound uintptr
	if d > 0 {
		ts := unix.NsecToTimespec(d.Nanoseconds())
		bound = uintptr(unsafe.Pointer(&ts))
	}
	unix.Syscall6(unix.SYS_FUTEX, uintptr(unsafe.Pointer(addr)), futexWaitOp, uintptr(val), bound, 0, 0)
}

// futexWake wakes every waiter on a word of shared memory.
func futexWake(addr *uint32) {
	unix.Syscall6(unix.SYS_FUTEX, uintptr(unsafe.Pointer(addr)), futexWakeOp, uintptr(math.MaxInt32), 0, 0, 0)
}
