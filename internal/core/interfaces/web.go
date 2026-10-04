package interfaces

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/encoding/protojson"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/peer"
	"github.com/yoke-project/yoke/internal/core/streams"
)

// Cookie is the attachment's cookie, which the browser sends with every control request and the page
// cannot read.
const Cookie = "yoke-attachment"

// web is what the browser projection holds: the attachments open on /v1/events, by their cookie, and the
// deliveries waiting at their paths.
type web struct {
	mu          sync.Mutex
	attachments map[string]*attachment
	deliveries  map[string]*webDelivery
}

// webDelivery is a delivery whose data travels on a binary WebSocket at its path.
type webDelivery struct {
	frames chan streams.Frame
	done   chan struct{}
	owner  *attachment
}

var upgrader = websocket.Upgrader{}

// httpStatus is the transport's coarse hint for a refusal; the code in the body is authoritative.
func httpStatus(code string) int {
	switch {
	case code == "subject.unknown":
		return http.StatusNotFound
	case strings.HasPrefix(code, "operation.") || code == "compat.unsupported":
		return http.StatusBadRequest
	case strings.HasPrefix(code, "auth."):
		return http.StatusUnauthorized
	case strings.HasPrefix(code, "scope."):
		return http.StatusForbidden
	case code == "instance.stopping":
		return http.StatusServiceUnavailable
	}
	return http.StatusConflict
}

func writeRefusal(w http.ResponseWriter, r *interfacev1.Refusal) {
	body, _ := protojson.Marshal(r)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus(r.GetCode()))
	w.Write(body)
}

// ServeHTTP is the browser projection: /v1/events attaches, POST /v1/<operation> is the control shape, a
// stream's delivery is a WebSocket at /v1/streams/<unit>/<stream>/<n>, and anything else is 404.
func (s *Surface) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/v1/events" && r.Method == http.MethodGet:
		s.events(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/streams/") && r.Method == http.MethodGet:
		s.streamSocket(w, r)
	case strings.HasPrefix(r.URL.Path, "/v1/") && r.Method == http.MethodPost && !strings.Contains(strings.TrimPrefix(r.URL.Path, "/v1/"), "/"):
		s.control(w, r)
	default:
		http.NotFound(w, r)
	}
}

// attached is the attachment a request's cookie names, while it lives.
func (s *Surface) attached(r *http.Request) *attachment {
	c, err := r.Cookie(Cookie)
	if err != nil {
		return nil
	}
	s.web.mu.Lock()
	defer s.web.mu.Unlock()
	return s.web.attachments[c.Value]
}

// events attaches: the WebSocket is the live shape, carrying the opening first and then the frames in
// JSON, and the attachment lives as long as it does.
func (s *Surface) events(w http.ResponseWriter, r *http.Request) {
	client, established := peer.Account(r.Context(), s.cfg.Accounts)
	if !established {
		client = "unestablished"
	}
	id := make([]byte, 16)
	rand.Read(id)
	cookie := hex.EncodeToString(id)
	// What the attachment tells waits for the upgrade, so nothing is written before the WebSocket exists.
	var conn *websocket.Conn
	ready := make(chan struct{})
	a, ref := s.begin(r.Context(), client, func(f *interfacev1.CoreFrame) error {
		<-ready
		if conn == nil {
			return io.ErrClosedPipe
		}
		data, err := protojson.Marshal(f)
		if err != nil {
			return err
		}
		return conn.WriteMessage(websocket.TextMessage, data)
	})
	if ref != nil {
		close(ready)
		writeRefusal(w, ref)
		return
	}
	header := http.Header{}
	header.Add("Set-Cookie", (&http.Cookie{Name: Cookie, Value: cookie, Path: "/v1", HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil}).String())
	upgraded, err := upgrader.Upgrade(w, r, header)
	if err != nil {
		close(ready)
		a.finish("lost")
		return
	}
	conn = upgraded
	close(ready)
	defer conn.Close()
	s.web.mu.Lock()
	s.web.attachments[cookie] = a
	s.web.mu.Unlock()
	reason := "lost"
	defer func() {
		s.web.mu.Lock()
		delete(s.web.attachments, cookie)
		s.web.mu.Unlock()
		a.finish(reason)
	}()
	if a.live(&interfacev1.CoreFrame{Carries: &interfacev1.CoreFrame_Opening{Opening: a.opening}}) != nil {
		return
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				reason = "closed"
			}
			return
		}
	}
}

// control is one request, answered once: the operation is the path, and a control request with no live
// attachment is refused, since the channel is held by the connection that can be told things.
func (s *Surface) control(w http.ResponseWriter, r *http.Request) {
	a := s.attached(r)
	if a == nil {
		writeRefusal(w, refusal("channel.not_attached", "a control request is made from a live attachment, and none is open"))
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2*Payload))
	if err != nil {
		writeRefusal(w, refusal("operation.malformed", err.Error()))
		return
	}
	req := &interfacev1.Request{}
	if err := protojson.Unmarshal(body, req); err != nil {
		writeRefusal(w, refusal("operation.malformed", "the request does not decode: "+err.Error()))
		return
	}
	if named := strings.TrimPrefix(r.URL.Path, "/v1/"); Name(req) != named {
		writeRefusal(w, refusal("operation.malformed", "the path names "+named+" and the request "+Name(req)))
		return
	}
	id := make([]byte, 8)
	rand.Read(id)
	call := "http-" + hex.EncodeToString(id)
	answered := make(chan *interfacev1.CoreFrame, 1)
	a.handle(call, req, func(f *interfacev1.CoreFrame) error {
		answered <- f
		return nil
	})
	f := <-answered
	if ref := f.GetRefusal(); ref != nil {
		writeRefusal(w, ref)
		return
	}
	data, _ := protojson.Marshal(f.GetAnswer())
	w.Header().Set("Content-Type", "application/json")
	// A stream's later answers travel on the live shape under this call.
	w.Header().Set("Yoke-Call", call)
	w.Write(data)
}

// streamSocket is a delivery's binary WebSocket, at the path the subscription was answered with.
func (s *Surface) streamSocket(w http.ResponseWriter, r *http.Request) {
	a := s.attached(r)
	s.web.mu.Lock()
	d := s.web.deliveries[r.URL.Path]
	s.web.mu.Unlock()
	if a == nil || d == nil || d.owner != a {
		http.NotFound(w, r)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	for {
		select {
		case <-d.done:
			conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "released"))
			return
		case f := <-d.frames:
			if conn.WriteMessage(websocket.BinaryMessage, frameOf(f)) != nil {
				return
			}
		}
	}
}
