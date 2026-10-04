package streams_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"
)

// The test binary is also the Plugin the Core launches: this variable, declared in the composition, says so.
const role = "TEST_UNIT_ROLE"

func TestMain(m *testing.M) {
	if os.Getenv(role) == "stream" {
		os.Exit(emitWhenActivated())
	}
	os.Exit(m.Run())
}

// emitWhenActivated registers, opens its Session and reports its health; told to activate its stream, it
// accepts, connects to the address it was given and sends three data messages; told to stop, it is done.
func emitWhenActivated() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("no channel:", err)
		return 1
	}
	defer conn.Close()
	resp, err := pluginv1.NewRegisterClient(conn).Register(context.Background(), &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		Declared: &pluginv1.Surface{Capabilities: []string{"stream.data.publish"}, Streams: []string{"station.data"}},
	})
	if err != nil || resp.SessionId == "" {
		fmt.Println("not admitted:", err, resp)
		return 1
	}
	s, err := pluginv1.NewSessionClient(conn).Open(context.Background())
	if err != nil {
		fmt.Println("no stream:", err)
		return 1
	}
	var mu sync.Mutex
	n := 0
	send := func(fill func(*pluginv1.Envelope)) {
		mu.Lock()
		defer mu.Unlock()
		n++
		e := &pluginv1.Envelope{MessageId: fmt.Sprint(n), SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano()}
		fill(e)
		s.Send(e)
	}
	send(func(e *pluginv1.Envelope) {
		e.Payload = &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Open_{Open: &pluginv1.SessionMessage_Open{}}}}
	})
	go func() {
		for {
			send(func(e *pluginv1.Envelope) {
				e.Payload = &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: 10, Line: "ready"}}
			})
			time.Sleep(time.Second)
		}
	}()
	acknowledge := func(to string) {
		send(func(e *pluginv1.Envelope) {
			e.CorrelationId = to
			e.Payload = &pluginv1.Envelope_Ack{Ack: &pluginv1.Ack{Outcome: pluginv1.Ack_OUTCOME_DONE}}
		})
	}
	for {
		e, err := s.Recv()
		if err != nil {
			return 0
		}
		switch c := e.GetControl(); {
		case c.GetActivate() != nil:
			acknowledge(e.MessageId)
			data, err := net.Dial("unixpacket", c.GetActivate().GetAddress())
			if err != nil {
				fmt.Println("no transport:", err)
				continue
			}
			for i := uint64(1); i <= 3; i++ {
				raw, _ := proto.Marshal(&pluginv1.Envelope{MessageId: fmt.Sprintf("d-%d", i), SessionId: resp.SessionId,
					SentAtUnixNano: time.Now().UnixNano(), Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: i, Payload: []byte("sample")}}})
				data.Write(raw)
			}
		case c.GetStop() != nil:
			acknowledge(e.MessageId)
		}
	}
}

// std: yoke:a-streams-transport.08
func TestThroughTheCoreAStreamStartedFlowsAndStops(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	manifests, executables := filepath.Join(dir, "plugins.d"), filepath.Join(dir, "plugins")
	os.MkdirAll(filepath.Join(manifests, "com.example.station"), 0o755)
	os.MkdirAll(executables, 0o755)
	os.WriteFile(filepath.Join(manifests, "com.example.station", "manifest.yaml"), []byte("manifest: 1\nid: com.example.station\nprotocol: 1\n"+
		"streams: [ { id: station.data } ]\ncapabilities: [ { name: stream.data.publish, governs: { stream: station.data } } ]\n"), 0o644)
	self, _ := os.Executable()
	if err := os.Symlink(self, filepath.Join(executables, "com.example.station")); err != nil {
		t.Fatal(err)
	}
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte("units:\n  acquire: { kind: plugin, plugin: com.example.station, env: { "+role+": stream } }\n"), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s\n  executables: %s\n", dir, run, manifests, executables)), 0o644)
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+composition)
	out, _ := command.StderrPipe()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	var said []string
	await := func(what string, holds func(string) bool) {
		t.Helper()
		deadline := time.After(20 * time.Second)
		for {
			select {
			case line, open := <-lines:
				if !open {
					t.Fatalf("the Core exited before %s:\n%s", what, strings.Join(said, "\n"))
				}
				said = append(said, line)
				if holds(line) {
					return
				}
			case <-deadline:
				t.Fatalf("within twenty seconds, no %s; the Core said:\n%s", what, strings.Join(said, "\n"))
			}
		}
	}
	opened := func(l string) bool { return strings.Contains(l, "msg=session") && strings.Contains(l, "event=opened") }
	await("the first Session", opened)

	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	operator := administrativev1.NewOperatorClient(conn)
	call := func(r *administrativev1.Request) *administrativev1.Response {
		t.Helper()
		r.Version = 1
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp, err := operator.Call(ctx, r)
		if err != nil {
			t.Fatalf("%v was refused: %v", r, err)
		}
		return resp
	}
	call(&administrativev1.Request{Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: "com.example.station", Capability: "stream.data.publish"}}})
	call(&administrativev1.Request{Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: "acquire"}}})
	await("the next life's Session", opened)

	stream := &administrativev1.UnitStream{Unit: "acquire", Stream: "station.data"}
	if resp := call(&administrativev1.Request{Operation: &administrativev1.Request_UnitStreamStart{UnitStreamStart: stream}}); resp.GetUnitStreamStart() == nil {
		t.Fatalf("the start answered %v", resp)
	}
	await("the activation", func(l string) bool { return strings.Contains(l, "type=unit.stream.activated") })
	time.Sleep(300 * time.Millisecond)
	if resp := call(&administrativev1.Request{Operation: &administrativev1.Request_UnitStreamStop{UnitStreamStop: stream}}); resp.GetUnitStreamStop() == nil {
		t.Fatalf("the stop answered %v", resp)
	}
	await("the stream stopped as asked, with three read", func(l string) bool {
		return strings.Contains(l, "msg=stream") && strings.Contains(l, "event=stopped") && strings.Contains(l, "reason=asked") && strings.Contains(l, "read=3")
	})
	if _, err := os.Stat(filepath.Join(run, "plugins", "acquire", "streams", "station.data.sock")); err == nil {
		t.Error("the stream's socket is still there")
	}
}
