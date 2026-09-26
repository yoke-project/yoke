// Package session is the accepted context everything after admission happens in: one bidirectional
// stream that is the Session.
//
// Its first envelope is a session OPEN carrying the identity admission issued, and anything else closes
// the stream with nothing sent; an identity is used once. Validity is a live property the Core governs:
// heartbeats within the terms admission set keep it, too many misses revoke it. An orderly close is the
// unit's and a revocation the Core's, and they stay distinguishable; a lost stream is neither, and what
// follows it comes from the heartbeat window. Every message carries four header fields and one payload,
// and a receiver validates in order, stopping at the first failure.
package session

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/unit"
)

// Admitted is what admission issued with a Session identity: whose it is, and the heartbeat terms.
type Admitted struct {
	Unit      string
	Interval  time.Duration
	Tolerance uint32
}

// Config is what the Session service reads and whom it tells.
type Config struct {
	// Lookup resolves an identity admission issued, and ok false for one it did not.
	Lookup func(id string) (Admitted, bool)
	// Observe hands the unit's lifecycle machine what the Session concluded.
	Observe func(unitID string, in unit.Input)
	Log     *slog.Logger
}

// Service serves Sessions.
type Service struct {
	pluginv1.UnimplementedSessionServer
	cfg Config

	mu   sync.Mutex
	used map[string]bool
	open map[string]*live
}

// New is the Session service over cfg.
func New(cfg Config) *Service {
	return &Service{cfg: cfg, used: map[string]bool{}, open: map[string]*live{}}
}

// live is one open Session.
type live struct {
	id    string
	terms Admitted
	out   chan *pluginv1.Envelope
	done  chan struct{}

	// Held under Service.mu.
	seen  map[string]bool // the unit's message identities
	sent  map[string]bool // the Core's, which a message may answer
	next  int
	timer *time.Timer
	ended bool
	lost  bool
}

// Open serves one stream.
func (s *Service) Open(stream pluginv1.Session_OpenServer) error {
	first, err := stream.Recv()
	if err != nil {
		return err
	}
	l, ok := s.opening(first)
	if !ok {
		// An anonymous connection whose first envelope attributes it to nothing: closed, nothing sent.
		return nil
	}
	lost := make(chan struct{})
	go func() {
		defer close(lost)
		for {
			e, err := stream.Recv()
			if err != nil {
				return
			}
			s.receive(l, e)
		}
	}()
	for {
		select {
		case e := <-l.out:
			stream.Send(e)
		case <-l.done:
			for {
				select {
				case e := <-l.out:
					stream.Send(e)
				default:
					return nil
				}
			}
		case <-lost:
			// The transport failed: an observation, not an intention. The heartbeat window decides.
			s.mu.Lock()
			l.lost = true
			s.mu.Unlock()
			return nil
		}
	}
}

// opening accepts a first envelope that is a session OPEN carrying an identity admission issued and
// nobody has used.
func (s *Service) opening(e *pluginv1.Envelope) (*live, bool) {
	if e.GetSession().GetOpen() == nil || e.MessageId == "" {
		return nil, false
	}
	admitted, issued := s.cfg.Lookup(e.SessionId)
	s.mu.Lock()
	if !issued || s.used[e.SessionId] {
		s.mu.Unlock()
		return nil, false
	}
	s.used[e.SessionId] = true
	l := &live{id: e.SessionId, terms: admitted, out: make(chan *pluginv1.Envelope, 64), done: make(chan struct{}),
		seen: map[string]bool{e.MessageId: true}, sent: map[string]bool{}}
	s.open[l.id] = l
	l.timer = time.AfterFunc(window(admitted), func() {
		s.end(l, "revoked", pluginv1.SessionMessage_Revoked_CAUSE_LIVENESS_LOST, "no heartbeat arrived within the terms admission set")
	})
	s.mu.Unlock()
	// The machine is told outside the lock: the supervisor may be telling this service something at the
	// same moment, holding its own.
	s.cfg.Log.Info("session", "unit", admitted.Unit, "event", "opened")
	s.observe(admitted.Unit, unit.SessionOpened{})
	return l, true
}

// window is how long the Core waits for a heartbeat before concluding: the interval, times the misses
// it tolerates.
func window(a Admitted) time.Duration {
	tolerance := a.Tolerance
	if tolerance < 1 {
		tolerance = 1
	}
	return a.Interval * time.Duration(tolerance)
}

func (s *Service) observe(unitID string, in unit.Input) {
	if s.cfg.Observe != nil {
		s.cfg.Observe(unitID, in)
	}
}

// receive validates one envelope from the unit, in order, and acts on it.
func (s *Service) receive(l *live, e *pluginv1.Envelope) {
	refuse := func(code pluginv1.Code, correlation, format string, args ...any) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.queue(l, &pluginv1.Envelope{CorrelationId: correlation, Payload: &pluginv1.Envelope_Error{Error: &pluginv1.Error{Code: codeName(code), Message: fmt.Sprintf(format, args...)}}})
	}
	// 1 · structural
	if e.Payload == nil || e.MessageId == "" || e.SessionId == "" {
		refuse(pluginv1.Code_CODE_SESSION_MESSAGE_MALFORMED, e.MessageId, "an envelope carries a message identity, a Session identity and exactly one payload")
		return
	}
	// 2 · Session resolution: a message this Session cannot attribute has nowhere to be answered.
	if e.SessionId != l.id {
		s.cfg.Log.Warn("session", "unit", l.terms.Unit, "event", "dropped", "reason", "the envelope names another Session")
		return
	}
	s.mu.Lock()
	// 3 · validity
	if l.ended {
		s.mu.Unlock()
		refuse(pluginv1.Code_CODE_SESSION_REVOKED, e.MessageId, "the Session has ended")
		return
	}
	// 4 · uniqueness
	if l.seen[e.MessageId] {
		s.mu.Unlock()
		refuse(pluginv1.Code_CODE_SESSION_MESSAGE_DUPLICATE, e.MessageId, "the message identity %s was already used in this Session", e.MessageId)
		return
	}
	l.seen[e.MessageId] = true
	sent := l.sent[e.CorrelationId]
	s.mu.Unlock()
	// 5 · direction: an instruction from a unit is refused for being one, whatever it says.
	switch {
	case e.GetControl() != nil, e.GetQuery().GetQuestion() != nil, e.GetSession().GetRevoked() != nil:
		refuse(pluginv1.Code_CODE_SESSION_DIRECTION, e.MessageId, "a unit does not originate this family")
		return
	}
	// 6 · correlation, where the family requires it.
	answers := e.GetAck() != nil || e.GetQuery().GetAnswer() != nil
	switch {
	case answers && e.CorrelationId == "":
		refuse(pluginv1.Code_CODE_SESSION_CORRELATION_MISSING, e.MessageId, "an answer names the message it answers")
		return
	case (answers || e.GetError() != nil && e.CorrelationId != "") && (!sent || e.CorrelationId == e.MessageId):
		refuse(pluginv1.Code_CODE_SESSION_CORRELATION_UNKNOWN, e.MessageId, "the correlation %s names no message the Core sent in this Session", e.CorrelationId)
		return
	}
	// 7 · semantics
	switch {
	case e.GetHealth() != nil:
		s.mu.Lock()
		l.timer.Reset(window(l.terms))
		s.mu.Unlock()
	case e.GetSession().GetClose() != nil:
		s.end(l, "closed", 0, "")
	case e.GetSession().GetOpen() != nil:
		s.end(l, "revoked", pluginv1.SessionMessage_Revoked_CAUSE_PROTOCOL_FAILURE, "a Session is opened once")
	case e.GetData() != nil:
		refuse(pluginv1.Code_CODE_STREAM_INACTIVE, e.MessageId, "data travels on its stream's own transport, never on the Session")
	}
}

// queue sends an envelope from the Core, filling its header. Lock held.
func (s *Service) queue(l *live, e *pluginv1.Envelope) string {
	l.next++
	e.MessageId, e.SessionId, e.SentAtUnixNano = fmt.Sprintf("c-%d", l.next), l.id, time.Now().UnixNano()
	l.sent[e.MessageId] = true
	select {
	case l.out <- e:
	default:
	}
	return e.MessageId
}

// Send sends an envelope from the Core on an open Session and returns its message identity.
func (s *Service) Send(id string, e *pluginv1.Envelope) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.open[id]
	if !ok || l.ended {
		return "", errors.New("no such Session is open")
	}
	return s.queue(l, e), nil
}

// Revoke ends a Session on the Core's authority, naming which of the four triggers it was.
func (s *Service) Revoke(id string, cause pluginv1.SessionMessage_Revoked_Cause, line string) error {
	s.mu.Lock()
	l, ok := s.open[id]
	s.mu.Unlock()
	if !ok {
		return errors.New("no such Session is open")
	}
	s.end(l, "revoked", cause, line)
	return nil
}

// Forget ends a unit's Session because its incarnation ended: the machine has already concluded, and
// nothing is told to it.
func (s *Service) Forget(unitID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, l := range s.open {
		if l.terms.Unit == unitID && !l.ended {
			l.ended = true
			l.timer.Stop()
			delete(s.open, id)
			close(l.done)
		}
	}
}

// end ends a Session once: closed by the unit, or revoked by the Core. A revocation is sent if the
// stream is still there; a withdrawal by decision is told to the machine as one.
func (s *Service) end(l *live, how string, cause pluginv1.SessionMessage_Revoked_Cause, line string) {
	s.mu.Lock()
	if l.ended {
		s.mu.Unlock()
		return
	}
	l.ended = true
	l.timer.Stop()
	delete(s.open, l.id)
	if how == "revoked" && !l.lost {
		l.next++
		e := &pluginv1.Envelope{MessageId: fmt.Sprintf("c-%d", l.next), SessionId: l.id, SentAtUnixNano: time.Now().UnixNano(),
			Payload: &pluginv1.Envelope_Session{Session: &pluginv1.SessionMessage{Kind: &pluginv1.SessionMessage_Revoked_{Revoked: &pluginv1.SessionMessage_Revoked{Cause: cause, Line: line}}}}}
		select {
		case l.out <- e:
		default:
		}
	}
	close(l.done)
	s.mu.Unlock()
	attrs := []any{"unit", l.terms.Unit, "ended", how}
	if how == "revoked" {
		attrs = append(attrs, "cause", cause.String(), "line", line)
	}
	s.cfg.Log.Info("session", attrs...)
	s.observe(l.terms.Unit, unit.SessionEnded{Withdrawn: how == "revoked" && cause == pluginv1.SessionMessage_Revoked_CAUSE_PLUGIN_DISABLED})
}

// codeName is a code as it travels: its dotted name.
func codeName(c pluginv1.Code) string {
	value := c.Descriptor().Values().ByNumber(c.Number())
	name, _ := proto.GetExtension(value.Options(), pluginv1.E_Code).(string)
	return name
}
