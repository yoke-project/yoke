// Package session is the accepted context everything after admission happens in: one bidirectional
// stream that is the Session.
//
// Its first envelope is a session OPEN carrying the identity admission issued, and anything else closes
// the stream with nothing sent; an identity is used once. Validity is a live property the Core governs:
// heartbeats within the terms admission set keep it, too many misses revoke it. An orderly close is the
// unit's and a revocation the Core's, and they stay distinguishable; a lost stream is neither, and what
// follows it comes from the heartbeat window. Every message carries four header fields and one payload,
// and a receiver validates in order, stopping at the first failure.
//
// Eight families travel in it, each in one direction only, except errors: the Core sends control,
// questions and errors, and the unit everything else. An acknowledgement answers an instruction and an
// answer a question; an acceptance is followed by at most one final answer, which ends the exchange,
// and whoever asked is handed the first answer.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/scope"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// Admitted is what admission issued with a Session identity: whose it is, the heartbeat terms, and the
// scope the unit was granted.
type Admitted struct {
	Unit      string
	Interval  time.Duration
	Tolerance uint32
	Scope     *scope.Scope
}

// Refused is a sending the Core declined where it was asked for, with the code a unit would have been
// answered with had it sent the same.
type Refused struct {
	Code   pluginv1.Code
	Reason string
}

func (r *Refused) Error() string { return codeName(r.Code) + ": " + r.Reason }

// permitted checks what the Core would send against the unit's grant: a command, a question, and a
// stream's activation or stop each name an object a capability governs.
func permitted(sc *scope.Scope, e *pluginv1.Envelope) error {
	var kind scope.Kind
	var id string
	switch c, q := e.GetControl(), e.GetQuery().GetQuestion(); {
	case c.GetCommand() != nil:
		kind, id = scope.Command, c.GetCommand().GetType()
	case c.GetActivate() != nil:
		kind, id = scope.Stream, c.GetActivate().GetStream()
	case c.GetStop() != nil:
		kind, id = scope.Stream, c.GetStop().GetStream()
	case q != nil:
		kind, id = scope.Query, q.GetType()
	default:
		return nil
	}
	if code := sc.Check(kind, id); code != pluginv1.Code_CODE_UNSPECIFIED {
		return &Refused{Code: code, Reason: fmt.Sprintf("the unit's grant does not cover the %s %s", kind, id)}
	}
	return nil
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
	seen      map[string]bool      // the unit's message identities
	sent      map[string]bool      // the Core's, which a message may answer
	exchanges map[string]*exchange // the Core's instructions and questions still awaiting an answer
	next      int
	timer     *time.Timer
	ended     bool
	lost      bool
}

// exchange is one instruction or question the Core sent, until its final answer: an acknowledgement
// answers an instruction and an answer a question, and whoever asked is handed the first.
type exchange struct {
	instruction bool
	accepted    bool
	handed      bool
	waiter      chan *pluginv1.Envelope // nil once nobody is waiting
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
		seen: map[string]bool{e.MessageId: true}, sent: map[string]bool{}, exchanges: map[string]*exchange{}}
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
		s.cfg.Log.Warn("session", "unit", l.terms.Unit, "refused", codeName(code), "message", e.MessageId)
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
	ex := l.exchanges[e.CorrelationId]
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
	// An acknowledgement answers an instruction and an answer a question, while it awaits one.
	case e.GetAck() != nil && (ex == nil || !ex.instruction), e.GetQuery().GetAnswer() != nil && (ex == nil || ex.instruction):
		refuse(pluginv1.Code_CODE_SESSION_CORRELATION_UNKNOWN, e.MessageId, "the correlation %s names nothing of the Core's awaiting this answer", e.CorrelationId)
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
	case e.GetAck() != nil:
		switch outcome := e.GetAck().GetOutcome(); {
		case outcome == pluginv1.Ack_OUTCOME_UNSPECIFIED:
			refuse(pluginv1.Code_CODE_SESSION_MESSAGE_MALFORMED, e.MessageId, "an acknowledgement is accepted, done or failed")
		case outcome == pluginv1.Ack_OUTCOME_ACCEPTED && ex.accepted:
			refuse(pluginv1.Code_CODE_SESSION_MESSAGE_MALFORMED, e.MessageId, "an acceptance is followed by one final answer and nothing else")
		default:
			s.answered(l, e, outcome != pluginv1.Ack_OUTCOME_ACCEPTED)
		}
	case e.GetQuery().GetAnswer() != nil:
		s.answered(l, e, true)
	case e.GetEvent() != nil:
		// The severity a unit attaches widens nothing: what it may report is its grant.
		if code := l.terms.Scope.Check(scope.Occurrence, e.GetEvent().GetOccurrence()); code != pluginv1.Code_CODE_UNSPECIFIED {
			refuse(code, e.MessageId, "the unit's grant does not cover the occurrence %s", e.GetEvent().GetOccurrence())
		}
	}
}

// answered hands an answer to whoever asked, if it is the first and somebody still waits, and ends the
// exchange if the answer is final. What reaches no caller is recorded.
func (s *Service) answered(l *live, e *pluginv1.Envelope, final bool) {
	s.mu.Lock()
	ex := l.exchanges[e.CorrelationId]
	if ex == nil {
		s.mu.Unlock()
		return
	}
	first := !ex.handed
	ex.handed, ex.accepted = true, ex.accepted || !final
	waiter := ex.waiter
	if final {
		delete(l.exchanges, e.CorrelationId)
	}
	if first && waiter != nil {
		ex.waiter = nil
		waiter <- e
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	attrs := []any{"unit", l.terms.Unit, "event", "answer", "correlation", e.CorrelationId, "reached", "no caller"}
	if ack := e.GetAck(); ack != nil {
		attrs = append(attrs, "outcome", ack.GetOutcome().String(), "line", ack.GetLine())
	}
	s.cfg.Log.Info("session", attrs...)
}

// queue sends an envelope from the Core, filling its header. Lock held.
func (s *Service) queue(l *live, e *pluginv1.Envelope) string {
	l.next++
	e.MessageId, e.SessionId, e.SentAtUnixNano = fmt.Sprintf("c-%d", l.next), l.id, time.Now().UnixNano()
	l.sent[e.MessageId] = true
	if e.GetControl() != nil || e.GetQuery().GetQuestion() != nil {
		l.exchanges[e.MessageId] = &exchange{instruction: e.GetControl() != nil}
	}
	select {
	case l.out <- e:
	default:
	}
	return e.MessageId
}

// Send sends an envelope from the Core on an open Session and returns its message identity. The Core
// originates control, questions and errors, and nothing else: a revocation goes by Revoke.
func (s *Service) Send(id string, e *pluginv1.Envelope) (string, error) {
	if e.GetControl() == nil && e.GetQuery().GetQuestion() == nil && e.GetError() == nil {
		return "", &Refused{Code: pluginv1.Code_CODE_SESSION_DIRECTION, Reason: "the Core does not originate this family"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.open[id]
	if !ok || l.ended {
		return "", errors.New("no such Session is open")
	}
	if err := permitted(l.terms.Scope, e); err != nil {
		return "", err
	}
	return s.queue(l, e), nil
}

// Command sends a command on an open Session and hands back the first acknowledgement the unit sends,
// or the context's error when the caller's wait runs out first. The exchange outlives the wait: an
// acknowledgement arriving later is received, and reaches no caller.
func (s *Service) Command(ctx context.Context, id string, c *pluginv1.Control_Command) (*pluginv1.Ack, error) {
	got, err := s.issue(ctx, id, &pluginv1.Envelope{Payload: &pluginv1.Envelope_Control{Control: &pluginv1.Control{Kind: &pluginv1.Control_Command_{Command: c}}}})
	return got.GetAck(), err
}

// Ask sends a question on an open Session and hands back the unit's answer, or the context's error when
// the caller's wait runs out first.
func (s *Service) Ask(ctx context.Context, id string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error) {
	got, err := s.issue(ctx, id, &pluginv1.Envelope{Payload: &pluginv1.Envelope_Query{Query: &pluginv1.Query{Kind: &pluginv1.Query_Question_{Question: q}}}})
	return got.GetQuery().GetAnswer(), err
}

// issue sends an instruction or a question and waits for the first answer.
func (s *Service) issue(ctx context.Context, id string, e *pluginv1.Envelope) (*pluginv1.Envelope, error) {
	s.mu.Lock()
	l, ok := s.open[id]
	if !ok || l.ended {
		s.mu.Unlock()
		return nil, errors.New("no such Session is open")
	}
	if err := permitted(l.terms.Scope, e); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	waiter := make(chan *pluginv1.Envelope, 1)
	sentAs := s.queue(l, e)
	l.exchanges[sentAs].waiter = waiter
	s.mu.Unlock()
	select {
	case got := <-waiter:
		return got, nil
	case <-l.done:
		return nil, errors.New("the Session ended before the unit answered")
	case <-ctx.Done():
		s.mu.Lock()
		if ex := l.exchanges[sentAs]; ex != nil {
			ex.waiter = nil
		}
		s.mu.Unlock()
		// An answer handed over while the wait ran out is still the caller's.
		select {
		case got := <-waiter:
			return got, nil
		default:
			return nil, ctx.Err()
		}
	}
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
// stream is still there; a withdrawal by decision is told to the machine as one, and the machine is told
// before the stream closes.
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
	s.mu.Unlock()
	attrs := []any{"unit", l.terms.Unit, "ended", how}
	if how == "revoked" {
		attrs = append(attrs, "cause", cause.String(), "line", line)
	}
	s.cfg.Log.Info("session", attrs...)
	s.observe(l.terms.Unit, unit.SessionEnded{Withdrawn: how == "revoked" && cause == pluginv1.SessionMessage_Revoked_CAUSE_PLUGIN_DISABLED})
	// The stream closes once the machine has concluded, so whoever sees it close sees the conclusion.
	close(l.done)
}

// codeName is a code as it travels: its dotted name.
func codeName(c pluginv1.Code) string {
	value := c.Descriptor().Values().ByNumber(c.Number())
	name, _ := proto.GetExtension(value.Options(), pluginv1.E_Code).(string)
	return name
}
