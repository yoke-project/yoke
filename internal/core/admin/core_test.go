package admin_test

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

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// started builds yoke-core and starts it in the service form, with nothing to run; it returns the
// runtime directory and a wait for a line of its output.
func started(t *testing.T, manifests ...string) (string, func(what string, holds func(string) bool)) {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	declared, executables := filepath.Join(dir, "plugins.d"), filepath.Join(dir, "plugins")
	os.MkdirAll(declared, 0o755)
	os.MkdirAll(executables, 0o755)
	for _, m := range manifests {
		// A Manifest lives in a directory named for the plugin it declares.
		id := strings.TrimSpace(strings.SplitN(strings.SplitN(m, "id:", 2)[1], "\n", 2)[0])
		path := filepath.Join(declared, id, "manifest.yaml")
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(m), 0o644)
	}
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s\n  executables: %s\n", dir, run, declared, executables)), 0o644)

	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core)
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
	await("readiness", func(l string) bool { return strings.Contains(l, "msg=ready") })
	return run, await
}

// dial is a client of one of the Core's sockets.
func dial(t *testing.T, path string) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// std: yoke:reaching-the-surface.07
func TestThroughTheCoreAShellConnectionOpensAsItsAccount(t *testing.T) {
	run, await := started(t)
	for _, name := range []string{"operator.sock", "shell.sock"} {
		info, err := os.Stat(filepath.Join(run, name))
		if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o660 {
			t.Errorf("%s is %v (%v), want a socket of mode 0660", name, info, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := administrativev1.NewShellClient(dial(t, filepath.Join(run, "shell.sock"))).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv()
	if err != nil {
		t.Fatalf("no opening: %v", err)
	}
	opening := frame.GetOpening()
	if person := opening.GetActor().GetPerson(); person != me(t).Username {
		t.Errorf("the opening names %q, want %q", person, me(t).Username)
	}
	about := "subject=connection:" + opening.GetConnection()
	await("connection.opened", func(l string) bool {
		return strings.Contains(l, "type=connection.opened") && strings.Contains(l, about) && strings.Contains(l, "actor=operator")
	})
	stream.CloseSend()
	await("connection.closed", func(l string) bool {
		return strings.Contains(l, "type=connection.closed") && strings.Contains(l, about) && strings.Contains(l, "actor=operator")
	})
}

// std: yoke:the-two-projections.06
func TestThroughTheCoreBothProjectionsAnswerAlike(t *testing.T) {
	run, _ := started(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	operator := administrativev1.NewOperatorClient(dial(t, filepath.Join(run, "operator.sock")))
	for name, r := range map[string]*administrativev1.Request{"no operation": nothing, "subscribe": subscribe} {
		_, err := operator.Call(ctx, r)
		if got := refusalOf(err); got.GetCode() != "operation.malformed" {
			t.Errorf("Call of %s was answered %v, want operation.malformed", name, err)
		}
	}
	stream, err := administrativev1.NewShellClient(dial(t, filepath.Join(run, "shell.sock"))).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next(t, stream)
	issue(t, stream, "x", nothing)
	issue(t, stream, "y", nothing)
	calls := map[string]bool{}
	for range 2 {
		frame := next(t, stream)
		if frame.GetRefusal().GetCode() != "operation.malformed" {
			t.Errorf("the shell answered %v, want a refusal operation.malformed", frame)
		}
		calls[frame.GetCall()] = true
	}
	if !calls["x"] || !calls["y"] {
		t.Errorf("the refusals carried %v, want x and y", calls)
	}
}

// std: yoke:the-operations.10
func TestThroughTheCoreAnOperatorDisablesAPlugin(t *testing.T) {
	run, await := started(t, "manifest: 1\nid: com.example.station\nprotocol: 1\nstreams: [ { id: station.data } ]\n"+
		"capabilities: [ { name: stream.data.publish, governs: { stream: station.data } } ]\n")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := administrativev1.NewOperatorClient(dial(t, filepath.Join(run, "operator.sock"))).Call(ctx, disable(station))
	if err != nil {
		t.Fatalf("the disable by Call failed: %v", err)
	}
	if !resp.GetPluginDisable().GetPreviously().GetEnabled() {
		t.Errorf("the disable by Call answered %v, want that it was enabled", resp)
	}
	stream, err := administrativev1.NewShellClient(dial(t, filepath.Join(run, "shell.sock"))).Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	next(t, stream)
	issue(t, stream, "again", disable(station))
	if frame := next(t, stream); frame.GetCall() != "again" || frame.GetAnswer() == nil || frame.GetAnswer().GetPluginDisable().GetPreviously().GetEnabled() {
		t.Errorf("the disable on the shell answered %v, want that it was disabled", frame)
	}
	await("plugin.policy.changed", func(l string) bool {
		return strings.Contains(l, "type=plugin.policy.changed") && strings.Contains(l, "subject=plugin:"+station) &&
			strings.Contains(l, "actor=operator") && strings.Contains(l, "person="+me(t).Username)
	})
}

// std: yoke:reads-and-the-log.08
func TestThroughTheCoreTheInstanceAPluginAndTheLogAreRead(t *testing.T) {
	run, _ := started(t, "manifest: 1\nid: com.example.station\nprotocol: 1\nstreams: [ { id: station.data } ]\n"+
		"capabilities: [ { name: stream.data.publish, governs: { stream: station.data } } ]\n")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	operator := administrativev1.NewOperatorClient(dial(t, filepath.Join(run, "operator.sock")))
	resp, err := operator.Call(ctx, readOf("instance", ""))
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetRead().GetRecords(); len(got) != 1 || !got[0].GetInstance().GetReady() || got[0].GetInstance().GetForm() != "service" {
		t.Errorf("the instance read %v", got)
	}
	resp, err = operator.Call(ctx, readOf("plugin", station))
	if err != nil {
		t.Fatal(err)
	}
	if got := resp.GetRead().GetRecords(); len(got) != 1 || !got[0].GetPlugin().GetObserved().GetManifestPresent() || !got[0].GetPlugin().GetAuthorized().GetEnabled() {
		t.Errorf("the plugin read %v", got)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err = operator.Call(ctx, query(&administrativev1.LogQuery{}))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, e := range resp.GetLogQuery().GetEntries() {
			found = found || e.GetType() == "instance.ready"
		}
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the log holds no instance.ready: %v", resp.GetLogQuery().GetEntries())
		}
		time.Sleep(100 * time.Millisecond)
	}
}
