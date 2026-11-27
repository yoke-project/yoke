package conformance

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/ring"
)

// Producing is the variable the suite launches itself with as the producer an interface run composes.
const Producing = "CONFORMANCE_PRODUCE"

// Producer is the unit an interface run composes from the fixture: a Plugin of the suite's own, written on
// the plugin contract with no family's library, so an interface run measures the interface library alone.
const Producer = "producer"

// The stream the producer emits on, and what it emits: one frame above 1 MiB every half second, on the
// shared ring, while the stream flows.
const (
	ProducedStream = "fixture.frames"
	ProducedSize   = 1572864
	producedEvery  = 500 * time.Millisecond
)

// produced is the payload of every frame the producer emits: printable, byte i being 'a' + i mod 26.
func produced() []byte {
	p := make([]byte, ProducedSize)
	for i := range p {
		p[i] = byte('a' + i%26)
	}
	return p
}

// Produce runs the producer: it registers offering the shared ring, opens its Session, reports its
// health, and publishes into each activation's ring until the stream stops. It returns when the Session
// ends.
func Produce() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "producer: no channel:", err)
		return 1
	}
	defer conn.Close()
	// The unit binds its own socket before it registers.
	if bind := os.Getenv("YOKE_BIND"); bind != "" {
		os.Remove(bind)
		if l, err := net.Listen("unix", bind); err == nil {
			defer l.Close()
		}
	}
	resp, err := pluginv1.NewRegisterClient(conn).Register(context.Background(), &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		Language: "go", SdkLine: "yoke-conformance producer",
		Declared:   &pluginv1.Surface{Capabilities: []string{"stream.frames.publish", "stream.data.publish", "query.status.answer"}, Streams: []string{ProducedStream, "fixture.data"}, Queries: []string{FixtureQuery}},
		Transports: []pluginv1.Control_Activate_Transport{pluginv1.Control_Activate_TRANSPORT_SHARED_OBJECT},
	})
	if err != nil || resp.SessionId == "" {
		fmt.Fprintln(os.Stderr, "producer: not admitted:", err, resp)
		return 1
	}
	s, err := pluginv1.NewSessionClient(conn).Open(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, "producer: no Session:", err)
		return 1
	}
	var mu sync.Mutex
	n := 0
	send := func(fill func(*pluginv1.Envelope)) {
		mu.Lock()
		defer mu.Unlock()
		n++
		e := &pluginv1.Envelope{MessageId: fmt.Sprint("p-", n), SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano()}
		fill(e)
		s.Send(e)
	}
	send(func(e *pluginv1.Envelope) {
		e.Payload = &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Open_{Open: &pluginv1.SessionMessage_Open{}}}}
	})
	go func() {
		for {
			send(func(e *pluginv1.Envelope) {
				e.Payload = &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: 0, Line: ProducerLine}}
			})
			time.Sleep(Beat)
		}
	}()
	acknowledge := func(to string, outcome pluginv1.Ack_Outcome, line string) {
		send(func(e *pluginv1.Envelope) {
			e.CorrelationId = to
			e.Payload = &pluginv1.Envelope_Ack{Ack: &pluginv1.Ack{Outcome: outcome, Line: line}}
		})
	}
	var stop chan struct{}
	for {
		e, err := s.Recv()
		if err != nil {
			return 0
		}
		c := e.GetControl()
		switch {
		case c.GetActivate() != nil:
			a := c.GetActivate()
			if a.GetTransport() != pluginv1.Control_Activate_TRANSPORT_SHARED_OBJECT {
				// Only the ring is offered; a stream on any other transport is acknowledged and left quiet.
				acknowledge(e.MessageId, pluginv1.Ack_OUTCOME_DONE, "quiet")
				continue
			}
			w, lifeline, err := takeRing(a.GetAddress())
			if err != nil {
				acknowledge(e.MessageId, pluginv1.Ack_OUTCOME_FAILED, err.Error())
				continue
			}
			acknowledge(e.MessageId, pluginv1.Ack_OUTCOME_DONE, "")
			stop = make(chan struct{})
			go publish(w, lifeline, stop)
		case c.GetStop() != nil:
			if stop != nil {
				close(stop)
				stop = nil
			}
			acknowledge(e.MessageId, pluginv1.Ack_OUTCOME_DONE, "")
		}
	}
}

// takeRing connects to a ring's socket and takes the descriptor the Core hands the first connection.
func takeRing(address string) (*ring.Writer, *net.UnixConn, error) {
	c, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: address, Net: "unixpacket"})
	if err != nil {
		return nil, nil, err
	}
	buf, oob := make([]byte, 1), make([]byte, unix.CmsgSpace(4))
	_, oobn, _, _, err := c.ReadMsgUnix(buf, oob)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	messages, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil || len(messages) != 1 {
		c.Close()
		return nil, nil, fmt.Errorf("no descriptor: %v", err)
	}
	fds, err := unix.ParseUnixRights(&messages[0])
	if err != nil || len(fds) != 1 {
		c.Close()
		return nil, nil, fmt.Errorf("no descriptor: %v", err)
	}
	f := os.NewFile(uintptr(fds[0]), "ring")
	defer f.Close()
	w, err := ring.OpenWriter(f)
	if err != nil {
		c.Close()
		return nil, nil, err
	}
	return w, c, nil
}

// publish writes a frame into the ring at its pace until the stream stops, then lets the ring go.
func publish(w *ring.Writer, lifeline *net.UnixConn, stop chan struct{}) {
	defer lifeline.Close()
	defer w.Close()
	payload := produced()
	tick := time.NewTicker(producedEvery)
	defer tick.Stop()
	for {
		if err := w.Publish(uint64(time.Now().UnixNano()), payload); err != nil {
			return
		}
		select {
		case <-stop:
			return
		case <-tick.C:
		}
	}
}
