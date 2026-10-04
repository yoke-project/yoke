package interfaces_test

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"
)

// The test binary is also the managed interface the Core launches: this variable, declared in the
// composition, says so.
const role = "TEST_UNIT_ROLE"

func TestMain(m *testing.M) {
	switch os.Getenv(role) {
	case "panel":
		os.Exit(reachTheChannel())
	case "once":
		os.Exit(0)
	case "commanded":
		os.Exit(acknowledgeCommands())
	case "streamer":
		os.Exit(emitWhenActivated())
	}
	os.Exit(m.Run())
}

// reachTheChannel connects to the address its environment names, says so, and stays.
func reachTheChannel() int {
	address := os.Getenv("YOKE_SOCKET")
	conn, err := net.Dial("unix", address)
	if err != nil {
		fmt.Println("no channel at", address, err)
		return 1
	}
	defer conn.Close()
	fmt.Println("connected to", address)
	time.Sleep(time.Hour)
	return 0
}

// startCore builds yoke-core and starts it with the composition given, whose %s is the test binary and
// %s the role variable; it returns the runtime directory and the Core's output, line by line.
func startCore(t *testing.T, composition string) (string, <-chan string) {
	t.Helper()
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
	self, _ := os.Executable()
	// The one Plugin a composition may run is this binary, declaring the command it acknowledges.
	os.WriteFile(filepath.Join(manifests, "com.example.station", "manifest.yaml"), []byte("manifest: 1\nid: com.example.station\nprotocol: 1\n"+
		"commands: [ { id: calibrate } ]\nstreams: [ { id: station.data } ]\n"+
		"capabilities: [ { name: command.calibrate.accept, governs: { command: calibrate } }, { name: stream.data.publish, governs: { stream: station.data } } ]\n"), 0o644)
	os.Symlink(self, filepath.Join(executables, "com.example.station"))
	path := filepath.Join(dir, "bench.yaml")
	os.WriteFile(path, []byte(fmt.Sprintf(composition, self, role)), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s\n  executables: %s\n", dir, run, manifests, executables)), 0o644)
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+path)
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
	return run, lines
}

// std: yoke:channels-bound.05
func TestThroughTheCoreChannelsAreBoundAndTheManagedInterfaceReachesItsOwn(t *testing.T) {
	run, lines := startCore(t, `units:
  panel-ui: { kind: interface, exec: %s, env: { %s: panel } }
channels:
  front: { unit: panel-ui, transport: local, clients: single }
  remote: { transport: http+ws, clients: multiple, address: { class: loopback, port: "`+freePort(t)+`" } }
`)
	socket := filepath.Join(run, "interfaces", "front.sock")
	weaker, reached := false, false
	var said []string
	deadline := time.After(20 * time.Second)
	for !weaker || !reached {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			weaker = weaker || (strings.Contains(line, "level=WARN") && strings.Contains(line, "finding") && strings.Contains(line, "remote"))
			reached = reached || strings.Contains(line, "connected to "+socket)
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
}

// std: yoke:the-local-projection.05
func TestThroughTheCoreAClientAttachesAndIsGivenTheOpening(t *testing.T) {
	run, lines := startCore(t, `# The test binary, %s, runs nothing here; %s is unused.
units: {}
channels:
  bench: { transport: local, clients: multiple }
`)
	var said []string
	deadline := time.After(20 * time.Second)
	for ready := false; !ready; {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			ready = strings.Contains(line, "msg=ready")
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
	go func() {
		for range lines {
		}
	}()
	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "interfaces", "bench.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err != nil {
		t.Fatalf("%v; the Core said:\n%s", err, strings.Join(said, "\n"))
	}
	f, err := stream.Recv()
	if err != nil || f.GetOpening().GetVersion() != 1 {
		t.Errorf("the first frame is %v %v", f, err)
	}
}

// std: yoke:attaching.07
func TestThroughTheCoreAClientAttachesIsToldWhoItIsAndASecondIsRefused(t *testing.T) {
	run, lines := startCore(t, `units:
  calibrate-once: { kind: oneshot, exec: %s, env: { %s: once } }
channels:
  panel: { transport: local, clients: single }
`)
	var said []string
	var mu sync.Mutex
	attachedSeen := make(chan struct{})
	ready := make(chan struct{})
	go func() {
		seenReady, seenAttached := false, false
		for line := range lines {
			mu.Lock()
			said = append(said, line)
			mu.Unlock()
			if !seenReady && strings.Contains(line, "msg=ready") {
				seenReady = true
				close(ready)
			}
			if !seenAttached && strings.Contains(line, "type=channel.attached") {
				seenAttached = true
				close(attachedSeen)
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(20 * time.Second):
		t.Fatal("the Core was not ready within twenty seconds")
	}
	address := filepath.Join(run, "interfaces", "panel.sock")
	a, err := attachTo(t, address)
	if err != nil {
		t.Fatal(err)
	}
	me, _ := user.LookupId(strconv.Itoa(os.Getuid()))
	picture := a.next(t).GetOpening().GetPicture()
	var unitSeen bool
	for _, r := range picture.GetRecords() {
		unitSeen = unitSeen || r.GetUnit().GetDeclared().GetIdentity() == "calibrate-once"
	}
	if records := channelRecord(picture); !unitSeen || len(records) != 1 || records[0].GetObserved().GetClient() != me.Username {
		t.Errorf("the picture is %v", picture)
	}
	select {
	case <-attachedSeen:
	case <-time.After(5 * time.Second):
		mu.Lock()
		t.Errorf("the Core recorded no channel.attached:\n%s", strings.Join(said, "\n"))
		mu.Unlock()
	}
	if _, err := attachTo(t, address); refusalIn(err).GetCode() != "channel.in_use" {
		t.Errorf("the second attachment was answered %v", err)
	}
}

// std: yoke:what-a-channel-sees.05
func TestThroughTheCoreAClientReadsItsChannelAndAnOperatorReadsEvery(t *testing.T) {
	run, lines := startCore(t, `units:
  calibrate-once: { kind: oneshot, exec: %s, env: { %s: once } }
channels:
  panel: { transport: local, clients: single }
  remote: { transport: local, clients: single }
`)
	ready := make(chan struct{})
	go func() {
		seen := false
		for line := range lines {
			if !seen && strings.Contains(line, "msg=ready") {
				seen = true
				close(ready)
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(20 * time.Second):
		t.Fatal("the Core was not ready within twenty seconds")
	}
	a, err := attachTo(t, filepath.Join(run, "interfaces", "panel.sock"))
	if err != nil {
		t.Fatal(err)
	}
	a.next(t)
	me, _ := user.LookupId(strconv.Itoa(os.Getuid()))
	own := a.answerTo(t, "own", readOf("channel", "panel")).GetAnswer().GetRead().GetRecords()
	if len(own) != 1 || own[0].GetChannel().GetObserved().GetClient() != me.Username {
		t.Errorf("the client read its channel as %v", own)
	}
	if f := a.answerTo(t, "other", readOf("channel", "remote")); f.GetRefusal().GetCode() != "subject.unknown" {
		t.Errorf("the client read the other channel as %v", f)
	}
	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := administrativev1.NewOperatorClient(conn).Call(ctx, &administrativev1.Request{Version: 1,
		Operation: &administrativev1.Request_Read{Read: &administrativev1.Read{Kind: "channel"}}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range resp.GetRead().GetRecords() {
		c := r.GetChannel()
		got[c.GetDeclared().GetName()] = c.GetObserved().GetClient()
	}
	if len(got) != 2 || got["panel"] != me.Username || got["remote"] != "" {
		t.Errorf("the operator read the channels as %v", got)
	}
}

// acknowledgeCommands registers, opens its Session, reports its health every second, and acknowledges
// every command as done, with a line.
func acknowledgeCommands() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 1
	}
	defer conn.Close()
	resp, err := pluginv1.NewRegisterClient(conn).Register(context.Background(), &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		Declared: &pluginv1.Surface{Capabilities: []string{"command.calibrate.accept", "stream.data.publish"}, Commands: []string{"calibrate"},
			Streams: []string{"station.data"}},
	})
	if err != nil || resp.SessionId == "" {
		fmt.Println("not admitted:", err, resp)
		return 1
	}
	s, err := pluginv1.NewSessionClient(conn).Open(context.Background())
	if err != nil {
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
	for {
		e, err := s.Recv()
		if err != nil {
			return 0
		}
		if e.GetControl().GetCommand() != nil {
			send(func(a *pluginv1.Envelope) {
				a.CorrelationId = e.MessageId
				a.Payload = &pluginv1.Envelope_Ack{Ack: &pluginv1.Ack{Outcome: pluginv1.Ack_OUTCOME_DONE, Line: "calibrated"}}
			})
		}
	}
}

// std: yoke:channel-operations.06
func TestThroughTheCoreACommandOnAChannelReachesTheUnit(t *testing.T) {
	run, lines := startCore(t, `units:
  acquire: { kind: plugin, plugin: com.example.station, env: { %[2]s: commanded } }
channels:
  panel: { transport: local, clients: single }
# %[1]s
`)
	_ = run
	opened := make(chan struct{}, 4)
	go func() {
		for line := range lines {
			if strings.Contains(line, "msg=session") && strings.Contains(line, "event=opened") {
				opened <- struct{}{}
			}
		}
	}()
	await := func(what string) {
		t.Helper()
		select {
		case <-opened:
		case <-time.After(20 * time.Second):
			t.Fatalf("no %s within twenty seconds", what)
		}
	}
	await("first Session")
	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	operator := administrativev1.NewOperatorClient(conn)
	for _, r := range []*administrativev1.Request{
		{Version: 1, Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: "com.example.station", Capability: "command.calibrate.accept"}}},
		{Version: 1, Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: "acquire"}}},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := operator.Call(ctx, r)
		cancel()
		if err != nil {
			t.Fatalf("%v was refused: %v", r, err)
		}
	}
	await("next life's Session")
	a, err := attachTo(t, filepath.Join(run, "interfaces", "panel.sock"))
	if err != nil {
		t.Fatal(err)
	}
	a.next(t)
	f := a.answerTo(t, "c", command("acquire", "calibrate", []byte("now")))
	if ack := f.GetAnswer().GetCommand(); ack.GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE || ack.GetLine() != "calibrated" {
		t.Errorf("the command was answered %v", f)
	}
}

// std: yoke:arbitration.06
func TestThroughTheCoreAPrevailingChannelSuspendsAnotherAndItsCommandIsRefused(t *testing.T) {
	run, lines := startCore(t, `units:
  calibrate-once: { kind: oneshot, exec: %s, env: { %s: once } }
channels:
  bench: { transport: local, clients: single }
  panel: { transport: local, clients: single }
arbitration:
  - { prevails: bench, over: [ panel ] }
`)
	ready, suspended := make(chan struct{}), make(chan struct{})
	go func() {
		r, s := false, false
		for line := range lines {
			if !r && strings.Contains(line, "msg=ready") {
				r = true
				close(ready)
			}
			if !s && strings.Contains(line, "type=channel.suspended") && strings.Contains(line, "channel:panel") {
				s = true
				close(suspended)
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(20 * time.Second):
		t.Fatal("the Core was not ready within twenty seconds")
	}
	panel, err := attachTo(t, filepath.Join(run, "interfaces", "panel.sock"))
	if err != nil {
		t.Fatal(err)
	}
	panel.next(t)
	bench, err := attachTo(t, filepath.Join(run, "interfaces", "bench.sock"))
	if err != nil {
		t.Fatal(err)
	}
	bench.next(t)
	select {
	case <-suspended:
	case <-time.After(5 * time.Second):
		t.Fatal("the Core recorded no suspension of panel")
	}
	ref := panel.answerTo(t, "c", command("calibrate-once", "calibrate", nil)).GetRefusal()
	if ref.GetCode() != "channel.suspended" || ref.GetSuspension().GetBy() != "bench" {
		t.Errorf("the command on panel was refused %v", ref)
	}
}

// emitWhenActivated registers as acknowledgeCommands does; told to activate its stream it accepts,
// connects to the address it was given and sends three data messages.
func emitWhenActivated() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 1
	}
	defer conn.Close()
	resp, err := pluginv1.NewRegisterClient(conn).Register(context.Background(), &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		Declared: &pluginv1.Surface{Capabilities: []string{"command.calibrate.accept", "stream.data.publish"}, Commands: []string{"calibrate"},
			Streams: []string{"station.data"}},
	})
	if err != nil || resp.SessionId == "" {
		fmt.Println("not admitted:", err, resp)
		return 1
	}
	s, err := pluginv1.NewSessionClient(conn).Open(context.Background())
	if err != nil {
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
	for {
		e, err := s.Recv()
		if err != nil {
			return 0
		}
		if a := e.GetControl().GetActivate(); a != nil {
			send(func(r *pluginv1.Envelope) {
				r.CorrelationId = e.MessageId
				r.Payload = &pluginv1.Envelope_Ack{Ack: &pluginv1.Ack{Outcome: pluginv1.Ack_OUTCOME_DONE}}
			})
			data, err := net.Dial("unixpacket", a.GetAddress())
			if err != nil {
				continue
			}
			for i := uint64(1); i <= 3; i++ {
				raw, _ := proto.Marshal(&pluginv1.Envelope{MessageId: fmt.Sprintf("d-%d", i), SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano(),
					Payload: &pluginv1.Envelope_Data{Data: &pluginv1.Data{Sequence: i, Payload: []byte(fmt.Sprint("sample ", i))}}})
				data.Write(raw)
			}
		}
	}
}

// std: yoke:streams-delivered.06
func TestThroughTheCoreAClientSubscribesStartsAndReads(t *testing.T) {
	run, lines := startCore(t, `units:
  acquire: { kind: plugin, plugin: com.example.station, env: { %[2]s: streamer } }
channels:
  panel: { transport: local, clients: single }
# %[1]s
`)
	opened := make(chan struct{}, 4)
	go func() {
		for line := range lines {
			if strings.Contains(line, "msg=session") && strings.Contains(line, "event=opened") {
				opened <- struct{}{}
			}
		}
	}()
	await := func(what string) {
		t.Helper()
		select {
		case <-opened:
		case <-time.After(20 * time.Second):
			t.Fatalf("no %s within twenty seconds", what)
		}
	}
	await("first Session")
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
	await("next life's Session")
	a, err := attachTo(t, filepath.Join(run, "interfaces", "panel.sock"))
	if err != nil {
		t.Fatal(err)
	}
	a.next(t)
	got := a.answerTo(t, "sub", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamSubscribe{StreamSubscribe: &interfacev1.UnitStream{Unit: "acquire", Stream: "station.data"}}})
	read := reader(t, got.GetAnswer().GetStreamSubscribe().GetSocket())
	start := a.answerTo(t, "start", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_StreamStart{StreamStart: &interfacev1.UnitStream{Unit: "acquire", Stream: "station.data"}}})
	if start.GetAnswer().GetStreamStart().GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE {
		t.Fatalf("the start was answered %v", start)
	}
	for i := uint64(1); i <= 3; i++ {
		p := read()
		if len(p) < 16 || binary.LittleEndian.Uint64(p[0:8]) != i || string(p[16:]) != fmt.Sprint("sample ", i) {
			t.Errorf("message %d read as %x", i, p)
		}
	}
}
