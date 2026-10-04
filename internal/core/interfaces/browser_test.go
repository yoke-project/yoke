package interfaces_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/interfaces"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// browser is a channel carrying http+ws on loopback, and a client holding its cookies as a browser does.
type browser struct {
	*delivering
	base string
	jar  *cookiejar.Jar
	http *http.Client
}

func browsing(t *testing.T) *browser {
	t.Helper()
	d := &delivering{w: newWorld(), root: root(t)}
	d.transports = streams.New(streams.Config{Root: d.root, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Publish: d.w.publish})
	t.Cleanup(func() { d.transports.CloseAll("acquire", streams.UnitExited) })
	manifest := &gate.Manifest{ID: "com.example.station", Commands: []string{"calibrate"}, Streams: []gate.Stream{{ID: "station.spectra"}}}
	ch := gate.Channel{Name: "remote", Transport: "http+ws", Clients: "multiple", Address: &gate.Address{Class: "loopback", Port: freePort(t)}}
	b, err := interfaces.BindServing(d.root, mode, []gate.Channel{ch}, func(ch gate.Channel) interfacev1.InterfaceServer {
		return interfaces.NewSurface(interfaces.Config{
			Channel: ch, Root: d.root, Bus: d.w.bus, Publish: d.w.publish, Units: bench{}, Sessions: &fakeSessions{}, Transports: d.transports,
			Declared: func(id string) (*gate.Manifest, bool) { return manifest, benchUnits[id].kind == unit.Plugin },
			Granted: func(id string) ([]string, []string, []string) {
				return []string{"station.spectra"}, []string{"calibrate"}, nil
			},
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	d.bound = b
	jar, _ := cookiejar.New(nil)
	return &browser{delivering: d, base: "http://" + b.Address("remote"), jar: jar, http: &http.Client{Jar: jar}}
}

// attach opens /v1/events, returning the WebSocket and the response to its upgrade.
func (b *browser) attach(t *testing.T) (*websocket.Conn, *http.Response) {
	t.Helper()
	dialer := websocket.Dialer{Jar: b.jar}
	conn, resp, err := dialer.Dial("ws"+strings.TrimPrefix(b.base, "http")+"/v1/events", nil)
	if err != nil {
		t.Fatalf("opening /v1/events: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, resp
}

func frameFrom(t *testing.T, conn *websocket.Conn) *interfacev1.CoreFrame {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	kind, data, err := conn.ReadMessage()
	if err != nil || kind != websocket.TextMessage {
		t.Fatalf("read %d %s %v", kind, data, err)
	}
	f := &interfacev1.CoreFrame{}
	if err := protojson.Unmarshal(data, f); err != nil {
		t.Fatalf("%s is not a frame in JSON: %v", data, err)
	}
	return f
}

// post sends a request to /v1/<operation>, returning the status and the body decoded as a response or a
// refusal.
func (b *browser) post(t *testing.T, operation string, r *interfacev1.Request) (int, *interfacev1.Response, *interfacev1.Refusal) {
	t.Helper()
	body, _ := protojson.Marshal(r)
	resp, err := b.http.Post(b.base+"/v1/"+operation, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		out := &interfacev1.Response{}
		if err := protojson.Unmarshal(data, out); err != nil {
			t.Fatalf("%s is not a response in JSON: %v", data, err)
		}
		return resp.StatusCode, out, nil
	}
	ref := &interfacev1.Refusal{}
	protojson.Unmarshal(data, ref)
	return resp.StatusCode, nil, ref
}

// std: yoke:the-browser-projection.01
func TestAttachingIsOpeningEventsWhichSetsTheCookieAndCarriesTheOpening(t *testing.T) {
	b := browsing(t)
	conn, resp := b.attach(t)
	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		cookie = c
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/v1" {
		t.Errorf("the upgrade set the cookie %+v", cookie)
	}
	if o := frameFrom(t, conn).GetOpening(); o.GetVersion() != 1 || o.GetSubscription() != interfaces.Standing {
		t.Errorf("the first message is the opening %v", o)
	}
	b.w.bus.Publish(event.StateChanged("acquire", 3, unit.Running, unit.Stopped))
	if f := frameFrom(t, conn); f.GetCall() != interfaces.Standing || f.GetEvent().GetSubject().GetIdentity() != "acquire" {
		t.Errorf("the next message is %v", f)
	}
}

// std: yoke:the-browser-projection.02
func TestTheOperationIsThePathAnsweredOnceInJSON(t *testing.T) {
	b := browsing(t)
	conn, _ := b.attach(t)
	frameFrom(t, conn)
	if status, resp, ref := b.post(t, "read", readOf("unit", "")); status != http.StatusOK || len(resp.GetRead().GetRecords()) == 0 {
		t.Errorf("a read was answered %d %v %v", status, resp, ref)
	}
	if status, _, ref := b.post(t, "read", readOf("unit", "nobody")); status != http.StatusNotFound || ref.GetCode() != "subject.unknown" {
		t.Errorf("a read of a unit nobody declared was answered %d %v", status, ref)
	}
	if status, _, ref := b.post(t, "read", &interfacev1.Request{Version: 1, Operation: &interfacev1.Request_Subscribe{Subscribe: &interfacev1.Subscribe{}}}); status != http.StatusBadRequest || ref.GetCode() != "operation.malformed" {
		t.Errorf("a request naming another operation was answered %d %v", status, ref)
	}
	if status, _, ref := b.post(t, "command", command("acquire", "dance", nil)); status != http.StatusForbidden || ref.GetCode() != "scope.undeclared" {
		t.Errorf("a command of an undeclared type was answered %d %v", status, ref)
	}
}

// std: yoke:the-browser-projection.03
func TestAControlRequestWithNoLiveAttachmentIsRefused(t *testing.T) {
	b := browsing(t)
	if status, _, ref := b.post(t, "read", readOf("unit", "")); status != http.StatusConflict || ref.GetCode() != "channel.not_attached" {
		t.Errorf("a read with no cookie was answered %d %v", status, ref)
	}
	conn, _ := b.attach(t)
	frameFrom(t, conn)
	conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	conn.Close()
	deadline := time.Now().Add(2 * time.Second)
	for len(b.w.of("channel.detached")) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if status, _, ref := b.post(t, "read", readOf("unit", "")); status != http.StatusConflict || ref.GetCode() != "channel.not_attached" {
		t.Errorf("a read with the cookie of a closed attachment was answered %d %v", status, ref)
	}
}

var streamPath = regexp.MustCompile(`^/v1/streams/acquire/station\.spectra/\d{8}$`)

// std: yoke:the-browser-projection.04
func TestAStreamsDeliveryIsOneBinaryWebSocket(t *testing.T) {
	b := browsing(t)
	conn, _ := b.attach(t)
	frameFrom(t, conn)
	status, resp, ref := b.post(t, "stream.subscribe", subscribeTo("station.spectra"))
	path := resp.GetStreamSubscribe().GetPath()
	if status != http.StatusOK || !streamPath.MatchString(path) {
		t.Fatalf("the subscription was answered %d %v %v", status, resp, ref)
	}
	dialer := websocket.Dialer{Jar: b.jar}
	data, _, err := dialer.Dial("ws"+strings.TrimPrefix(b.base, "http")+path, nil)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer data.Close()
	time.Sleep(50 * time.Millisecond)
	write := b.flow(t)
	write(1, []byte("one"))
	write(2, []byte("two"))
	for i, want := range []string{"one", "two"} {
		data.SetReadDeadline(time.Now().Add(3 * time.Second))
		kind, msg, err := data.ReadMessage()
		if err != nil || kind != websocket.BinaryMessage || len(msg) < 16 || binary.LittleEndian.Uint64(msg[0:8]) != uint64(i+1) ||
			binary.LittleEndian.Uint64(msg[8:16]) != uint64(1001+i) || string(msg[16:]) != want {
			t.Errorf("message %d is %d %x %v", i, kind, msg, err)
		}
	}
}

// std: yoke:the-browser-projection.05
func TestAnythingElseIs404(t *testing.T) {
	b := browsing(t)
	for _, c := range []struct{ method, path string }{{"GET", "/"}, {"GET", "/index.html"}, {"GET", "/v1/elsewhere"}, {"POST", "/v2/read"}} {
		req, _ := http.NewRequest(c.method, b.base+c.path, nil)
		resp, err := b.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s %s was answered %d", c.method, c.path, resp.StatusCode)
		}
	}
}
