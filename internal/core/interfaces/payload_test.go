package interfaces_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
)

// std: yoke:a-command-payload.01
func TestThroughTheCoreACommandsPayloadIsWrittenNowhere(t *testing.T) {
	dir := t.TempDir()
	run, lines := startCoreIn(t, dir, "log:\n  level: debug\n", `units:
  acquire: { kind: plugin, plugin: com.example.station, env: { %[2]s: commanded } }
channels:
  panel: { transport: local, clients: single }
# %[1]s
`)
	var mu sync.Mutex
	var said []string
	opened := make(chan struct{}, 4)
	go func() {
		for line := range lines {
			mu.Lock()
			said = append(said, line)
			mu.Unlock()
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
	call := func(r *administrativev1.Request) *administrativev1.Response {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		answer, err := operator.Call(ctx, r)
		if err != nil {
			t.Fatalf("%v was refused: %v", r, err)
		}
		return answer
	}
	call(&administrativev1.Request{Version: 1, Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: "com.example.station", Capability: "command.calibrate.accept"}}})
	call(&administrativev1.Request{Version: 1, Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: "acquire"}}})
	await("next life's Session")

	// Three secrets, one for each road a payload may take: a channel's command carried, a channel's
	// command refused before any Session sees it, and a command given on the administrative surface.
	secret := func() []byte {
		b := make([]byte, 16)
		rand.Read(b)
		return []byte("passphrase-" + hex.EncodeToString(b))
	}
	carried, refused, administrative := secret(), secret(), secret()

	a, err := attachTo(t, filepath.Join(run, "interfaces", "panel.sock"))
	if err != nil {
		t.Fatal(err)
	}
	a.next(t)
	if f := a.answerTo(t, "c1", command("acquire", "calibrate", carried)); f.GetAnswer().GetCommand().GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE {
		t.Fatalf("the carried command was answered %v", f)
	}
	if f := a.answerTo(t, "c2", command("acquire", "zero", refused)); f.GetRefusal() == nil {
		t.Fatalf("the command of a type nobody declared was answered %v", f)
	}
	answer := call(&administrativev1.Request{Version: 1, Operation: &administrativev1.Request_UnitCommand{UnitCommand: &administrativev1.UnitCommand{Unit: "acquire", Type: "calibrate", Payload: administrative}}})
	if answer.GetUnitCommand().GetOutcome() != administrativev1.Commanded_OUTCOME_DONE {
		t.Fatalf("the administrative command was answered %v", answer)
	}

	// The edge the administrative command leaves has reached the log store before it is read: the
	// events read back say so, and the files under the state directory are then what holds it.
	issued := false
	for deadline := time.Now().Add(10 * time.Second); !issued && time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		issued = stateHolds(t, filepath.Join(dir, "state"), []byte("unit.command.issued"))
	}
	if !issued {
		t.Fatal("no unit.command.issued under the state directory within ten seconds: the log store holds no event to look in")
	}

	// The log store writes in the order it is handed lines, and every line the three commands could
	// have left was handed before that edge; a moment more lets the Core's output catch up with it.
	time.Sleep(time.Second)
	mu.Lock()
	output := []byte(strings.Join(said, "\n"))
	mu.Unlock()
	for name, s := range map[string][]byte{"carried": carried, "refused": refused, "administrative": administrative} {
		if bytes.Contains(output, s) {
			t.Errorf("the Core's output carries the %s command's payload", name)
		}
		if stateHolds(t, filepath.Join(dir, "state"), s) {
			t.Errorf("a file under the state directory holds the %s command's payload", name)
		}
	}
}

// stateHolds says whether any file under the directory holds the bytes, in any part of it.
func stateHolds(t *testing.T, root string, needle []byte) bool {
	t.Helper()
	found := false
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found {
			return nil
		}
		content, err := os.ReadFile(path)
		if err == nil && bytes.Contains(content, needle) {
			found = true
		}
		return nil
	})
	return found
}
