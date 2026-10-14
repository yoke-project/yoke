package interfaces_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/encoding/protojson"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/gate"
)

// clock is the Core's clock under a case's hand.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time        { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) move(by time.Duration) { c.mu.Lock(); c.now = c.now.Add(by); c.mu.Unlock() }

// sessions binds routable channels whose host accepts alice with hunter2x, on the case's clock, and
// keeps the clients each channel.attached named.
func sessions(t *testing.T, c *clock, channels ...gate.Channel) (*interfaces.Bound, func() []string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tls")
	var names []string
	for _, ch := range channels {
		names = append(names, ch.Name)
	}
	material(t, dir, names...)
	var mu sync.Mutex
	var attached []string
	b, err := interfaces.BindServing(root(t), dir, mode, channels, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{Channel: ch, Authenticate: host, Now: c.Now,
			Publish: func(e event.Event) {
				if e.Type == "channel.attached" {
					var d map[string]any
					json.Unmarshal(e.Detail, &d)
					mu.Lock()
					attached = append(attached, d["client"].(string))
					mu.Unlock()
				}
			}})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), attached...) }
}

func token(secret string) *interfacev1.Request {
	return &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Authenticate{Authenticate: &interfacev1.Authenticate{
		Credential: &interfacev1.Authenticate_Token{Token: secret}}}}
}

// typed attaches over TLS to a local-projection channel, sending first the request given: the call's
// answer, or the refusal.
func typed(t *testing.T, address string, first *interfacev1.Request) (*interfacev1.Response, error) {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err != nil {
		return nil, err
	}
	stream.Send(&interfacev1.ClientFrame{Call: "a", Carries: &interfacev1.ClientFrame_Request{Request: first}})
	if _, err := stream.Recv(); err != nil {
		return nil, err
	}
	f, err := stream.Recv()
	return f.GetAnswer(), err
}

var trusting = &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}

// signIn posts the password to a browser-projection channel: the response, and its body.
func signIn(t *testing.T, address string) (*http.Response, string) {
	t.Helper()
	body, _ := protojson.Marshal(password("alice", "hunter2x"))
	resp, err := trusting.Post("https://"+address+"/v1/authenticate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	said, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(said)
}

// events opens /v1/events with the cookie given, if any: the first frame, or the refusal's body.
func events(t *testing.T, address string, cookie *http.Cookie) (string, error) {
	t.Helper()
	header := http.Header{}
	if cookie != nil {
		header.Set("Cookie", cookie.String())
	}
	dialer := websocket.Dialer{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, HandshakeTimeout: 5 * time.Second}
	conn, resp, err := dialer.Dial("wss://"+address+"/v1/events", header)
	if err != nil {
		if resp != nil {
			said, _ := io.ReadAll(resp.Body)
			return string(said), err
		}
		return "", err
	}
	defer conn.Close()
	_, frame, err := conn.ReadMessage()
	return string(frame), err
}

// std: yoke:the-session-token.01
func TestAnAcceptedPasswordIsAnsweredWithAToken(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	b, _ := sessions(t, c, routable("panel", "local", t), routable("service", "http+ws", t))
	answer, err := typed(t, b.Address("panel"), password("alice", "hunter2x"))
	if err != nil {
		t.Fatal(err)
	}
	got := answer.GetAuthenticate()
	raw, decoded := base64.RawURLEncoding.DecodeString(got.GetToken())
	if got.GetAccount() != "alice" || decoded != nil || len(raw) != 32 || !got.GetExpires().AsTime().Equal(c.Now().Add(12*time.Hour)) {
		t.Errorf("the typed projection answered %v", got)
	}
	resp, body := signIn(t, b.Address("service"))
	var session *http.Cookie
	for _, k := range resp.Cookies() {
		if k.Name == "yoke-session" {
			session = k
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || !session.Secure || session.Path != "/v1" || session.Value == "" {
		t.Errorf("the cookie is %+v", session)
	}
	if !strings.Contains(body, `"alice"`) || !strings.Contains(body, "expires") || session != nil && strings.Contains(body, session.Value) || strings.Contains(body, `"token"`) {
		t.Errorf("the body is %s", body)
	}
}

// std: yoke:the-session-token.02
func TestATokenAttachesAsTheAccountItStandsFor(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	b, attached := sessions(t, c, routable("panel", "local", t), routable("service", "http+ws", t))
	answer, err := typed(t, b.Address("panel"), password("alice", "hunter2x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := typed(t, b.Address("panel"), token(answer.GetAuthenticate().GetToken())); err != nil {
		t.Errorf("the typed projection's token: %v", err)
	}
	resp, _ := signIn(t, b.Address("service"))
	var session *http.Cookie
	for _, k := range resp.Cookies() {
		if k.Name == "yoke-session" {
			session = k
		}
	}
	if frame, err := events(t, b.Address("service"), session); err != nil || !strings.Contains(frame, "opening") {
		t.Errorf("the browser's cookie: %v %s", err, frame)
	}
	if said, _ := events(t, b.Address("service"), nil); !strings.Contains(said, "auth.required") {
		t.Errorf("no cookie: %s", said)
	}
	if said, _ := events(t, b.Address("service"), &http.Cookie{Name: "yoke-session", Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))}); !strings.Contains(said, "auth.invalid") {
		t.Errorf("a token never issued: %s", said)
	}
	time.Sleep(100 * time.Millisecond)
	if got := attached(); len(got) != 3 || got[0] != "alice" || got[1] != "alice" || got[2] != "alice" {
		t.Errorf("channel.attached named %v", got)
	}
}

// std: yoke:the-session-token.03
func TestATokenIsBoundToTheChannelThatIssuedIt(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	b, _ := sessions(t, c, routable("panel", "local", t), routable("bench", "local", t))
	answer, err := typed(t, b.Address("panel"), password("alice", "hunter2x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := typed(t, b.Address("bench"), token(answer.GetAuthenticate().GetToken())); refusalIn(err).GetCode() != "auth.invalid" {
		t.Errorf("panel's token on bench: %v", err)
	}
}

// std: yoke:the-session-token.04
func TestATokenEndsAtTwelveHoursAndEarlier(t *testing.T) {
	c := &clock{now: time.Date(2026, 10, 14, 9, 0, 0, 0, time.UTC)}
	panel := routable("panel", "local", t)
	b, _ := sessions(t, c, panel)
	issue := func() string {
		answer, err := typed(t, b.Address("panel"), password("alice", "hunter2x"))
		if err != nil {
			t.Fatal(err)
		}
		return answer.GetAuthenticate().GetToken()
	}
	first := issue()
	c.move(11 * time.Hour)
	if _, err := typed(t, b.Address("panel"), token(first)); err != nil {
		t.Errorf("at eleven hours: %v", err)
	}
	c.move(time.Hour + time.Second)
	if _, err := typed(t, b.Address("panel"), token(first)); refusalIn(err).GetCode() != "auth.invalid" {
		t.Errorf("past twelve hours: %v", err)
	}
	second := issue()
	issue()
	if _, err := typed(t, b.Address("panel"), token(second)); refusalIn(err).GetCode() != "auth.invalid" {
		t.Errorf("after a fresh authentication: %v", err)
	}
	last := issue()
	b.Close()
	again, _ := sessions(t, c, gate.Channel{Name: panel.Name, Transport: panel.Transport, Clients: panel.Clients,
		Address: &gate.Address{Class: "routable", Host: "127.0.0.1", Port: panel.Address.Port, Transport: "encrypted", Caller: "authenticated"}})
	if _, err := typed(t, again.Address("panel"), token(last)); refusalIn(err).GetCode() != "auth.invalid" {
		t.Errorf("after the channel was bound again: %v", err)
	}
}
