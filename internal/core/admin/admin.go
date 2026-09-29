// Package admin is the administrative surface: one contract carried by two projections, the operator's
// on operator.sock and the shell's on shell.sock, reached by whoever can open the socket and attributed
// to the account the kernel names at the other end.
package admin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"

	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Version is the contract's version this Core speaks.
const Version = 1

// Projection is which of the surface's two a connection arrived on.
type Projection string

const (
	OperatorProjection Projection = "operator"
	ShellProjection    Projection = "shell"
)

// The reasons a connection closes: its caller ended it, or its transport went.
const (
	Cancelled = "cancelled"
	Dropped   = "dropped"
)

// A Connection is a person or a tool attached to the surface. It is not a Session: that word is a unit's.
type Connection struct {
	ID         string
	Projection Projection
	Actor      event.Actor
	Opened     time.Time
}

// Config is what the surface is served with.
type Config struct {
	// Publish publishes what the surface concludes.
	Publish func(event.Event)
	// Accounts resolves an account's number to its name; nil reads the host's account database.
	Accounts func(uid string) (string, error)
	Log      *slog.Logger
}

// Surface serves both projections.
type Surface struct {
	cfg      Config
	accounts func(uid string) (string, error)

	mu    sync.Mutex
	count uint64
	open  map[string]Connection
}

// New makes the surface.
func New(cfg Config) *Surface {
	s := &Surface{cfg: cfg, accounts: cfg.Accounts, open: map[string]Connection{}}
	if s.accounts == nil {
		s.accounts = hostAccounts
	}
	if s.cfg.Publish == nil {
		s.cfg.Publish = func(event.Event) {}
	}
	if s.cfg.Log == nil {
		s.cfg.Log = slog.New(slog.DiscardHandler)
	}
	return s
}

// Probe and Missed are the surface's liveness, on the shape of every other in the project: a probe every
// ten seconds, and a transport that leaves three unanswered is closed.
const (
	Probe  = 10 * time.Second
	Missed = 3
)

// Liveness is the probe every server of the surface is built with.
func Liveness() keepalive.ServerParameters {
	return keepalive.ServerParameters{Time: Probe, Timeout: Missed * Probe}
}

func (s *Surface) server() *grpc.Server {
	return grpc.NewServer(grpc.Creds(peerCredentials{}), grpc.KeepaliveParams(Liveness()),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: Probe / 2, PermitWithoutStream: true}))
}

// Operator is the server of the operator projection.
func (s *Surface) Operator() *grpc.Server {
	server := s.server()
	administrativev1.RegisterOperatorServer(server, operator{Surface: s})
	return server
}

// Shell is the server of the shell projection.
func (s *Surface) Shell() *grpc.Server {
	server := s.server()
	administrativev1.RegisterShellServer(server, shell{Surface: s})
	return server
}

// Connections are the connections open now, in the order they opened.
func (s *Surface) Connections() []Connection {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Connection, 0, len(s.open))
	for _, c := range s.open {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Connection) int { return a.Opened.Compare(b.Opened) })
	return out
}

// opened issues a connection its identity, unique within this life of the instance and never reused, and
// announces it.
func (s *Surface) opened(p Projection, actor event.Actor) Connection {
	s.mu.Lock()
	s.count++
	c := Connection{ID: fmt.Sprintf("c-%d", s.count), Projection: p, Actor: actor, Opened: time.Now()}
	s.open[c.ID] = c
	s.mu.Unlock()
	s.cfg.Publish(event.ConnectionOpened(c.ID, string(p), actor))
	return c
}

func (s *Surface) closed(c Connection, reason string) {
	s.mu.Lock()
	delete(s.open, c.ID)
	s.mu.Unlock()
	s.cfg.Publish(event.ConnectionClosed(c.ID, string(c.Projection), reason, c.Actor))
}

// refusal is a refusal on this surface: a code, and a message for a person.
func refusal(code, message string) *administrativev1.Refusal {
	return &administrativev1.Refusal{Code: code, Message: message}
}

// answer answers one request from actor. No operation is served yet: every one is refused as unknown.
func (s *Surface) answer(_ context.Context, _ event.Actor, _ *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal) {
	return nil, refusal("operation.unknown", "this Core serves no operation yet")
}

// refused is a refusal carried in the transport's status, which it does not replace.
func refused(r *administrativev1.Refusal) error {
	st, err := status.New(codes.Unimplemented, r.GetMessage()).WithDetails(r)
	if err != nil {
		return status.Error(codes.Internal, r.GetCode())
	}
	return st.Err()
}

type operator struct {
	*Surface
	administrativev1.UnimplementedOperatorServer
}

// Call answers one request, established anew as the account that made it: a call has nothing that
// survives it.
func (o operator) Call(ctx context.Context, r *administrativev1.Request) (*administrativev1.Response, error) {
	resp, ref := o.answer(ctx, o.actor(ctx), r)
	if ref != nil {
		return nil, refused(ref)
	}
	return resp, nil
}

// Watch answers one request with a stream.
func (o operator) Watch(r *administrativev1.Request, stream administrativev1.Operator_WatchServer) error {
	_, ref := o.answer(stream.Context(), o.actor(stream.Context()), r)
	if ref == nil {
		ref = refusal("operation.unknown", "this Core serves no operation yet")
	}
	return refused(ref)
}

type shell struct {
	*Surface
	administrativev1.UnimplementedShellServer
}

// Connect holds a connection open: the actor is established once, when it opens, and every request on it
// is that actor's.
func (sh shell) Connect(stream administrativev1.Shell_ConnectServer) error {
	ctx := stream.Context()
	actor := sh.actor(ctx)
	c := sh.opened(ShellProjection, actor)
	reason := Dropped
	defer func() { sh.closed(c, reason) }()
	var sending sync.Mutex
	send := func(f *administrativev1.CoreFrame) error {
		sending.Lock()
		defer sending.Unlock()
		return stream.Send(f)
	}
	if err := send(&administrativev1.CoreFrame{Carries: &administrativev1.CoreFrame_Opening{Opening: &administrativev1.Opening{
		Connection: c.ID, Actor: &administrativev1.Actor{Class: string(actor.Class), Person: actor.Person}, Version: Version,
	}}}); err != nil {
		return err
	}
	for {
		f, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			reason = Cancelled
			return nil
		}
		if err != nil {
			if status.Code(err) == codes.Canceled && ctx.Err() == nil {
				reason = Cancelled
			}
			return nil
		}
		if f.GetRequest() == nil {
			continue
		}
		resp, ref := sh.answer(ctx, actor, f.GetRequest())
		out := &administrativev1.CoreFrame{Call: f.GetCall()}
		if ref != nil {
			out.Carries = &administrativev1.CoreFrame_Refusal{Refusal: ref}
		} else {
			out.Carries = &administrativev1.CoreFrame_Answer{Answer: resp}
		}
		if err := send(out); err != nil {
			return err
		}
	}
}
