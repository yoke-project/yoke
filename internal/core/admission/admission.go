// Package admission is the single gate between a running process and a participant. A registration is
// decided in nine ordered stages against four inputs — the request, the Manifest as read at this start,
// the Registry, and the bootstrap token — and has three outcomes.
//
// The order is part of the contract. Structure comes before trust; a disabled plugin is refused before
// its credential is examined, so a refusal it earns burns no token; authentication comes before policy,
// so an unproven caller learns nothing of what a plugin is authorised for. Stage 7 never refuses: what
// is granted is what is declared and authorised, computed once, and what is withheld is named item by
// item. The credential is never written anywhere, and a refused request's declarations are not kept.
package admission

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// Composed is what the deployment says of a unit: which plugin it is a copy of, and its figures.
type Composed struct {
	Plugin string
	Policy gate.Policy
}

// Config is what admission decides against.
type Config struct {
	Registry *registry.Registry
	// Manifest is a plugin's Manifest as read at this start, and ok false when nothing declares it.
	Manifest func(plugin string) (*gate.Manifest, bool)
	// Unit is what the composing document says of a unit, and ok false when nothing composes it.
	Unit   func(unitID string) (Composed, bool)
	Tokens *Tokens
	// AdmitUnlaunched is the development waiver: it relaxes authentication and nothing else.
	AdmitUnlaunched bool
	Log             *slog.Logger
	// Observe hands the lifecycle machine an outcome that is the unit's.
	Observe func(unitID string, in unit.Input)
}

// Session is what an acceptance commits to, kept in memory for the life of the incarnation.
type Session struct {
	ID        string
	Unit      string
	Plugin    string
	Granted   *pluginv1.Surface
	Heartbeat *pluginv1.HeartbeatTerms
}

// Admission decides registrations.
type Admission struct {
	pluginv1.UnimplementedRegisterServer
	cfg Config

	mu       sync.Mutex
	live     map[string]string   // unit → the Session it holds
	sessions map[string]*Session // by identity
}

// New is admission over cfg.
func New(cfg Config) *Admission {
	return &Admission{cfg: cfg, live: map[string]string{}, sessions: map[string]*Session{}}
}

// Release ends a unit's life as far as admission is concerned: its incarnation ended, and the unit may
// be admitted again.
func (a *Admission) Release(unitID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, a.live[unitID])
	delete(a.live, unitID)
}

// Lookup finds a Session by the identity admission issued.
func (a *Admission) Lookup(id string) (*Session, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[id]
	return s, ok
}

// refusal is a stage's answer when it refuses.
type refusal struct {
	stage   pluginv1.Stage
	code    pluginv1.Code
	message string
}

// Register decides one registration.
func (a *Admission) Register(_ context.Context, req *pluginv1.RegisterRequest) (*pluginv1.RegisterResponse, error) {
	resp, refused, examined := a.decide(req)
	if refused != nil {
		resp = &pluginv1.RegisterResponse{Outcome: pluginv1.RegisterResponse_OUTCOME_REFUSED, Stage: refused.stage, Code: codeName(refused.code), Message: refused.message}
		credential := "unexamined"
		switch {
		case a.cfg.AdmitUnlaunched && refused.stage > pluginv1.Stage_STAGE_ADMINISTRATIVE_STATE:
			credential = "waived"
		case examined:
			credential = "examined"
		}
		a.cfg.Log.Warn("admission", "unit", req.GetUnit(), "plugin", req.GetPlugin(), "outcome", "refused",
			"stage", refused.stage.String(), "code", resp.Code, "credential", credential, "reason", refused.message)
		// A refusal that came after the credential was spent is the unit's own: this process was launched
		// for it. One that came before proves nothing about who sent it.
		if refused.stage >= pluginv1.Stage_STAGE_COMPATIBILITY && examined && a.cfg.Observe != nil {
			a.cfg.Observe(req.GetUnit(), unit.AdmissionDeclined{})
		}
		return resp, nil
	}
	outcome := "accepted"
	withheld := len(resp.Withheld.GetCapabilities()) + len(resp.Withheld.GetStreams()) + len(resp.Withheld.GetCommands()) + len(resp.Withheld.GetQueries())
	if resp.Outcome == pluginv1.RegisterResponse_OUTCOME_ACCEPTED_WITH_RESTRICTIONS {
		outcome = "accepted_with_restrictions"
	}
	a.cfg.Log.Info("admission", "unit", req.GetUnit(), "plugin", req.GetPlugin(), "outcome", outcome, "withheld", withheld)
	if a.cfg.Observe != nil {
		a.cfg.Observe(req.GetUnit(), unit.AdmissionAccepted{})
	}
	return resp, nil
}

// decide runs the nine stages, stopping at the first that refuses. examined says whether the credential
// was reached.
func (a *Admission) decide(req *pluginv1.RegisterRequest) (*pluginv1.RegisterResponse, *refusal, bool) {
	refuse := func(stage pluginv1.Stage, code pluginv1.Code, format string, args ...any) *refusal {
		return &refusal{stage, code, fmt.Sprintf(format, args...)}
	}
	// 1 · structural
	if req.GetPlugin() == "" || req.GetUnit() == "" {
		return nil, refuse(pluginv1.Stage_STAGE_STRUCTURAL, pluginv1.Code_CODE_ADMISSION_STRUCTURAL_MALFORMED, "a registration names both a plugin and a unit"), false
	}
	// 2 · identity: the plugin known to the Registry, and the unit one the deployment composes.
	p, known, err := a.cfg.Registry.Plugin(req.Plugin)
	if err != nil {
		return nil, refuse(pluginv1.Stage_STAGE_IDENTITY, pluginv1.Code_CODE_ADMISSION_IDENTITY_UNKNOWN_PLUGIN, "the Registry cannot be read: %v", err), false
	}
	if !known {
		return nil, refuse(pluginv1.Stage_STAGE_IDENTITY, pluginv1.Code_CODE_ADMISSION_IDENTITY_UNKNOWN_PLUGIN, "no plugin %s is known to this host", req.Plugin), false
	}
	composed, ok := a.cfg.Unit(req.Unit)
	if !ok {
		return nil, refuse(pluginv1.Stage_STAGE_IDENTITY, pluginv1.Code_CODE_ADMISSION_IDENTITY_UNKNOWN_UNIT, "no document composes a unit %s", req.Unit), false
	}
	// 3 · administrative state, before the credential is examined and so before it is spent.
	if !p.Enabled {
		return nil, refuse(pluginv1.Stage_STAGE_ADMINISTRATIVE_STATE, pluginv1.Code_CODE_ADMISSION_STATE_DISABLED, "an operator disabled the plugin %s", req.Plugin), false
	}
	// 4 · authentication: the token issued for this pair, just now, and never before used.
	examined := false
	if !a.cfg.AdmitUnlaunched {
		examined = true
		if composed.Plugin != req.Plugin {
			return nil, refuse(pluginv1.Stage_STAGE_AUTHENTICATION, pluginv1.Code_CODE_ADMISSION_AUTH_INVALID, "the unit %s is a copy of %s, and no token was issued for it as %s", req.Unit, composed.Plugin, req.Plugin), true
		}
		if code := a.cfg.Tokens.spend(req.Unit, req.Token); code != pluginv1.Code_CODE_UNSPECIFIED {
			return nil, refuse(pluginv1.Stage_STAGE_AUTHENTICATION, code, "the token presented for %s is %s", req.Unit, map[pluginv1.Code]string{
				pluginv1.Code_CODE_ADMISSION_AUTH_INVALID:  "not the one this Core issued for it",
				pluginv1.Code_CODE_ADMISSION_AUTH_EXPIRED:  "past its startup window",
				pluginv1.Code_CODE_ADMISSION_AUTH_CONSUMED: "already spent",
			}[code]), true
		}
	}
	// 5 · compatibility
	if !slices.Contains(gate.Protocols, int(req.Protocol)) {
		return nil, refuse(pluginv1.Stage_STAGE_COMPATIBILITY, pluginv1.Code_CODE_ADMISSION_COMPAT_UNSUPPORTED, "the process speaks protocol %d, and this Core speaks %v", req.Protocol, gate.Protocols), examined
	}
	// 6 · declaration consistency, against the Manifest as read at this start.
	m, declared := a.cfg.Manifest(req.Plugin)
	if !declared {
		return nil, refuse(pluginv1.Stage_STAGE_DECLARATION_CONSISTENCY, pluginv1.Code_CODE_ADMISSION_CONSISTENCY_DIVERGENT, "nothing on this host declares %s any more", req.Plugin), examined
	}
	want := manifestSurface(m)
	for i, got := range lists(req.Declared) {
		if !sameSet(got, lists(want)[i]) {
			return nil, refuse(pluginv1.Stage_STAGE_DECLARATION_CONSISTENCY, pluginv1.Code_CODE_ADMISSION_CONSISTENCY_DIVERGENT, "the process declares %s %v, and its Manifest %v", listNames[i], got, lists(want)[i]), examined
		}
	}
	// 7 · authorisation, which cannot refuse: what is declared and authorised is granted.
	granted, withheld := intersect(m, p.Grants)
	// 8 · unit conflict: one unit live twice, never one plugin running four times.
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, live := a.live[req.Unit]; live {
		return nil, refuse(pluginv1.Stage_STAGE_UNIT_CONFLICT, pluginv1.Code_CODE_ADMISSION_CONFLICT_UNIT_LIVE, "the unit %s is already live", req.Unit), examined
	}
	// 9 · session preparation: a commitment, made last.
	id, err := sessionIdentity()
	if err != nil {
		return nil, refuse(pluginv1.Stage_STAGE_SESSION_PREPARATION, pluginv1.Code_CODE_ADMISSION_SESSION_UNAVAILABLE, "no Session identity could be issued: %v", err), examined
	}
	if err := a.cfg.Registry.Record(req.Plugin, registry.Registration{Version: req.ArtifactVersion, Language: req.Language, SDK: req.SdkLine}); err != nil {
		return nil, refuse(pluginv1.Stage_STAGE_SESSION_PREPARATION, pluginv1.Code_CODE_ADMISSION_SESSION_UNAVAILABLE, "the registration could not be recorded: %v", err), examined
	}
	terms := &pluginv1.HeartbeatTerms{Interval: durationpb.New(composed.Policy.HeartbeatInterval), Tolerance: uint32(composed.Policy.HeartbeatTolerance)}
	a.live[req.Unit] = id
	a.sessions[id] = &Session{ID: id, Unit: req.Unit, Plugin: req.Plugin, Granted: granted, Heartbeat: terms}
	resp := &pluginv1.RegisterResponse{Outcome: pluginv1.RegisterResponse_OUTCOME_ACCEPTED, SessionId: id, Granted: granted, Heartbeat: terms}
	if len(slices.Concat(lists(withheld)...)) > 0 {
		resp.Outcome, resp.Withheld = pluginv1.RegisterResponse_OUTCOME_ACCEPTED_WITH_RESTRICTIONS, withheld
	}
	return resp, nil, examined
}

var listNames = []string{"capabilities", "streams", "commands", "queries"}

func lists(s *pluginv1.Surface) [][]string {
	return [][]string{s.GetCapabilities(), s.GetStreams(), s.GetCommands(), s.GetQueries()}
}

func sameSet(a, b []string) bool {
	return slices.Equal(slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b)))
}

func manifestSurface(m *gate.Manifest) *pluginv1.Surface {
	s := &pluginv1.Surface{Commands: m.Commands, Queries: m.Queries}
	for _, c := range m.Capabilities {
		s.Capabilities = append(s.Capabilities, c.Name)
	}
	for _, st := range m.Streams {
		s.Streams = append(s.Streams, st.ID)
	}
	return s
}

// intersect is the grant: a capability declared and authorised is granted, and so is what it governs;
// what is declared and not granted is withheld, item by item. What is authorised and not declared grants
// nothing and is reported to nobody here.
func intersect(m *gate.Manifest, grants []string) (granted, withheld *pluginv1.Surface) {
	granted, withheld = &pluginv1.Surface{}, &pluginv1.Surface{}
	governed := map[gate.Object]bool{}
	for _, c := range m.Capabilities {
		if slices.Contains(grants, c.Name) {
			granted.Capabilities = append(granted.Capabilities, c.Name)
			governed[c.Governs] = true
		} else {
			withheld.Capabilities = append(withheld.Capabilities, c.Name)
		}
	}
	sort := func(kind string, ids []string, into, out *[]string) {
		for _, id := range ids {
			if governed[gate.Object{Kind: kind, ID: id}] {
				*into = append(*into, id)
			} else {
				*out = append(*out, id)
			}
		}
	}
	var streams []string
	for _, s := range m.Streams {
		streams = append(streams, s.ID)
	}
	sort("stream", streams, &granted.Streams, &withheld.Streams)
	sort("command", m.Commands, &granted.Commands, &withheld.Commands)
	sort("query", m.Queries, &granted.Queries, &withheld.Queries)
	return granted, withheld
}

// sessionIdentity is 32 random bytes: a secret a message carries in place of a credential.
func sessionIdentity() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// codeName is a code as it travels: its dotted name.
func codeName(c pluginv1.Code) string {
	value := c.Descriptor().Values().ByNumber(c.Number())
	name, _ := proto.GetExtension(value.Options(), pluginv1.E_Code).(string)
	return name
}

// Tokens are the bootstrap tokens this Core issued: one per unit, for one launch, valid for that unit's
// startup window and spent by the first registration that presents it. They are never written down.
type Tokens struct {
	window func(unitID string) time.Duration
	now    func() time.Time

	mu     sync.Mutex
	issued map[string]*token
}

type token struct {
	value string
	at    time.Time
	spent bool
}

// NewTokens keeps tokens valid for each unit's own startup window.
func NewTokens(window func(unitID string) time.Duration) *Tokens {
	return &Tokens{window: window, now: time.Now, issued: map[string]*token{}}
}

// Issue is the token for a unit's next launch; the one issued for its last launch is no longer any good.
func (t *Tokens) Issue(unitID string) string {
	b := make([]byte, 32)
	rand.Read(b)
	value := base64.RawURLEncoding.EncodeToString(b)
	t.mu.Lock()
	defer t.mu.Unlock()
	t.issued[unitID] = &token{value: value, at: t.now()}
	return value
}

// spend checks a token presented for a unit and spends it if it is good, returning why it is not.
func (t *Tokens) spend(unitID, value string) pluginv1.Code {
	t.mu.Lock()
	defer t.mu.Unlock()
	tok, ok := t.issued[unitID]
	switch {
	case !ok || value == "" || subtle.ConstantTimeCompare([]byte(tok.value), []byte(value)) != 1:
		return pluginv1.Code_CODE_ADMISSION_AUTH_INVALID
	case tok.spent:
		return pluginv1.Code_CODE_ADMISSION_AUTH_CONSUMED
	case t.now().Sub(tok.at) > t.window(unitID):
		return pluginv1.Code_CODE_ADMISSION_AUTH_EXPIRED
	}
	tok.spent = true
	return pluginv1.Code_CODE_UNSPECIFIED
}
