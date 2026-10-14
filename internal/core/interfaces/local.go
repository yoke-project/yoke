package interfaces

import (
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"
	"io"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/peer"
	"github.com/yoke-project/yoke/internal/core/streams"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/hostauth"
)

// Version is the interface contract's version this Core speaks.
const Version = 1

// Operation answers one member of the union: once, or by a stream the Core ends or the caller cancels.
type Operation struct {
	Answer func(context.Context, *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal)
	Stream func(context.Context, *interfacev1.Request, func(*interfacev1.Response) error) *interfacev1.Refusal
}

// Standing is the call identity of the subscription that stands from the moment a channel attaches.
const Standing = "standing"

// The confirmed subscription's figures: confirmed every 10 s, stale after 3 intervals missed. Neither is
// declarable.
const (
	ConfirmEvery     = 10 * time.Second
	ConfirmTolerance = 3
)

// Units are the units a deployment declares, as the supervisor observes them.
type Units interface {
	IDs() []string
	Kind(id string) unit.Kind
	Status(id string) supervisor.Status
}

// Confirmation is what a confirmed subscription is held to: confirmed every interval, stale after the
// tolerance's worth of intervals missed, on the channels named in an arbitration rule.
type Confirmation struct {
	Every     time.Duration
	Tolerance int
	Required  map[string]bool
}

// Config is what a surface serves.
type Config struct {
	// Authenticate asks the host's authentication service whether a secret is an account's, for a
	// routable channel. Where it is nil, nothing can establish a caller and every credential is refused.
	Authenticate func(ctx context.Context, account, secret string) (hostauth.Verdict, error)
	// Now is the Core's clock, which a session token's life is measured on. Where it is nil, the
	// system's.
	Now func() time.Time
	// Operations are the members of the union served, by the name the contract writes them with.
	Operations map[string]Operation
	// Picture is the opening picture, where nothing below computes one. Optional.
	Picture func() *interfacev1.Snapshot

	// Channel is the channel this surface serves.
	Channel gate.Channel
	// Root is the instance root, under which a subscriber's socket is created.
	Root string
	// Queue replaces the frames a delivery holds for its client, for a test.
	Queue int
	// Bus is what the standing subscription and the picture's sequence are taken from. Optional.
	Bus *bus.Bus
	// Publish is told what the channel concludes about itself. Optional.
	Publish func(event.Event)
	// Instance, Units, Granted and Active are what the records are made of. Optional.
	Instance func() *interfacev1.InstanceRecord
	Units    Units
	Granted  func(unit string) (streams, commands, queries []string)
	Active   func(unit string) []string
	// Arbiter decides which channels are suspended. Optional.
	Arbiter *Arbiter
	// Confirm is what a confirmed subscription is held to. Optional.
	Confirm *Confirmation
	// Accounts resolves an account's number to its name; the host's database where nil.
	Accounts func(uid string) (string, error)

	// Sessions carry what a channel issues against a unit, and Transports are the streams' own. Optional.
	Sessions   Sessions
	Transports Transports
	// Declared is the Manifest of the plugin a unit runs, and false for a unit that is not a Plugin.
	Declared func(unit string) (*gate.Manifest, bool)
	// Stopping says whether the instance is stopping, when operations that act are refused. Optional.
	Stopping func() bool
	// Wait replaces the 30 s a unit is waited for, for a test.
	Wait time.Duration
}

// Sessions are the units' Sessions, by unit.
type Sessions interface {
	Open(unit string) bool
	OffersShared(unit string) bool
	Instruct(ctx context.Context, unit string, c *pluginv1.Control) (*pluginv1.Ack, error)
	Ask(ctx context.Context, unit string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error)
}

// Transports are the streams' own transports.
type Transports interface {
	Open(unit string, incarnation uint64, stream string, t streams.Tolerances, shared bool) (streams.Transport, string, error)
	Close(unit, stream, reason string) bool
	Discard(unit, stream string)
	Active(unit string) []string
	Feed(unit, stream string, to func(streams.Frame), released func()) (stop func())
	NextSubscriber(unit, stream string) string
}

// Surface is the local projection's terminator: one bidirectional stream per attachment, many calls on it.
type Surface struct {
	interfacev1.UnimplementedInterfaceServer
	cfg Config

	web      *web
	sessions sessions

	mu      sync.Mutex
	clients []string // the clients attached now, in the order they attached
	current int      // the attachments whose picture is current: what makes the channel hold
}

// hold counts an attachment becoming current, or no longer current, and tells the arbiter whether the
// channel holds.
func (s *Surface) hold(delta int) {
	s.mu.Lock()
	s.current += delta
	holds := s.current > 0
	s.mu.Unlock()
	if s.cfg.Arbiter != nil {
		s.cfg.Arbiter.Hold(s.cfg.Channel.Name, holds)
	}
}

// NewSurface is a terminator serving the operations given.
func NewSurface(cfg Config) *Surface {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	s := &Surface{cfg: cfg, web: &web{attachments: map[string]*attachment{}, deliveries: map[string]*webDelivery{}}}
	ops := map[string]Operation{}
	if cfg.Bus != nil {
		ops["read"] = Operation{Answer: s.read}
		ops["subscribe"] = Operation{Stream: s.subscribe}
	}
	for name, op := range s.acting() {
		ops[name] = op
	}
	for name, op := range cfg.Operations {
		ops[name] = op
	}
	s.cfg.Operations = ops
	return s
}

var operationOneof = (&interfacev1.Request{}).ProtoReflect().Descriptor().Oneofs().ByName("operation")

// Name is the operation a request names, as the contract writes it — `read`, `stream.start` — and empty
// where it names none.
func Name(r *interfacev1.Request) string {
	f := r.ProtoReflect().WhichOneof(operationOneof)
	if f == nil {
		return ""
	}
	return strings.ReplaceAll(string(f.Name()), "_", ".")
}

func refusal(code, message string) *interfacev1.Refusal {
	return &interfacev1.Refusal{Code: code, Message: message}
}

// Attach serves one attachment on the typed projection. The client is established by the class of the
// channel's address; a single channel already held refuses it. The Core's first frame is the opening: the
// picture, the standing subscription and the version. Then every call is answered in frames carrying its
// identity, and completes with its one answer, its refusal, or a completion saying who ended it; a
// refusal belongs to its call and ends nothing else. When the attachment ends, nothing about it remains.
func (s *Surface) Attach(stream interfacev1.Interface_AttachServer) error {
	client, established := peer.Account(stream.Context(), s.cfg.Accounts)
	if !established {
		client = "unestablished"
	}
	// On a routable channel the first frame establishes the caller, before anything is attached; its
	// answer follows the opening, which stays the Core's first frame.
	var first *interfacev1.ClientFrame
	var answered *interfacev1.Authenticated
	if s.routable() {
		f, err := stream.Recv()
		if err != nil {
			return nil
		}
		var ref *interfacev1.Refusal
		if answered, ref = s.credential(stream.Context(), f.GetRequest()); ref != nil {
			return refusedStatus(ref)
		}
		client, first = answered.GetAccount(), f
	}
	a, ref := s.begin(stream.Context(), client, stream.Send)
	if ref != nil {
		return refusedStatus(ref)
	}
	reason := "lost"
	defer func() { a.finish(reason) }()
	if err := a.start(); err != nil {
		return err
	}
	if first != nil {
		a.live(&interfacev1.CoreFrame{Call: first.GetCall(), Carries: &interfacev1.CoreFrame_Answer{Answer: &interfacev1.Response{
			Answer: &interfacev1.Response_Authenticate{Authenticate: answered}}}})
	}
	for {
		f, err := stream.Recv()
		if err == io.EOF {
			reason = "closed"
			return nil
		}
		if err != nil {
			return nil
		}
		if f.GetCancel() != nil {
			a.cancel(f.GetCall())
			continue
		}
		a.handle(f.GetCall(), f.GetRequest(), a.live)
	}
}

// routable says whether the channel is on a routable address, where a credential establishes the caller.
func (s *Surface) routable() bool {
	a := s.cfg.Channel.Address
	return a != nil && a.Class == "routable"
}

// credential establishes the caller a request presents, on a routable channel: a request that is not
// authenticate presents none; a token this channel issued stands for its account while it stands; a
// password is the host's to judge, and an accepted one is answered with a fresh token. Whatever else is
// presented is the same refusal, carrying nothing about why.
func (s *Surface) credential(ctx context.Context, r *interfacev1.Request) (*interfacev1.Authenticated, *interfacev1.Refusal) {
	invalid := refusal("auth.invalid", "the credential is not accepted")
	auth := r.GetAuthenticate()
	if auth == nil {
		return nil, refusal("auth.required", "the channel "+s.cfg.Channel.Name+" is on a routable address, and requires a credential establishing the caller")
	}
	if token := auth.GetToken(); token != "" {
		account, expires, ok := s.sessions.redeem(token, s.cfg.Now())
		if !ok {
			return nil, invalid
		}
		return &interfacev1.Authenticated{Account: account, Expires: timestamppb.New(expires)}, nil
	}
	p := auth.GetPassword()
	if p == nil || p.GetAccount() == "" || s.cfg.Authenticate == nil {
		return nil, invalid
	}
	if verdict, _ := s.cfg.Authenticate(ctx, p.GetAccount(), p.GetSecret()); verdict != hostauth.Accepted {
		return nil, invalid
	}
	token, expires := s.sessions.issue(p.GetAccount(), s.cfg.Now())
	return &interfacev1.Authenticated{Account: p.GetAccount(), Token: token, Expires: timestamppb.New(expires)}, nil
}

func (s *Surface) publish(e event.Event) {
	if s.cfg.Publish != nil {
		s.cfg.Publish(e)
	}
}

// refusedStatus is a refusal that ends the attachment before it began, carried in the transport's status.
func refusedStatus(r *interfacev1.Refusal) error {
	st, err := status.New(codes.FailedPrecondition, r.GetMessage()).WithDetails(r)
	if err != nil {
		return status.Error(codes.FailedPrecondition, r.GetCode())
	}
	return st.Err()
}
