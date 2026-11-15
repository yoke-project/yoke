package conformance

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.yaml.in/yaml/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
)

// Consumer is the channel a plugin run's Core binds for the suite: it starts, subscribes to and reads a
// stream on the interface contract itself, so a run measures the plugin library alone.
const Consumer = "suite"

// consumerChannels is what a plugin run composes: one local channel of one client, the suite's.
var consumerChannels = map[string]any{Consumer: map[string]any{"transport": "local", "clients": "single"}}

// StreamCases are the plugin contract's cases for streams, which run once the life of case 8 is granted
// everything, and before case 12 ends its Session.
func StreamCases() []Case {
	return []Case{
		{Contract: "plugin", ID: "yoke:plugin.13", Title: "a stream is activated on the transport its tolerances select",
			Cites:        []string{"specs/50.86", "specs/90.33", "arch/50-plugin-surface/07 §A stream flows because it was told to", "arch/50-plugin-surface/07 §What the two tolerances select", "arch/50-plugin-surface/07 §The shared ring"},
			Precondition: "the harness of case 8, granted everything; its Manifest declaring a stream that tolerates nothing and one that tolerates loss; the suite attached to the channel `suite` the run composes",
			Issues:       "for each of the two streams, `stream.subscribe` and then `stream.start`, on the interface surface",
			Requires:     "for each, a delivery on a per-subscriber socket and an acknowledgement; and an observation `activated` naming the stream, with the transport `ordered` — or `ordered_stream` where the library offered the stream socket — for the one that tolerates nothing, and for the one that tolerates loss `framed`, `framed_stream`, or `shared_object` where the library offered the shared ring",
			Run:          activatedOnItsTransport},
		{Contract: "plugin", ID: "yoke:plugin.14", Title: "what is emitted on the ordered transport arrives in order, numbered from 1, unchanged",
			Cites:        []string{"specs/50.86", "specs/50.90", "arch/50-plugin-surface/07 §What every transport keeps", "arch/90-sdks/03 §A real Core, and no fixture"},
			Precondition: "the stream that tolerates nothing, activated in case 13, and the suite reading its delivery",
			Issues:       "`emit` on it three times, carrying `one`, `two` and `three`",
			Requires:     "each answered with no refusal; the delivery carrying three frames, of the sequences 1, 2 and 3, with those payloads, in that order",
			Run:          orderedInOrder},
		{Contract: "plugin", ID: "yoke:plugin.15", Title: "what is emitted on the framed transport is numbered from 1 with no gap, so a gap would be seen",
			Cites:        []string{"specs/50.90", "arch/50-plugin-surface/07 §The frame", "arch/90-sdks/03 §What is outside the suite, and why"},
			Precondition: "the stream that tolerates loss, activated in case 13, and the suite reading its delivery",
			Issues:       "`emit` on it three times, carrying `one`, `two` and `three`",
			Requires:     "each answered with no refusal; the delivery carrying three frames, of the sequences 1, 2 and 3, with those payloads",
			Run:          framedContiguous},
		{Contract: "plugin", ID: "yoke:plugin.16", Title: "a stop is surfaced, and closes the emission",
			Cites:        []string{"specs/90.33", "arch/50-plugin-surface/07 §Three routes end a stream, and the transport goes in all three", "arch/90-sdks/06 §It may not create a stream's transport"},
			Precondition: "the stream that tolerates nothing, flowing",
			Issues:       "`stream.stop` of it, on the interface surface; then `emit` on it",
			Requires:     "an acknowledgement; an observation `stopped` naming the stream; then a refusal `stream.inactive`",
			Run:          stoppedAndClosed},
		{Contract: "plugin", ID: "yoke:plugin.19", Title: "the library offers the stream socket, and its streams are activated on it",
			Cites:        []string{"specs/50.86", "arch/50-plugin-surface/07 §What the two tolerances select", "arch/50-plugin-surface/08 §Where the data goes"},
			Precondition: "the two streams case 13 activated",
			Issues:       "nothing: the transports case 13 observed are read",
			Requires:     "the stream that tolerates nothing activated on `ordered_stream`, and the one that tolerates loss on `framed_stream` or `shared_object`: a library that offers neither stream socket would leave a frame bounded by its host's socket buffer",
			Run:          activatedOnAStreamSocket},
		{Contract: "plugin", ID: "yoke:plugin.20", Title: "a frame larger than 1 MiB, on a stream that declares it, arrives whole",
			Cites:        []string{"specs/50.86", "arch/50-plugin-surface/02 §The four declared lists", "arch/50-plugin-surface/07 §The shared ring"},
			Precondition: "the harness's Manifest declaring a stream that tolerates loss with a `max_frame` of at least 2 MiB; the suite subscribed to it, reading a stream socket",
			Issues:       "`stream.start` of it; then `emit-sized` on it of 1572864 bytes",
			Requires:     "an observation `activated` naming the stream; the emission answered with no refusal; the delivery carrying one frame of sequence 1 whose payload is 1572864 bytes, byte i being i mod 251",
			Run:          aLargeFrameArrivesWhole},
	}
}

// largeFrame is what case 20 asks to be emitted, and what it checks arrives.
const largeFrame = 1572864

// largeStream is a declared stream that tolerates loss and declares a largest frame of at least 2 MiB.
func (r *Run) largeStream() string {
	var m struct {
		Streams []struct {
			ID            string `yaml:"id"`
			ToleratesLoss bool   `yaml:"tolerates_loss"`
			MaxFrame      int64  `yaml:"max_frame"`
		} `yaml:"streams"`
	}
	yaml.Unmarshal([]byte(r.described), &m)
	for _, s := range m.Streams {
		if s.ToleratesLoss && s.MaxFrame >= 2<<20 {
			return s.ID
		}
	}
	return ""
}

// readStreamDelivery reads a per-subscriber stream socket: each message its length, then the 16-byte
// header and the payload.
func readStreamDelivery(path string) (*delivery, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	d := &delivery{conn: conn, frames: make(chan Frame, 64)}
	go func() {
		defer close(d.frames)
		for {
			var length [4]byte
			if _, err := io.ReadFull(conn, length[:]); err != nil {
				return
			}
			body := make([]byte, binary.LittleEndian.Uint32(length[:]))
			if _, err := io.ReadFull(conn, body); err != nil || len(body) < 16 {
				return
			}
			d.frames <- Frame{Sequence: binary.LittleEndian.Uint64(body[0:8]), SentAt: binary.LittleEndian.Uint64(body[8:16]), Payload: string(body[16:])}
		}
	}()
	return d, nil
}

// std: yoke:plugin.19
func activatedOnAStreamSocket(r *Run) Outcome {
	strict, lossy := r.streamsDeclared()
	got := fmt.Sprintf("%s on %q, %s on %q", strict, r.activated[strict], lossy, r.activated[lossy])
	if r.activated[strict] != "ordered_stream" || !slices.Contains([]string{"framed_stream", "shared_object"}, r.activated[lossy]) {
		return Fail("the activations of case 13", "`ordered_stream`, and `framed_stream` or `shared_object`", got)
	}
	return Pass()
}

// std: yoke:plugin.20
func aLargeFrameArrivesWhole(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("`emit-sized`", "the harness of case 8", err.Error())
	}
	large := r.largeStream()
	if large == "" {
		return Fail("`stream.start`", "a Manifest declaring a stream that tolerates loss with a `max_frame` of at least 2 MiB", r.described)
	}
	a, err := r.Attach()
	if err != nil {
		return Fail("the suite attaching to `suite`", "an attachment", err.Error())
	}
	subject := &interfacev1.UnitStream{Unit: h.Hello().Unit, Stream: large, Reads: []interfacev1.UnitStream_Reads{interfacev1.UnitStream_READS_STREAM_SOCKET}}
	resp, code, err := a.Call(&interfacev1.Request{Operation: &interfacev1.Request_StreamSubscribe{StreamSubscribe: subject}})
	socket := resp.GetStreamSubscribe().GetStreamSocket()
	if err != nil || code != "" || socket == "" {
		return Fail("`stream.subscribe` of "+large+" reading a stream socket", "a delivery on a stream socket", fmt.Sprintf("%v %q %v", resp, code, err))
	}
	d, err := readStreamDelivery(socket)
	if err != nil {
		return Fail("`stream.subscribe` of "+large, "a socket the suite reaches", err.Error())
	}
	r.held = append(r.held, func() { d.conn.Close() })
	if resp, code, err := a.Call(&interfacev1.Request{Operation: &interfacev1.Request_StreamStart{StreamStart: subject}}); err != nil || code != "" {
		return Fail("`stream.start` of "+large, "an acknowledgement", fmt.Sprintf("%v %q %v", resp, code, err))
	}
	if o, before, ok := observed(h, "activated", 10*time.Second); !ok || o.Fields["stream"] != large {
		return Fail("`stream.start` of "+large, "an observation `activated` naming it", fmt.Sprintf("%v %v", o, before))
	}
	res := h.Do("emit-sized", map[string]any{"stream": large, "bytes": largeFrame})
	if res.Unrecognised {
		return Absent("emit-sized")
	}
	if res.Refusal != "" || res.Lost {
		return Fail("`emit-sized` of "+strconv.Itoa(largeFrame)+" bytes on "+large, "an answer with no refusal", fmt.Sprintf("%q lost=%v", res.Refusal, res.Lost))
	}
	f, ok := d.next(20 * time.Second)
	if !ok || f.Sequence != 1 || len(f.Payload) != largeFrame {
		return Fail("`emit-sized` on "+large, "one frame of sequence 1 and "+strconv.Itoa(largeFrame)+" bytes", fmt.Sprintf("sequence %d, %d bytes, arrived=%v", f.Sequence, len(f.Payload), ok))
	}
	for i := range len(f.Payload) {
		if f.Payload[i] != byte(i%251) {
			return Fail("`emit-sized` on "+large, "byte i being i mod 251", fmt.Sprintf("byte %d is %d", i, f.Payload[i]))
		}
	}
	return Pass()
}

// streamsDeclared are the declared streams that tolerate nothing and that tolerate loss.
func (r *Run) streamsDeclared() (strict, lossy string) {
	var m struct {
		Streams []struct {
			ID               string `yaml:"id"`
			ToleratesLoss    bool   `yaml:"tolerates_loss"`
			ToleratesReorder bool   `yaml:"tolerates_reorder"`
		} `yaml:"streams"`
	}
	yaml.Unmarshal([]byte(r.described), &m)
	for _, s := range m.Streams {
		switch {
		case !s.ToleratesLoss && !s.ToleratesReorder && strict == "":
			strict = s.ID
		case s.ToleratesLoss && lossy == "":
			lossy = s.ID
		}
	}
	return strict, lossy
}

// Frame is one frame a delivery carried.
type Frame struct {
	Sequence uint64
	SentAt   uint64
	Payload  string
}

// delivery reads a per-subscriber socket: each packet the 16-byte header and the payload.
type delivery struct {
	conn   net.Conn
	frames chan Frame
}

func readDelivery(path string) (*delivery, error) {
	conn, err := net.Dial("unixpacket", path)
	if err != nil {
		return nil, err
	}
	d := &delivery{conn: conn, frames: make(chan Frame, 64)}
	go func() {
		defer close(d.frames)
		buf := make([]byte, 1<<16)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			if n < 16 {
				continue
			}
			d.frames <- Frame{Sequence: binary.LittleEndian.Uint64(buf[0:8]), SentAt: binary.LittleEndian.Uint64(buf[8:16]), Payload: string(buf[16:n])}
		}
	}()
	return d, nil
}

// next is the delivery's next frame, within the bound.
func (d *delivery) next(within time.Duration) (Frame, bool) {
	select {
	case f, open := <-d.frames:
		return f, open
	case <-time.After(within):
		return Frame{}, false
	}
}

// Attachment is the suite's own attachment to a channel, on the interface contract.
type Attachment struct {
	stream  interfacev1.Interface_AttachClient
	mu      sync.Mutex
	calls   int
	waiting map[string]chan *interfacev1.CoreFrame
	end     func()
}

// Attach attaches the suite to the channel the run composes for it, once, for as long as the run lasts.
func (r *Run) Attach() (*Attachment, error) {
	if r.consumer != nil {
		return r.consumer, nil
	}
	conn, err := grpc.NewClient("unix://"+filepath.Join(r.instance, "interfaces", Consumer+".sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	ctx, end := context.WithCancel(context.Background())
	r.held = append(r.held, func() { end(); conn.Close() })
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err != nil {
		return nil, err
	}
	first := make(chan error, 1)
	go func() {
		f, err := stream.Recv()
		if err == nil && f.GetOpening() == nil {
			err = fmt.Errorf("the attachment opened with %v", f)
		}
		first <- err
	}()
	select {
	case err := <-first:
		if err != nil {
			return nil, err
		}
	case <-time.After(10 * time.Second):
		return nil, errors.New("no opening within ten seconds")
	}
	a := &Attachment{stream: stream, waiting: map[string]chan *interfacev1.CoreFrame{}, end: end}
	go a.read()
	r.consumer = a
	return a, nil
}

// read hands each answer or refusal to the call it belongs to; events are not this attachment's subject.
func (a *Attachment) read() {
	for {
		f, err := a.stream.Recv()
		if err != nil {
			a.mu.Lock()
			for _, w := range a.waiting {
				close(w)
			}
			a.waiting = map[string]chan *interfacev1.CoreFrame{}
			a.mu.Unlock()
			return
		}
		if f.GetAnswer() == nil && f.GetRefusal() == nil {
			continue
		}
		a.mu.Lock()
		w := a.waiting[f.GetCall()]
		delete(a.waiting, f.GetCall())
		a.mu.Unlock()
		if w != nil {
			w <- f
		}
	}
}

// Call issues one request and waits for its answer, or its refusal's code.
func (a *Attachment) Call(req *interfacev1.Request) (*interfacev1.Response, string, error) {
	req.Version = 1
	a.mu.Lock()
	a.calls++
	call := "s-" + strconv.Itoa(a.calls)
	w := make(chan *interfacev1.CoreFrame, 1)
	a.waiting[call] = w
	err := a.stream.Send(&interfacev1.ClientFrame{Call: call, Carries: &interfacev1.ClientFrame_Request{Request: req}})
	a.mu.Unlock()
	if err != nil {
		return nil, "", err
	}
	select {
	case f, open := <-w:
		if !open {
			return nil, "", errors.New("the attachment ended")
		}
		if f.GetRefusal() != nil {
			return nil, f.GetRefusal().GetCode(), nil
		}
		return f.GetAnswer(), "", nil
	case <-time.After(20 * time.Second):
		return nil, "", errors.New("no answer within twenty seconds")
	}
}

// std: yoke:plugin.13
func activatedOnItsTransport(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("`stream.start`", "the harness of case 8", err.Error())
	}
	strict, lossy := r.streamsDeclared()
	if strict == "" || lossy == "" {
		return Fail("`stream.start`", "a Manifest declaring a stream that tolerates nothing and one that tolerates loss", r.described)
	}
	a, err := r.Attach()
	if err != nil {
		return Fail("the suite attaching to `suite`", "an attachment", err.Error())
	}
	unit := h.Hello().Unit
	// A stream that tolerates loss is activated on the shared ring for a library that offered it at
	// registration, and on the framed socket for any other.
	for stream, transports := range map[string][]string{strict: {"ordered", "ordered_stream"}, lossy: {"framed", "framed_stream", "shared_object"}} {
		subject := &interfacev1.UnitStream{Unit: unit, Stream: stream}
		resp, code, err := a.Call(&interfacev1.Request{Operation: &interfacev1.Request_StreamSubscribe{StreamSubscribe: subject}})
		socket := resp.GetStreamSubscribe().GetSocket()
		if err != nil || code != "" || socket == "" {
			return Fail("`stream.subscribe` of "+stream, "a delivery on a per-subscriber socket", fmt.Sprintf("%v %q %v", resp, code, err))
		}
		d, err := readDelivery(socket)
		if err != nil {
			return Fail("`stream.subscribe` of "+stream, "a socket the suite reaches", err.Error())
		}
		r.held = append(r.held, func() { d.conn.Close() })
		r.deliveries[stream] = d
		if resp, code, err := a.Call(&interfacev1.Request{Operation: &interfacev1.Request_StreamStart{StreamStart: subject}}); err != nil || code != "" {
			return Fail("`stream.start` of "+stream, "an acknowledgement", fmt.Sprintf("%v %q %v", resp, code, err))
		}
		o, before, ok := observed(h, "activated", 10*time.Second)
		if !ok || o.Fields["stream"] != stream || !slices.Contains(transports, fmt.Sprint(o.Fields["transport"])) {
			return Fail("`stream.start` of "+stream, "an observation `activated` naming it, on the transport "+strings.Join(transports, " or "),
				fmt.Sprintf("%v %v", o, before))
		}
		r.activated[stream] = fmt.Sprint(o.Fields["transport"])
	}
	return Pass()
}

// emittedThree issues three emissions on a stream and reads what its delivery carried.
func emittedThree(r *Run, stream string) ([]Frame, *Outcome) {
	h, err := r.Unit()
	if err != nil {
		o := Fail("`emit`", "the harness of case 13", err.Error())
		return nil, &o
	}
	d := r.deliveries[stream]
	if d == nil {
		o := Fail("`emit` on "+stream, "the delivery case 13 opened", "none")
		return nil, &o
	}
	payloads := []string{"one", "two", "three"}
	for _, p := range payloads {
		res := h.Do("emit", map[string]any{"stream": stream, "payload": p})
		if res.Unrecognised {
			o := Absent("emit")
			return nil, &o
		}
		if res.Refusal != "" || res.Lost {
			o := Fail("`emit` of "+p+" on "+stream, "an answer with no refusal", fmt.Sprintf("%q lost=%v", res.Refusal, res.Lost))
			return nil, &o
		}
	}
	var got []Frame
	for range payloads {
		f, ok := d.next(10 * time.Second)
		if !ok {
			break
		}
		got = append(got, f)
	}
	return got, nil
}

// std: yoke:plugin.14
func orderedInOrder(r *Run) Outcome {
	strict, _ := r.streamsDeclared()
	got, absent := emittedThree(r, strict)
	if absent != nil {
		return *absent
	}
	want := []Frame{{Sequence: 1, Payload: "one"}, {Sequence: 2, Payload: "two"}, {Sequence: 3, Payload: "three"}}
	if len(got) != len(want) {
		return Fail("`emit` three times on "+strict, "three frames, 1 `one`, 2 `two`, 3 `three`", fmt.Sprint(got))
	}
	for i := range want {
		if got[i].Sequence != want[i].Sequence || got[i].Payload != want[i].Payload {
			return Fail("`emit` three times on "+strict, "three frames, 1 `one`, 2 `two`, 3 `three`", fmt.Sprint(got))
		}
	}
	return Pass()
}

// std: yoke:plugin.15
func framedContiguous(r *Run) Outcome {
	_, lossy := r.streamsDeclared()
	got, absent := emittedThree(r, lossy)
	if absent != nil {
		return *absent
	}
	payloads := map[uint64]string{}
	for _, f := range got {
		payloads[f.Sequence] = f.Payload
	}
	if len(got) != 3 || payloads[1] != "one" || payloads[2] != "two" || payloads[3] != "three" {
		return Fail("`emit` three times on "+lossy, "the sequences 1, 2 and 3, carrying `one`, `two` and `three`", fmt.Sprint(got))
	}
	return Pass()
}

// std: yoke:plugin.16
func stoppedAndClosed(r *Run) Outcome {
	h, err := r.Unit()
	if err != nil {
		return Fail("`stream.stop`", "the harness of case 13", err.Error())
	}
	strict, _ := r.streamsDeclared()
	a, err := r.Attach()
	if err != nil {
		return Fail("`stream.stop` of "+strict, "the suite's attachment", err.Error())
	}
	subject := &interfacev1.UnitStream{Unit: h.Hello().Unit, Stream: strict}
	if resp, code, err := a.Call(&interfacev1.Request{Operation: &interfacev1.Request_StreamStop{StreamStop: subject}}); err != nil || code != "" {
		return Fail("`stream.stop` of "+strict, "an acknowledgement", fmt.Sprintf("%v %q %v", resp, code, err))
	}
	o, before, ok := observed(h, "stopped", 10*time.Second)
	if !ok || o.Fields["stream"] != strict {
		return Fail("`stream.stop` of "+strict, "an observation `stopped` naming it", fmt.Sprintf("%v %v", o, before))
	}
	res := h.Do("emit", map[string]any{"stream": strict, "payload": "after"})
	if res.Unrecognised {
		return Absent("emit")
	}
	if res.Refusal != "stream.inactive" {
		return Fail("`emit` on "+strict+" after its stop", "`stream.inactive`", fmt.Sprintf("%q %v", res.Refusal, res.Value))
	}
	return Pass()
}
