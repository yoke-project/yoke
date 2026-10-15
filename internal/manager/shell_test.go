package manager

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// projection is an instance's shell projection that opens as ada and records every call it is sent.
type projection struct {
	administrativev1.UnimplementedShellServer
	refuse       *administrativev1.Refusal
	endAfterOpen bool

	mu    sync.Mutex
	calls []*administrativev1.Request
}

func (p *projection) Connect(stream administrativev1.Shell_ConnectServer) error {
	if p.refuse != nil {
		st, _ := status.New(codes.PermissionDenied, p.refuse.GetMessage()).WithDetails(p.refuse)
		return st.Err()
	}
	stream.Send(&administrativev1.CoreFrame{Carries: &administrativev1.CoreFrame_Opening{Opening: &administrativev1.Opening{
		Connection: "s-1", Actor: &administrativev1.Actor{Class: "operator", Person: "ada"}, Subscription: "standing", Version: 1}}})
	if p.endAfterOpen {
		return nil
	}
	for {
		f, err := stream.Recv()
		if err != nil {
			return nil
		}
		if f.GetRequest() == nil {
			continue
		}
		p.mu.Lock()
		p.calls = append(p.calls, f.GetRequest())
		p.mu.Unlock()
		stream.Send(&administrativev1.CoreFrame{Call: f.GetCall(), Carries: &administrativev1.CoreFrame_Answer{Answer: &administrativev1.Response{
			Answer: &administrativev1.Response_Read{Read: &administrativev1.Records{}}}}})
	}
}

// servedAt binds p where the instance bench's shell projection is under run, and returns the environment
// that reaches it.
func servedAt(t *testing.T, p *projection) map[string]string {
	t.Helper()
	run, _ := os.MkdirTemp("", "ys")
	t.Cleanup(func() { os.RemoveAll(run) })
	os.MkdirAll(filepath.Join(run, "yoke", "bench"), 0o700)
	l, err := net.Listen("unix", filepath.Join(run, "yoke", "bench", "shell.sock"))
	if err != nil {
		t.Fatal(err)
	}
	s := grpc.NewServer()
	administrativev1.RegisterShellServer(s, p)
	go s.Serve(l)
	t.Cleanup(s.Stop)
	return map[string]string{"XDG_RUNTIME_DIR": run, "HOME": t.TempDir()}
}

// lockedBuffer is an output written by the shell while the test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// std: yoke:the-shell.01
func TestALineIsAClientVerbAndAnythingElseIsRefused(t *testing.T) {
	p := &projection{}
	vars := servedAt(t, p)
	input := strings.Join([]string{"", "unit list --instance other", "app list", "frobnicate", "unit ask bench status --input -", `unit show "two words"`, "exit", "unit list"}, "\n") + "\n"
	var out, errs bytes.Buffer
	code := shell([]string{"--instance", "bench"}, environment(vars), strings.NewReader(input), &out, &errs, nil)
	if code != Done {
		t.Fatalf("the shell exited %d:\n%s%s", code, out.String(), errs.String())
	}
	if !strings.Contains(out.String(), "ada") || !strings.Contains(out.String(), "s-1") {
		t.Errorf("the shell did not say who it is:\n%s", out.String())
	}
	said := errs.String()
	for _, w := range []string{"--instance", `"app list"`, `"frobnicate"`, "--input -"} {
		if !strings.Contains(said, w) {
			t.Errorf("no refusal names %s:\n%s", w, said)
		}
	}
	p.mu.Lock()
	calls := append([]*administrativev1.Request(nil), p.calls...)
	p.mu.Unlock()
	if len(calls) != 1 || calls[0].GetRead().GetKind() != "unit" || calls[0].GetRead().GetIdentity() != "two words" {
		t.Errorf("the projection was sent %v", calls)
	}
	if left, _ := os.ReadDir(vars["HOME"]); len(left) != 0 {
		t.Errorf("the shell wrote %v", left)
	}
}

// std: yoke:the-shell.02
func TestWhatEndsTheShellDecidesItsExitStatus(t *testing.T) {
	run := func(vars map[string]string) (int, string) {
		var out bytes.Buffer
		code := shell([]string{"--instance", "bench"}, environment(vars), strings.NewReader("unit list\n"), &out, &out, nil)
		return code, out.String()
	}
	if code, out := run(servedAt(t, &projection{})); code != Done {
		t.Errorf("an input that ended: %d\n%s", code, out)
	}
	refused := &projection{refuse: &administrativev1.Refusal{Code: "auth.required", Message: "the connection establishes nobody"}}
	if code, out := run(servedAt(t, refused)); code != Refused || !strings.Contains(out, "auth.required") && !strings.Contains(out, "establishes nobody") {
		t.Errorf("a refused connection: %d\n%s", code, out)
	}
	ended := servedAt(t, &projection{endAfterOpen: true})
	var out bytes.Buffer
	in, writer := io.Pipe()
	defer writer.Close()
	if code := shell([]string{"--instance", "bench"}, environment(ended), in, &out, &out, nil); code != Fault || !strings.Contains(out.String(), "ended") {
		t.Errorf("a connection the Core ended: %d\n%s", code, out.String())
	}
	if code, out := run(map[string]string{"XDG_RUNTIME_DIR": t.TempDir()}); code != Fault || !strings.Contains(out, "not running") {
		t.Errorf("an instance not running: %d\n%s", code, out)
	}
}

// std: yoke:the-shell.03
func TestThroughTheCoreAnswersEventsAndAFollowShareOneConnection(t *testing.T) {
	vars := startCore(t)
	in, typed := io.Pipe()
	interrupts := make(chan os.Signal, 1)
	var out lockedBuffer
	exited := make(chan int, 1)
	go func() { exited <- shell(nil, environment(vars), in, &out, &out, interrupts) }()
	waitFor := func(what string, holds func(string) bool) {
		t.Helper()
		for deadline := time.Now().Add(15 * time.Second); !holds(squash(out.String())); time.Sleep(50 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("waiting for %s, the shell said:\n%s", what, out.String())
			}
		}
	}
	typeLine := func(line string) { io.WriteString(typed, line+"\n") }
	count := func(s, sub string) int { return strings.Count(squash(s), sub) }

	waitFor("the opening", func(s string) bool { return strings.Contains(s, "connection") })
	typeLine("unit list")
	waitFor("the unit's record", func(s string) bool { return strings.Contains(s, `"Running"`) })
	typeLine("unit restart bench")
	waitFor("the restart and its events", func(s string) bool {
		return strings.Contains(s, "consequence") && strings.Contains(out.String(), "\n* ")
	})
	typeLine("log follow --unit bench")
	waitFor("the follow", func(s string) bool { return strings.Contains(s, `"unit": "bench"`) })
	interrupts <- os.Interrupt
	before := count(out.String(), `"identity": "bench"`)
	typeLine("unit show bench")
	waitFor("the unit shown after the follow", func(s string) bool { return count(s, `"identity": "bench"`) > before })
	typeLine("exit")
	select {
	case code := <-exited:
		if code != Done {
			t.Errorf("the shell exited %d:\n%s", code, out.String())
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("the shell did not exit:\n%s", out.String())
	}
	// An event is printed between answers and never inside one: no `* ` line inside an answer's braces.
	depth := 0
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.HasPrefix(l, "* ") && depth > 0 {
			t.Errorf("an event was printed inside an answer: %q", l)
		}
		if !strings.HasPrefix(l, "* ") {
			depth += strings.Count(l, "{") - strings.Count(l, "}")
		}
	}
}
