package interfaces

import (
	"context"
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
	Instruct(ctx context.Context, unit string, c *pluginv1.Control) (*pluginv1.Ack, error)
	Ask(ctx context.Context, unit string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error)
}

// Transports are the streams' own transports.
type Transports interface {
	Open(unit string, incarnation uint64, stream string, t streams.Tolerances) (streams.Transport, string, error)
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
	s := &Surface{cfg: cfg}
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
	a, ref := s.begin(stream.Context(), client, stream.Send)
	if ref != nil {
		return refusedStatus(ref)
	}
	reason := "lost"
	defer func() { a.finish(reason) }()
	if err := a.live(&interfacev1.CoreFrame{Carries: &interfacev1.CoreFrame_Opening{Opening: a.opening}}); err != nil {
		return err
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
