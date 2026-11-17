package interfaces_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
)

// std: yoke:a-stream-nobody-reads.01
func TestThroughTheCoreAStreamOutlivesTheClientThatStartedIt(t *testing.T) {
	run, lines := startCore(t, `units:
  acquire: { kind: plugin, plugin: com.example.station, env: { %[2]s: streamer } }
channels:
  panel: { transport: local, clients: multiple }
# %[1]s
`)
	opened, detached := make(chan struct{}, 4), make(chan struct{}, 4)
	go func() {
		for line := range lines {
			if strings.Contains(line, "msg=session") && strings.Contains(line, "event=opened") {
				opened <- struct{}{}
			}
			if strings.Contains(line, "channel.detached") {
				detached <- struct{}{}
			}
		}
	}()
	await := func(what string, c chan struct{}) {
		t.Helper()
		select {
		case <-c:
		case <-time.After(20 * time.Second):
			t.Fatalf("no %s within twenty seconds", what)
		}
	}
	await("first Session", opened)
	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	operator := administrativev1.NewOperatorClient(conn)
	for _, r := range []*administrativev1.Request{
		{Version: 1, Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: "com.example.station", Capability: "stream.data.publish"}}},
		{Version: 1, Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: "acquire"}}},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := operator.Call(ctx, r)
		cancel()
		if err != nil {
			t.Fatalf("%v was refused: %v", r, err)
		}
	}
	await("next life's Session", opened)
	socket := filepath.Join(run, "interfaces", "panel.sock")
	stream := &interfacev1.UnitStream{Unit: "acquire", Stream: "station.data"}
	subscribe := &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamSubscribe{StreamSubscribe: stream}}

	// The first client starts the stream without subscribing to it, and goes away.
	first, gone := attachLeaving(t, socket)
	first.next(t)
	if f := first.answerTo(t, "start", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamStart{StreamStart: stream}}); f.GetAnswer().GetStreamStart().GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE {
		t.Fatalf("the start was answered %v", f)
	}
	gone()
	await("detachment of the client that started the stream", detached)

	// A client attaching later finds it still flowing: nothing stopped it when its starter left.
	second, err := attachTo(t, socket)
	if err != nil {
		t.Fatal(err)
	}
	second.next(t)
	if f := second.answerTo(t, "sub", subscribe); !f.GetAnswer().GetStreamSubscribe().GetFlowing() {
		t.Fatalf("after its starter left, the stream is answered as not flowing: %v", f)
	}

	// What stops it is a stop, from any client: then it is not flowing for anyone.
	if f := second.answerTo(t, "stop", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamStop{StreamStop: stream}}); f.GetAnswer().GetStreamStop().GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE {
		t.Fatalf("the stop was answered %v", f)
	}
	third, err := attachTo(t, socket)
	if err != nil {
		t.Fatal(err)
	}
	third.next(t)
	if f := third.answerTo(t, "sub", subscribe); f.GetAnswer().GetStreamSubscribe().GetFlowing() {
		t.Errorf("after a stop, the stream is answered as flowing: %v", f)
	}
}

// attachLeaving attaches as attachTo does and returns, beside the attachment, what ends its connection.
func attachLeaving(t *testing.T, address string) (*attached, func()) {
	t.Helper()
	conn, err := grpc.NewClient("unix://"+address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
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
	leave := func() { cancel(); conn.Close() }
	t.Cleanup(leave)
	return a, leave
}
