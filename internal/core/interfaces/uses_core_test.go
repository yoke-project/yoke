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
)

// std: yoke:nothing-a-use-carries.01
func TestThroughTheCoreAUnitAsksAnotherAndNothingCarriedIsKept(t *testing.T) {
	b := make([]byte, 16)
	rand.Read(b)
	secret := "token-" + hex.EncodeToString(b)
	dir := t.TempDir()
	run, lines := startCoreIn(t, dir, "log:\n  level: debug\n", `units:
  source: { kind: plugin, plugin: com.example.source, env: { %[2]s: answerer, SECRET: `+secret+` } }
  archive: { kind: plugin, plugin: com.example.archive, env: { %[2]s: relayer }, bind: { source: source }, args: [ "${bind.source}" ] }
# %[1]s
`)
	var mu sync.Mutex
	var said []string
	opened := make(chan struct{}, 8)
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
	await := func(what string, n int) {
		t.Helper()
		for i := 0; i < n; i++ {
			select {
			case <-opened:
			case <-time.After(30 * time.Second):
				mu.Lock()
				defer mu.Unlock()
				t.Fatalf("no %s within thirty seconds:\n%s", what, strings.Join(said, "\n"))
			}
		}
	}
	await("first Sessions", 2)
	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	operator := administrativev1.NewOperatorClient(conn)
	call := func(r *administrativev1.Request) *administrativev1.Response {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		answer, err := operator.Call(ctx, r)
		if err != nil {
			t.Fatalf("%v was refused: %v", r, err)
		}
		return answer
	}
	grant := func(plugin, capability string) {
		call(&administrativev1.Request{Version: 1, Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: plugin, Capability: capability}}})
	}
	grant("com.example.source", "query.head-status.answer")
	grant("com.example.archive", "query.relay.answer")
	grant("com.example.archive", "query.head-status.ask")
	for _, u := range []string{"source", "archive"} {
		call(&administrativev1.Request{Version: 1, Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: u}}})
	}
	await("next lives' Sessions", 2)
	answer := call(&administrativev1.Request{Version: 1, Operation: &administrativev1.Request_UnitAsk{UnitAsk: &administrativev1.UnitAsk{Unit: "archive", Type: "relay"}}})
	fields := strings.Fields(string(answer.GetUnitAsk().GetAnswer()))
	if len(fields) != 3 || fields[0] != secret || fields[1] != "unit:archive" || len(fields[2]) != 32 {
		t.Fatalf("archive answered %q", answer.GetUnitAsk().GetAnswer())
	}
	state := filepath.Join(dir, "state")
	found := func(needle []byte) bool {
		hit := false
		filepath.WalkDir(state, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if data, err := os.ReadFile(path); err == nil && bytes.Contains(data, needle) {
					hit = true
				}
			}
			return nil
		})
		return hit
	}
	for deadline := time.Now().Add(20 * time.Second); !found([]byte("unit.question.chained")); {
		if time.Now().After(deadline) {
			t.Fatal("unit.question.chained was never kept")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if found([]byte(secret)) {
		t.Error("the secret is under the state directory")
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Contains(strings.Join(said, "\n"), secret) {
		t.Error("the secret is in the Core's output")
	}
}
