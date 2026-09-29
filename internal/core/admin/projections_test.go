package admin_test

import (
	"context"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"

	"github.com/yoke-project/yoke/internal/core/admin"
	"github.com/yoke-project/yoke/internal/core/event"
)

// fakes are operations of every shape: a read answered once, which waits for release when it names
// slow; a subscription that streams two answers and ends; and a follow that streams until cancelled.
type fakes struct {
	release  chan struct{}
	followed chan struct{} // closed when a follow saw its context end
}

func newFakes() *fakes {
	return &fakes{release: make(chan struct{}), followed: make(chan struct{})}
}

var (
	records    = &administrativev1.Response{Answer: &administrativev1.Response_Read{Read: &administrativev1.Records{}}}
	subscribed = &administrativev1.Response{Answer: &administrativev1.Response_Subscribe{Subscribe: &administrativev1.Subscribed{}}}
	followed   = &administrativev1.Response{Answer: &administrativev1.Response_LogFollow{LogFollow: &administrativev1.Followed{Carries: &administrativev1.Followed_BehindAt{BehindAt: 7}}}}
)

func (f *fakes) operations() map[string]admin.Operation {
	return map[string]admin.Operation{
		"read": {Answer: func(ctx context.Context, _ event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
			if r.GetRead().GetIdentity() == "slow" {
				<-f.release
			}
			return records, nil
		}},
		"subscribe": {Stream: func(ctx context.Context, _ event.Actor, _ *administrativev1.Request, send func(*administrativev1.Response) error) *administrativev1.Refusal {
			send(subscribed)
			send(subscribed)
			return nil
		}},
		"log.follow": {Stream: func(ctx context.Context, _ event.Actor, _ *administrativev1.Request, send func(*administrativev1.Response) error) *administrativev1.Refusal {
			send(followed)
			<-ctx.Done()
			close(f.followed)
			return nil
		}},
	}
}

func read(identity string) *administrativev1.Request {
	return &administrativev1.Request{Version: 1, Operation: &administrativev1.Request_Read{Read: &administrativev1.Read{Kind: "unit", Identity: identity}}}
}

var (
	subscribe = &administrativev1.Request{Version: 1, Operation: &administrativev1.Request_Subscribe{Subscribe: &administrativev1.Subscribe{}}}
	follow    = &administrativev1.Request{Version: 1, Operation: &administrativev1.Request_LogFollow{LogFollow: &administrativev1.LogFollow{}}}
	nothing   = &administrativev1.Request{Version: 1}
)

func issue(t *testing.T, stream administrativev1.Shell_ConnectClient, call string, r *administrativev1.Request) {
	t.Helper()
	if err := stream.Send(&administrativev1.ClientFrame{Call: call, Carries: &administrativev1.ClientFrame_Request{Request: r}}); err != nil {
		t.Fatal(err)
	}
}

func cancel(t *testing.T, stream administrativev1.Shell_ConnectClient, call string) {
	t.Helper()
	if err := stream.Send(&administrativev1.ClientFrame{Call: call, Carries: &administrativev1.ClientFrame_Cancel{Cancel: &administrativev1.Cancel{}}}); err != nil {
		t.Fatal(err)
	}
}

// next reads the next frame, failing the test on none within two seconds.
func next(t *testing.T, stream administrativev1.Shell_ConnectClient) *administrativev1.CoreFrame {
	t.Helper()
	got := make(chan *administrativev1.CoreFrame, 1)
	go func() {
		f, err := stream.Recv()
		if err != nil {
			close(got)
			return
		}
		got <- f
	}()
	select {
	case f, ok := <-got:
		if !ok {
			t.Fatal("the connection ended")
		}
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("no frame within two seconds")
	}
	return nil
}

// refusalOf is the refusal a status carries.
func refusalOf(err error) *administrativev1.Refusal {
	for _, d := range status.Convert(err).Details() {
		if r, ok := d.(*administrativev1.Refusal); ok {
			return r
		}
	}
	return nil
}

// std: yoke:the-two-projections.01
func TestBothProjectionsCarryTheSameOperationsAlike(t *testing.T) {
	f := newFakes()
	_, operator, shell := served(t, admin.Config{Operations: f.operations()})
	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	called, err := operator.Call(ctx, read("acquire"))
	if err != nil || !proto.Equal(called, records) {
		t.Fatalf("Call answered %v, %v", called, err)
	}
	stream, _, _ := opened(t, shell)
	issue(t, stream, "r", read("acquire"))
	if frame := next(t, stream); frame.GetCall() != "r" || !proto.Equal(frame.GetAnswer(), called) {
		t.Errorf("the shell answered %v, want what Call answered", frame)
	}

	watched, err := operator.Watch(ctx, subscribe)
	if err != nil {
		t.Fatal(err)
	}
	var byWatch []*administrativev1.Response
	for {
		r, err := watched.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("the watch ended with %v", err)
		}
		byWatch = append(byWatch, r)
	}
	issue(t, stream, "s", subscribe)
	for i, want := range byWatch {
		if frame := next(t, stream); frame.GetCall() != "s" || !proto.Equal(frame.GetAnswer(), want) {
			t.Errorf("the shell's answer %d is %v, want %v", i, frame, want)
		}
	}
	if len(byWatch) != 2 {
		t.Errorf("the watch streamed %d answers, want 2", len(byWatch))
	}
}

// std: yoke:the-two-projections.02
func TestTheWrongMethodIsRefusedAsMalformed(t *testing.T) {
	f := newFakes()
	_, operator, shell := served(t, admin.Config{Operations: f.operations()})
	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	for name, r := range map[string]*administrativev1.Request{"subscribe": subscribe, "no operation": nothing} {
		_, err := operator.Call(ctx, r)
		if got := refusalOf(err); got.GetCode() != "operation.malformed" {
			t.Errorf("Call of %s was answered %v, want operation.malformed", name, err)
		}
	}
	for name, r := range map[string]*administrativev1.Request{"read": read("acquire"), "no operation": nothing} {
		watched, err := operator.Watch(ctx, r)
		if err == nil {
			_, err = watched.Recv()
		}
		if got := refusalOf(err); got.GetCode() != "operation.malformed" {
			t.Errorf("Watch of %s was answered %v, want operation.malformed", name, err)
		}
	}
	stream, _, _ := opened(t, shell)
	issue(t, stream, "z", nothing)
	if frame := next(t, stream); frame.GetCall() != "z" || frame.GetRefusal().GetCode() != "operation.malformed" {
		t.Errorf("the shell answered %v, want a refusal operation.malformed carrying z", frame)
	}
}

// std: yoke:the-two-projections.03
func TestCallsInterleaveEachAnswerCarryingItsIdentity(t *testing.T) {
	f := newFakes()
	_, _, shell := served(t, admin.Config{Operations: f.operations()})
	stream, _, _ := opened(t, shell)
	issue(t, stream, "a", read("slow"))
	issue(t, stream, "b", read("acquire"))
	if frame := next(t, stream); frame.GetCall() != "b" || frame.GetAnswer() == nil {
		t.Fatalf("the first frame is %v, want b's answer", frame)
	}
	close(f.release)
	if frame := next(t, stream); frame.GetCall() != "a" || frame.GetAnswer() == nil {
		t.Errorf("the second frame is %v, want a's answer", frame)
	}
}

// std: yoke:the-two-projections.04
func TestEveryCallCompletesExplicitly(t *testing.T) {
	f := newFakes()
	_, _, shell := served(t, admin.Config{Operations: f.operations()})
	stream, _, _ := opened(t, shell)
	issue(t, stream, "s", subscribe)
	for i := range 2 {
		if frame := next(t, stream); frame.GetCall() != "s" || frame.GetAnswer() == nil {
			t.Fatalf("frame %d is %v, want an answer carrying s", i, frame)
		}
	}
	if frame := next(t, stream); frame.GetCall() != "s" || frame.GetCompletion() == nil {
		t.Fatalf("after its answers s got %v, want its completion", frame)
	}

	issue(t, stream, "f", follow)
	if frame := next(t, stream); frame.GetCall() != "f" || frame.GetAnswer() == nil {
		t.Fatalf("f got %v, want its first answer", frame)
	}
	issue(t, stream, "f", read("acquire"))
	if frame := next(t, stream); frame.GetCall() != "f" || frame.GetRefusal().GetCode() != "operation.malformed" {
		t.Errorf("a second f while f is in flight got %v, want operation.malformed", frame)
	}
	cancel(t, stream, "f")
	if frame := next(t, stream); frame.GetCall() != "f" || frame.GetCompletion() == nil {
		t.Errorf("after its cancellation f got %v, want its completion", frame)
	}
	select {
	case <-f.followed:
	case <-time.After(2 * time.Second):
		t.Error("the follow did not see its context end")
	}
}

// std: yoke:the-two-projections.05
func TestAnOperatorStreamIsAConnectionWhileItLasts(t *testing.T) {
	f := newFakes()
	var p published
	s, operator, _ := served(t, admin.Config{Operations: f.operations(), Publish: p.publish})
	ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
	defer done()
	if _, err := operator.Call(ctx, read("acquire")); err != nil {
		t.Fatal(err)
	}
	if n := len(p.of("connection.opened")); n != 0 || len(s.Connections()) != 0 {
		t.Errorf("a call answered once opened %d connections, and %v are listed", n, s.Connections())
	}
	watching, stop := context.WithCancel(ctx)
	watched, err := operator.Watch(watching, follow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := watched.Recv(); err != nil {
		t.Fatal(err)
	}
	listed := s.Connections()
	if len(listed) != 1 || listed[0].Projection != admin.OperatorProjection {
		t.Fatalf("while the watch lasts the surface lists %v, want one operator connection", listed)
	}
	if opens := p.of("connection.opened"); len(opens) != 1 || opens[0].Subject.ID != listed[0].ID {
		t.Errorf("connection.opened is %v, want one about %s", opens, listed[0].ID)
	}
	stop()
	deadline := time.Now().Add(3 * time.Second)
	for len(s.Connections()) != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	closes := p.of("connection.closed")
	if len(s.Connections()) != 0 || len(closes) != 1 || string(closes[0].Detail) != `{"projection":"operator","reason":"cancelled"}` {
		t.Errorf("after the cancellation the surface lists %v and published %v", s.Connections(), closes)
	}
}
