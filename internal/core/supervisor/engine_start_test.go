package supervisor_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// rootOwned serves, on a socket of the test's, an engine that answers as a daemon running as root and
// holds nothing, and returns the socket's path.
func rootOwned(t *testing.T) string {
	t.Helper()
	dir, _ := os.MkdirTemp("", "eng-")
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "docker.sock")
	l, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	answer := func(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }
	s := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/version"):
			answer(w, map[string]any{"Version": "28.5.2", "ApiVersion": "1.51", "MinAPIVersion": "1.40",
				"Components": []map[string]any{{"Name": "Engine"}}})
		case strings.HasSuffix(path, "/info"):
			answer(w, map[string]any{"SecurityOptions": []string{"name=seccomp,profile=builtin"}})
		case strings.HasSuffix(path, "/containers/json"):
			answer(w, []any{})
		case strings.HasSuffix(path, "/events"):
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	})}
	go s.Serve(l)
	t.Cleanup(func() { s.Close() })
	return socket
}

// std: yoke:the-engine-at-the-start.01
func TestAnEngineTheGateCannotReachIsTheRefusalStep7FailsOn(t *testing.T) {
	nothing := filepath.Join(t.TempDir(), "engine.sock")
	image := "localhost/yoke-l3-absent@sha256:" + strings.Repeat("ab", 32)
	core, lines := deployed(t, nothing, "units:\n  announcer:\n    kind: oneshot\n    image: "+image+"\n").start(t)
	r := &reader{t: t, lines: lines}
	said := strings.Join(r.ended(30*time.Second), "\n")
	var exit *exec.ExitError
	if err := core.Wait(); !errors.As(err, &exit) || exit.ExitCode() == 0 {
		t.Errorf("the Core ended with %v, want a failure", err)
	}
	for _, want := range []string{"step inherited runtime facts", "engine.unreachable", nothing} {
		if !strings.Contains(said, want) {
			t.Errorf("the failure does not name %s:\n%s", want, said)
		}
	}
	if strings.Contains(said, "type=instance.ready") {
		t.Errorf("the Core became ready:\n%s", said)
	}
}

// std: yoke:the-engine-at-the-start.02
func TestARootOwnedEngineIsReportedAndRepeatedWhenTheInstanceIsRead(t *testing.T) {
	image := "localhost/yoke-l3-absent@sha256:" + strings.Repeat("ab", 32)
	d := deployed(t, rootOwned(t), "units:\n  announcer:\n    kind: oneshot\n    image: "+image+"\n")
	core, lines := d.start(t)
	r := &reader{t: t, lines: lines}
	r.until("the finding", 20*time.Second, containing("level=WARN", "code=engine.rootful"))
	r.until("readiness", 20*time.Second, containing("msg=ready"))
	run := ""
	for _, line := range r.said {
		if _, after, ok := strings.Cut(line, "msg=ready root="); ok {
			run = strings.Fields(after)[0]
		}
	}
	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := administrativev1.NewOperatorClient(conn).Call(ctx, &administrativev1.Request{Version: 1,
		Operation: &administrativev1.Request_Read{Read: &administrativev1.Read{Kind: "instance"}}})
	if err != nil {
		t.Fatal(err)
	}
	records := resp.GetRead().GetRecords()
	if len(records) != 1 || !slices.ContainsFunc(records[0].GetInstance().GetWeaker(), func(w string) bool { return strings.HasPrefix(w, "engine.rootful") }) {
		t.Errorf("the instance read %v", records)
	}
	core.Process.Signal(syscall.SIGTERM)
	r.rest()
	core.Wait()
}
