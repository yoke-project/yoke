package session_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"
)

// The test binary is also the Plugin the Core launches: this variable, declared in the composition, says so.
const role = "TEST_UNIT_ROLE"

func TestMain(m *testing.M) {
	switch os.Getenv(role) {
	case "session":
		os.Exit(openASession())
	case "direction":
		os.Exit(sendACommand())
	case "occurrence":
		os.Exit(reportAnOccurrence())
	}
	os.Exit(m.Run())
}

// openASession registers, opens the Session with the identity it was given, heartbeats, and closes it
// after a second.
func openASession() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("no channel:", err)
		return 1
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := pluginv1.NewRegisterClient(conn).Register(ctx, &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		Declared: &pluginv1.Surface{Capabilities: []string{"stream.data.publish"}, Streams: []string{"station.data"}},
	})
	if err != nil || resp.SessionId == "" {
		fmt.Println("not admitted:", err, resp)
		return 1
	}
	s, err := pluginv1.NewSessionClient(conn).Open(ctx)
	if err != nil {
		fmt.Println("no stream:", err)
		return 1
	}
	n := 0
	send := func(fill func(*pluginv1.Envelope)) {
		n++
		e := &pluginv1.Envelope{MessageId: fmt.Sprint(n), SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano()}
		fill(e)
		s.Send(e)
	}
	send(func(e *pluginv1.Envelope) {
		e.Payload = &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Open_{Open: &pluginv1.SessionMessage_Open{}}}}
	})
	for range 4 {
		send(func(e *pluginv1.Envelope) { e.Payload = &pluginv1.Envelope_Health{Health: &pluginv1.Health{Grade: 90}} })
		time.Sleep(250 * time.Millisecond)
	}
	send(func(e *pluginv1.Envelope) {
		e.Payload = &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Close_{Close: &pluginv1.SessionMessage_Close{}}}}
	})
	s.CloseSend()
	time.Sleep(time.Hour)
	return 0
}

// sendACommand registers, opens the Session, heartbeats, sends a command, and says what came back.
func sendACommand() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("no channel:", err)
		return 1
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := pluginv1.NewRegisterClient(conn).Register(ctx, &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		Declared: &pluginv1.Surface{Capabilities: []string{"stream.data.publish"}, Streams: []string{"station.data"}},
	})
	if err != nil || resp.SessionId == "" {
		fmt.Println("not admitted:", err, resp)
		return 1
	}
	s, err := pluginv1.NewSessionClient(conn).Open(ctx)
	if err != nil {
		fmt.Println("no stream:", err)
		return 1
	}
	s.Send(&pluginv1.Envelope{MessageId: "1", SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano(),
		Payload: &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Open_{Open: &pluginv1.SessionMessage_Open{}}}}})
	s.Send(&pluginv1.Envelope{MessageId: "2", SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano(),
		Payload: &pluginv1.Envelope_Control{Control: &pluginv1.Control{Kind: &pluginv1.Control_Command_{Command: &pluginv1.Control_Command{Type: "calibrate"}}}}})
	e, err := s.Recv()
	if err != nil {
		fmt.Println("nothing came back:", err)
		return 1
	}
	fmt.Printf("received %s for %s\n", e.GetError().GetCode(), e.CorrelationId)
	time.Sleep(time.Hour)
	return 0
}

// std: yoke:the-event-bus.05
func TestThroughTheCoreAUnitsLifeIsPublished(t *testing.T) {
	lines := runCore(t, "session")
	var said []string
	ready := false
	var seqs []int
	into := map[string]bool{}
	deadline := time.After(20 * time.Second)
	for !ready || !into["Starting"] || !into["Admitted"] || !into["Running"] {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			if !strings.Contains(line, "msg=event") {
				continue
			}
			var seq int
			if _, err := fmt.Sscanf(field(line, "seq"), "%d", &seq); err != nil {
				t.Fatalf("an event without its number: %s", line)
			}
			if len(seqs) > 0 && seq <= seqs[len(seqs)-1] {
				t.Fatalf("the event numbered %d follows %d: %s", seq, seqs[len(seqs)-1], line)
			}
			seqs = append(seqs, seq)
			ready = ready || field(line, "type") == "instance.ready"
			if field(line, "type") == "unit.state.changed" && field(line, "subject") == "unit:acquire#1" {
				into[field(line, "to")] = true
			}
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
}

// field is the value of one key in a line the Core's logger wrote.
func field(line, key string) string {
	for _, part := range strings.Fields(line) {
		if value, ok := strings.CutPrefix(part, key+"="); ok {
			return strings.Trim(value, `"`)
		}
	}
	return ""
}

// reportAnOccurrence registers declaring an occurrence, opens the Session, reports it, and says what
// came back.
func reportAnOccurrence() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("no channel:", err)
		return 1
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := pluginv1.NewRegisterClient(conn).Register(ctx, &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		Declared: &pluginv1.Surface{Capabilities: []string{"stream.data.publish", "event.calibration-drift.report"}, Streams: []string{"station.data"}},
	})
	if err != nil || resp.SessionId == "" {
		fmt.Println("not admitted:", err, resp)
		return 1
	}
	s, err := pluginv1.NewSessionClient(conn).Open(ctx)
	if err != nil {
		fmt.Println("no stream:", err)
		return 1
	}
	s.Send(&pluginv1.Envelope{MessageId: "1", SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano(),
		Payload: &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Open_{Open: &pluginv1.SessionMessage_Open{}}}}})
	s.Send(&pluginv1.Envelope{MessageId: "2", SessionId: resp.SessionId, SentAtUnixNano: time.Now().UnixNano(),
		Payload: &pluginv1.Envelope_Event{Event: &pluginv1.Event{Occurrence: "calibration.drift", Severity: 60, Line: "drifting"}}})
	e, err := s.Recv()
	if err != nil {
		fmt.Println("nothing came back:", err)
		return 1
	}
	fmt.Printf("received %s for %s\n", e.GetError().GetCode(), e.CorrelationId)
	time.Sleep(time.Hour)
	return 0
}

// std: yoke:the-granted-scope.05
func TestThroughTheCoreAnOccurrenceNobodyAuthorisedIsRefused(t *testing.T) {
	lines := runCore(t, "occurrence")
	var said []string
	refused, recorded := false, false
	deadline := time.After(20 * time.Second)
	for !refused || !recorded {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			refused = refused || strings.Contains(line, "received scope.withheld for 2")
			recorded = recorded || (strings.Contains(line, "msg=session") && strings.Contains(line, "unit=acquire") &&
				strings.Contains(line, "refused=scope.withheld"))
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
}

// std: yoke:the-session.10
func TestARegisteredUnitOpensItsSession(t *testing.T) {
	lines := runCore(t, "session")
	var said []string
	opened, closed := false, false
	deadline := time.After(20 * time.Second)
	for !opened || !closed {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			isSession := strings.Contains(line, "msg=session") && strings.Contains(line, "unit=acquire")
			opened = opened || (isSession && strings.Contains(line, "event=opened"))
			closed = closed || (opened && isSession && strings.Contains(line, "ended=closed"))
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
}

// std: yoke:the-families.07
func TestThroughTheCoreAUnitThatSendsACommandIsRefused(t *testing.T) {
	lines := runCore(t, "direction")
	var said []string
	refused, recorded := false, false
	deadline := time.After(20 * time.Second)
	for !refused || !recorded {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			refused = refused || strings.Contains(line, "received session.direction for 2")
			recorded = recorded || (strings.Contains(line, "msg=session") && strings.Contains(line, "unit=acquire") &&
				strings.Contains(line, "refused=session.direction"))
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
}

// manifestFor is the station's Manifest; the occurrence role's also declares an occurrence.
func manifestFor(as string) string {
	m := "manifest: 1\nid: com.example.station\nprotocol: 1\nstreams: [ { id: station.data } ]\n"
	if as == "occurrence" {
		return m + "occurrences: [ { id: calibration.drift } ]\ncapabilities: [ { name: stream.data.publish, governs: { stream: station.data } }, " +
			"{ name: event.calibration-drift.report, governs: { occurrence: calibration.drift } } ]\n"
	}
	return m + "capabilities: [ { name: stream.data.publish, governs: { stream: station.data } } ]\n"
}

// runCore starts yoke-core with one unit of this test binary in the role given, and hands back the
// Core's output line by line.
func runCore(t *testing.T, as string) <-chan string {
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
	os.WriteFile(filepath.Join(manifests, "com.example.station", "manifest.yaml"), []byte(
		manifestFor(as)), 0o644)
	self, _ := os.Executable()
	if err := os.Symlink(self, filepath.Join(executables, "com.example.station")); err != nil {
		t.Fatal(err)
	}
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte("units:\n  acquire: { kind: plugin, plugin: com.example.station, env: { "+role+": "+as+" } }\n"), 0o644)
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
	return lines
}
