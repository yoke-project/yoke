package admin_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/reflect/protoreflect"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"

	"github.com/yoke-project/yoke/internal/core/admin"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/trunk"
)

// published is what the surface concluded, in order.
type published struct {
	mu     sync.Mutex
	events []event.Event
}

func (p *published) publish(e event.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, e)
}

func (p *published) of(typ string) []event.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []event.Event
	for _, e := range p.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// served serves both projections of a surface made with cfg on sockets in a directory of the test's, and
// returns a client of each.
func served(t *testing.T, cfg admin.Config) (*admin.Surface, administrativev1.OperatorClient, administrativev1.ShellClient) {
	t.Helper()
	dir, _ := os.MkdirTemp("", "yka")
	t.Cleanup(func() { os.RemoveAll(dir) })
	s := admin.New(cfg)
	clients := map[string]*grpc.ClientConn{}
	for name, server := range map[string]*grpc.Server{"operator.sock": s.Operator(), "shell.sock": s.Shell()} {
		path := filepath.Join(dir, name)
		l, err := net.Listen("unix", path)
		if err != nil {
			t.Fatal(err)
		}
		go server.Serve(l)
		t.Cleanup(server.Stop)
		conn, err := grpc.NewClient("unix://"+path, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		clients[name] = conn
	}
	return s, administrativev1.NewOperatorClient(clients["operator.sock"]), administrativev1.NewShellClient(clients["shell.sock"])
}

// opened opens a shell connection and reads its opening.
func opened(t *testing.T, shell administrativev1.ShellClient) (administrativev1.Shell_ConnectClient, *administrativev1.Opening, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := shell.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := stream.Recv()
	if err != nil {
		t.Fatalf("no opening: %v", err)
	}
	if frame.GetOpening() == nil {
		t.Fatalf("the first frame is %v, want an opening", frame)
	}
	return stream, frame.GetOpening(), cancel
}

func me(t *testing.T) *user.User {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// std: yoke:reaching-the-surface.01
func TestBothSocketsAreBoundWithTheFormsMode(t *testing.T) {
	for form, want := range map[trunk.Form]os.FileMode{trunk.Service: 0o660, trunk.Application: 0o600} {
		dir := t.TempDir()
		env := map[string]string{"XDG_RUNTIME_DIR": dir + "/run", "XDG_STATE_HOME": dir + "/state"}
		if form == trunk.Service {
			path := filepath.Join(dir, "core.yaml")
			os.WriteFile(path, []byte("state_dir: "+dir+"/state\nruntime_dir: "+dir+"/run\n"), 0o644)
			env = map[string]string{"YOKE_CONFIG": path}
		}
		st := &trunk.State{Form: form, Name: "bench-a", Env: func(k string) string { return env[k] }, Stderr: &lockedBuffer{}}
		if err := trunk.Run(st, trunk.Steps()); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"operator.sock", "shell.sock"} {
			info, err := os.Stat(filepath.Join(st.Paths.Root, name))
			if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != want {
				t.Errorf("form %d: %s is %v (%v), want a socket of mode %v", form, name, info, err, want)
			}
		}
		st.Stop()
	}
}

// std: yoke:reaching-the-surface.02
func TestTheActorIsTheAccountTheKernelNames(t *testing.T) {
	u := me(t)
	_, _, shell := served(t, admin.Config{Publish: func(event.Event) {}})
	_, opening, _ := opened(t, shell)
	if got := opening.GetActor(); got.GetClass() != "operator" || got.GetPerson() != u.Username {
		t.Errorf("the opening names %v, want the operator %s", got, u.Username)
	}

	nobody := func(string) (string, error) { return "", errors.New("no such account") }
	_, _, shell = served(t, admin.Config{Publish: func(event.Event) {}, Accounts: nobody})
	_, opening, _ = opened(t, shell)
	if got := opening.GetActor(); got.GetClass() != "operator" || got.GetPerson() != "uid:"+u.Uid {
		t.Errorf("with no name the opening names %v, want the operator uid:%s", got, u.Uid)
	}
}

// std: yoke:reaching-the-surface.03
func TestOncePerConnectionOnTheShellOncePerCallOnTheOperator(t *testing.T) {
	var mu sync.Mutex
	asked := 0
	counting := func(uid string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		asked++
		return "someone", nil
	}
	count := func() int { mu.Lock(); defer mu.Unlock(); return asked }
	_, operator, shell := served(t, admin.Config{Publish: func(event.Event) {}, Accounts: counting})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for range 2 {
		operator.Call(ctx, &administrativev1.Request{Version: 1})
	}
	if n := count(); n != 2 {
		t.Errorf("two calls asked the account database %d times, want 2", n)
	}
	stream, _, _ := opened(t, shell)
	for i := range 2 {
		stream.Send(&administrativev1.ClientFrame{Call: fmt.Sprint(i), Carries: &administrativev1.ClientFrame_Request{Request: &administrativev1.Request{Version: 1}}})
		stream.Recv()
	}
	if n := count(); n != 3 {
		t.Errorf("one shell connection asked the account database %d times, want 1", n-2)
	}
}

// std: yoke:reaching-the-surface.04
func TestNoRequestCarriesAnActor(t *testing.T) {
	actor := (&administrativev1.Actor{}).ProtoReflect().Descriptor().FullName()
	seen := map[protoreflect.FullName]bool{}
	var walk func(m protoreflect.MessageDescriptor)
	walk = func(m protoreflect.MessageDescriptor) {
		if seen[m.FullName()] {
			return
		}
		seen[m.FullName()] = true
		fields := m.Fields()
		for i := range fields.Len() {
			f := fields.Get(i)
			if f.Name() == "actor" {
				t.Errorf("%s has a field named actor", m.FullName())
			}
			if f.Message() != nil {
				if f.Message().FullName() == actor {
					t.Errorf("%s.%s is an actor", m.FullName(), f.Name())
				}
				walk(f.Message())
			}
		}
	}
	walk((&administrativev1.Request{}).ProtoReflect().Descriptor())
	walk((&administrativev1.ClientFrame{}).ProtoReflect().Descriptor())
	if len(seen) < 10 {
		t.Errorf("the walk reached %d messages, too few to be the whole of a request", len(seen))
	}
}

// std: yoke:reaching-the-surface.05
func TestAConnectionIsAnnouncedWhenItOpensAndWhenItCloses(t *testing.T) {
	u := me(t)
	var p published
	s, _, shell := served(t, admin.Config{Publish: p.publish})
	first, a, _ := opened(t, shell)
	_, b, _ := opened(t, shell)
	if a.GetConnection() == "" || a.GetConnection() == b.GetConnection() {
		t.Fatalf("the two connections are %q and %q, want two identities", a.GetConnection(), b.GetConnection())
	}
	listed := s.Connections()
	if len(listed) != 2 {
		t.Fatalf("the surface lists %v, want both connections", listed)
	}
	for _, c := range listed {
		if c.Projection != admin.ShellProjection || c.Actor != (event.Actor{Class: event.ByOperator, Person: u.Username}) || c.Opened.IsZero() {
			t.Errorf("a connection is listed as %+v", c)
		}
	}
	opens := p.of("connection.opened")
	if len(opens) != 2 {
		t.Fatalf("%d connection.opened were published, want 2", len(opens))
	}
	for _, e := range opens {
		var d map[string]any
		json.Unmarshal(e.Detail, &d)
		if e.Subject.Kind != event.Connection || e.Actor != (event.Actor{Class: event.ByOperator, Person: u.Username}) || d["projection"] != "shell" {
			t.Errorf("connection.opened is %+v with %v", e, d)
		}
	}

	first.CloseSend()
	deadline := time.Now().Add(3 * time.Second)
	for len(s.Connections()) != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if listed := s.Connections(); len(listed) != 1 || listed[0].ID != b.GetConnection() {
		t.Errorf("after the first closed the surface lists %v, want only %s", listed, b.GetConnection())
	}
	closes := p.of("connection.closed")
	if len(closes) != 1 {
		t.Fatalf("%d connection.closed were published, want 1", len(closes))
	}
	var d map[string]any
	json.Unmarshal(closes[0].Detail, &d)
	if closes[0].Subject.ID != a.GetConnection() || d["reason"] != "cancelled" || d["projection"] != "shell" {
		t.Errorf("connection.closed is %+v with %v, want the first, cancelled", closes[0], d)
	}
}

// std: yoke:reaching-the-surface.06
func TestATransportThatStopsAnsweringIsClosed(t *testing.T) {
	l := admin.Liveness()
	if l.Time != 10*time.Second || l.Timeout != 30*time.Second {
		t.Errorf("the liveness is a probe every %v closed after %v, want 10s and 30s", l.Time, l.Timeout)
	}
}

// lockedBuffer is a log the Core may write from several goroutines while a test reads it.
type lockedBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b = append(l.b, p...)
	return len(p), nil
}
