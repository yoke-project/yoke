package interfaces_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/encoding/protojson"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/hostauth"
)

// host accepts alice with hunter2x, refuses any other, and cannot be asked about carol.
func host(_ context.Context, account, secret string) (hostauth.Verdict, error) {
	switch {
	case account == "carol":
		return hostauth.Unasked, io.ErrUnexpectedEOF
	case account == "alice" && secret == "hunter2x":
		return hostauth.Accepted, nil
	}
	return hostauth.Refused, nil
}

func password(account, secret string) *interfacev1.Request {
	return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Authenticate{Authenticate: &interfacev1.Authenticate{
		Credential: &interfacev1.Authenticate_Password_{Password: &interfacev1.Authenticate_Password{Account: account, Secret: secret}}}}}
}

func authenticating(t *testing.T, transport string) (*interfaces.Bound, *[]event.Event, *sync.Mutex) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tls")
	material(t, dir, "panel")
	var mu sync.Mutex
	var published []event.Event
	b, err := interfaces.BindServing(root(t), dir, mode, []gate.Channel{routable("panel", transport, t)}, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{Channel: ch, Authenticate: host,
			Publish: func(e event.Event) { mu.Lock(); published = append(published, e); mu.Unlock() }})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b, &published, &mu
}

// std: yoke:attaching.08
func TestTheTypedProjectionsFirstFrameAuthenticates(t *testing.T) {
	b, published, mu := authenticating(t, "local")
	conn, err := grpc.NewClient(b.Address("panel"), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	open := func(first *interfacev1.Request) (interfacev1.Interface_AttachClient, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		t.Cleanup(cancel)
		stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
		if err != nil {
			return nil, err
		}
		if err := stream.Send(&interfacev1.ClientFrame{Call: "a", Carries: &interfacev1.ClientFrame_Request{Request: first}}); err != nil {
			return nil, err
		}
		_, err = stream.Recv()
		return stream, err
	}
	stream, err := open(password("alice", "hunter2x"))
	if err != nil {
		t.Fatalf("alice: %v", err)
	}
	if f, err := stream.Recv(); err != nil || f.GetCall() != "a" || f.GetAnswer().GetAuthenticate().GetAccount() != "alice" {
		t.Errorf("after the opening, the call was answered %v %v", f, err)
	}
	stream.CloseSend()
	for first, want := range map[*interfacev1.Request]string{
		password("alice", "wrong"):    "auth.invalid",
		password("carol", "hunter2x"): "auth.invalid",
		{Version: 1, Operation: &interfacev1.Request_Read{Read: &interfacev1.Read{}}}: "auth.required",
	} {
		_, err := open(first)
		ref := refusalIn(err)
		if ref.GetCode() != want || strings.Contains(ref.GetMessage(), "carol") {
			t.Errorf("%v was answered %v", first, err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	attached := 0
	for _, e := range *published {
		if e.Type == "channel.attached" {
			attached++
		}
	}
	if attached != 1 {
		t.Errorf("published %v", *published)
	}
}

// std: yoke:the-browser-projection.07
func TestTheBrowserAuthenticatesWithNoAttachment(t *testing.T) {
	b, _, _ := authenticating(t, "http+ws")
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	post := func(op string, req *interfacev1.Request) (int, string) {
		body, _ := protojson.Marshal(req)
		resp, err := client.Post("https://"+b.Address("panel")+"/v1/"+op, "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		said, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(said)
	}
	if code, said := post("authenticate", password("alice", "hunter2x")); code != http.StatusOK || !strings.Contains(said, `"alice"`) {
		t.Errorf("alice: %d %s", code, said)
	}
	if code, said := post("authenticate", password("alice", "wrong")); code != http.StatusUnauthorized || !strings.Contains(said, "auth.invalid") {
		t.Errorf("a wrong secret: %d %s", code, said)
	}
	if code, said := post("read", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Read{Read: &interfacev1.Read{}}}); !strings.Contains(said, "auth.required") {
		t.Errorf("another operation: %d %s", code, said)
	}
}
