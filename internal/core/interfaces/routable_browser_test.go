package interfaces_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/cookiejar"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
)

// std: yoke:a-browser-across-a-routable-channel.01
func TestABrowserAcrossARoutableChannelIsDecidedByTheGrantedScope(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	certs := material(t, filepath.Join(dir, "state", "tls"), "service")
	run, lines := startCoreIn(t, dir, "", `# %[1]s
units:
  acquire: { kind: plugin, plugin: com.example.station, env: { %[2]s: commanded } }
channels:
  service: { transport: http+ws, clients: multiple, address: { class: routable, host: 127.0.0.1, port: "`+port+`", security: { transport: encrypted, caller: authenticated } } }
`)
	bound, opened := make(chan struct{}, 1), make(chan struct{}, 4)
	go func() {
		for line := range lines {
			if strings.Contains(line, "msg=channel") && strings.Contains(line, "name=service") {
				select {
				case bound <- struct{}{}:
				default:
				}
			}
			if strings.Contains(line, "msg=session") && strings.Contains(line, "event=opened") {
				opened <- struct{}{}
			}
		}
	}()
	await := func(c chan struct{}, what string) {
		t.Helper()
		select {
		case <-c:
		case <-time.After(20 * time.Second):
			t.Fatalf("no %s within twenty seconds", what)
		}
	}
	await(bound, "bound channel")
	await(opened, "first Session")

	roots := x509.NewCertPool()
	roots.AddCert(certs["service"])
	trust := &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13, NextProtos: []string{"http/1.1"}}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: trust}}
	base := "https://127.0.0.1:" + port

	resp, err := client.Get(base + "/v1/events")
	if err != nil {
		t.Fatal(err)
	}
	said, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if st := resp.TLS; st == nil || st.Version != tls.VersionTLS13 || st.NegotiatedProtocol != "http/1.1" || len(st.VerifiedChains) == 0 {
		t.Errorf("the connection was %+v", st)
	}
	if resp.StatusCode != http.StatusUnauthorized || !strings.Contains(string(said), "auth.required") {
		t.Errorf("an attachment with no cookie answered %d %s", resp.StatusCode, said)
	}

	body, _ := protojson.Marshal(password("alice", "hunter2x"))
	resp, err = client.Post(base+"/v1/authenticate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	said, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	var session *http.Cookie
	for _, k := range resp.Cookies() {
		if k.Name == "yoke-session" {
			session = k
		}
	}
	if session == nil || !session.HttpOnly || session.SameSite != http.SameSiteStrictMode || !session.Secure || strings.Contains(string(said), session.Value) {
		t.Fatalf("the sign-in answered %d, set %+v and said %s", resp.StatusCode, session, said)
	}

	dialer := websocket.Dialer{Jar: jar, TLSClientConfig: trust, HandshakeTimeout: 5 * time.Second}
	conn, _, err := dialer.Dial("wss://127.0.0.1:"+port+"/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if f := frameFrom(t, conn); f.GetOpening().GetVersion() != 1 {
		t.Errorf("the first frame is %v", f)
	}

	calibrate := func() (int, []byte) {
		t.Helper()
		body, _ := protojson.Marshal(command("acquire", "calibrate", []byte("now")))
		resp, err := client.Post(base+"/v1/command", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		said, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, said
	}
	if status, said := calibrate(); status != http.StatusForbidden || !strings.Contains(string(said), "scope.withheld") {
		t.Errorf("the command before the grant answered %d %s", status, said)
	}

	op, err := grpc.NewClient("unix://"+filepath.Join(run, "operator.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer op.Close()
	for _, r := range []*administrativev1.Request{
		{Version: 1, Operation: &administrativev1.Request_PluginGrant{PluginGrant: &administrativev1.PluginGrant{Plugin: "com.example.station", Capability: "command.calibrate.accept"}}},
		{Version: 1, Operation: &administrativev1.Request_UnitRestart{UnitRestart: &administrativev1.UnitAct{Unit: "acquire"}}},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := administrativev1.NewOperatorClient(op).Call(ctx, r)
		cancel()
		if err != nil {
			t.Fatalf("%v was refused: %v", r, err)
		}
	}
	await(opened, "next life's Session")
	status, said := calibrate()
	answer := &interfacev1.Response{}
	if status != http.StatusOK || protojson.Unmarshal(said, answer) != nil ||
		answer.GetCommand().GetOutcome() != interfacev1.Acknowledged_OUTCOME_DONE || answer.GetCommand().GetLine() != "calibrated" {
		t.Errorf("the command after the grant answered %d %s", status, said)
	}
}
