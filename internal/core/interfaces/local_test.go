package interfaces_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/interfaces"
)

// attached is a client attached to a surface served on a socket of its own.
type attached struct {
	stream interfacev1.Interface_AttachClient
	frames chan *interfacev1.CoreFrame
}

func attach(t *testing.T, cfg interfaces.Config) *attached {
	t.Helper()
	path := filepath.Join(root(t), "panel.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	interfacev1.RegisterInterfaceServer(server, interfaces.NewSurface(cfg))
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a := &attached{stream: stream, frames: make(chan *interfacev1.CoreFrame, 64)}
	go func() {
		defer close(a.frames)
		for {
			f, err := stream.Recv()
			if err != nil {
				return
			}
			a.frames <- f
		}
	}()
	return a
}

func (a *attached) next(t *testing.T) *interfacev1.CoreFrame {
	t.Helper()
	select {
	case f, open := <-a.frames:
		if !open {
			t.Fatal("the attachment ended")
		}
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no frame arrived")
		return nil
	}
}

func (a *attached) call(t *testing.T, call string, r *interfacev1.Request) {
	t.Helper()
	if err := a.stream.Send(&interfacev1.ClientFrame{Call: call, Carries: &interfacev1.ClientFrame_Request{Request: r}}); err != nil {
		t.Fatal(err)
	}
}

func read() *interfacev1.Request {
	return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Read{Read: &interfacev1.Read{Kind: "instance"}}}
}

func subscribe() *interfacev1.Request {
	return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Subscribe{Subscribe: &interfacev1.Subscribe{}}}
}

// served is a channel serving read, answered once, and a subscribe that streams until it is cancelled.
func served() interfaces.Config {
	return interfaces.Config{Operations: map[string]interfaces.Operation{
		"read": {Answer: func(context.Context, *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal) {
			return &interfacev1.Response{Answer: &interfacev1.Response_Read{Read: &interfacev1.Records{}}}, nil
		}},
		"subscribe": {Stream: func(ctx context.Context, _ *interfacev1.Request, send func(*interfacev1.Response) error) *interfacev1.Refusal {
			send(&interfacev1.Response{Answer: &interfacev1.Response_Subscribe{Subscribe: &interfacev1.Subscribed{}}})
			<-ctx.Done()
			return nil
		}},
	}}
}

// std: yoke:the-local-projection.01
func TestTheFirstFrameIsTheOpening(t *testing.T) {
	a := attach(t, interfaces.Config{Picture: func() *interfacev1.Snapshot { return &interfacev1.Snapshot{At: 7} }})
	f := a.next(t)
	if o := f.GetOpening(); f.GetCall() != "" || o == nil || o.GetVersion() != 1 || o.GetPicture().GetAt() != 7 {
		t.Errorf("the first frame is %v", f)
	}
}

// std: yoke:the-local-projection.02
func TestACallIsAnsweredWithItsIdentityAndARefusalEndsNothing(t *testing.T) {
	a := attach(t, served())
	a.next(t)
	a.call(t, "a", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Command{Command: &interfacev1.Command{Unit: "acquire", Type: "calibrate"}}})
	if f := a.next(t); f.GetCall() != "a" || f.GetRefusal().GetCode() != "operation.unknown" {
		t.Errorf("call a was answered %v", f)
	}
	a.call(t, "b", read())
	if f := a.next(t); f.GetCall() != "b" || f.GetAnswer().GetRead() == nil {
		t.Errorf("call b was answered %v", f)
	}
}

// std: yoke:the-local-projection.03
func TestTheVersionAMalformedRequestAndACallInFlightAreRefused(t *testing.T) {
	a := attach(t, served())
	a.next(t)
	wrong := read()
	wrong.Version = 2
	a.call(t, "v", wrong)
	if f := a.next(t); f.GetCall() != "v" || f.GetRefusal().GetCode() != "compat.unsupported" {
		t.Errorf("version 2 was answered %v", f)
	}
	a.call(t, "m", &interfacev1.Request{Version: 1})
	if f := a.next(t); f.GetCall() != "m" || f.GetRefusal().GetCode() != "operation.malformed" {
		t.Errorf("a request naming no operation was answered %v", f)
	}
	a.call(t, "s", subscribe())
	if f := a.next(t); f.GetCall() != "s" || f.GetAnswer().GetSubscribe() == nil {
		t.Fatalf("the subscription began with %v", f)
	}
	a.call(t, "s", read())
	if f := a.next(t); f.GetCall() != "s" || f.GetRefusal().GetCode() != "operation.malformed" {
		t.Errorf("a second request on a call in flight was answered %v", f)
	}
	select {
	case f := <-a.frames:
		t.Errorf("the subscription was ended: %v", f)
	case <-time.After(200 * time.Millisecond):
	}
}

// std: yoke:the-local-projection.04
func TestAStreamCompletesExplicitlyAndSaysWhoEndedIt(t *testing.T) {
	cfg := served()
	ending := cfg.Operations["subscribe"]
	cfg.Operations["subscribe"] = interfaces.Operation{Stream: func(ctx context.Context, r *interfacev1.Request, send func(*interfacev1.Response) error) *interfacev1.Refusal {
		if r.GetSubscribe().GetFilter().GetType() == "two" {
			send(&interfacev1.Response{Answer: &interfacev1.Response_Subscribe{Subscribe: &interfacev1.Subscribed{}}})
			send(&interfacev1.Response{Answer: &interfacev1.Response_Subscribe{Subscribe: &interfacev1.Subscribed{}}})
			return nil
		}
		return ending.Stream(ctx, r, send)
	}}
	a := attach(t, cfg)
	a.next(t)
	two := subscribe()
	two.GetSubscribe().Filter = &interfacev1.Filter{Type: "two"}
	a.call(t, "e", two)
	for i := 0; i < 2; i++ {
		if f := a.next(t); f.GetCall() != "e" || f.GetAnswer() == nil {
			t.Errorf("answer %d of e is %v", i, f)
		}
	}
	if f := a.next(t); f.GetCall() != "e" || f.GetCompletion().GetBy() != interfacev1.Completion_BY_CORE {
		t.Errorf("e ended with %v", f)
	}
	a.call(t, "c", subscribe())
	a.next(t)
	a.stream.Send(&interfacev1.ClientFrame{Call: "c", Carries: &interfacev1.ClientFrame_Cancel{Cancel: &interfacev1.Cancel{}}})
	if f := a.next(t); f.GetCall() != "c" || f.GetCompletion().GetBy() != interfacev1.Completion_BY_CALLER {
		t.Errorf("c ended with %v", f)
	}
}
