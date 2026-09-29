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
	"strings"
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

// The reasons a connection closes: its caller ended it, its transport went, or — for an operator call
// answered by a stream — the Core completed the stream.
const (
	Cancelled = "cancelled"
	Dropped   = "dropped"
	Completed = "completed"
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
	// Operations answer the union's members, by the operation's name: `read`, `log.follow`.
	Operations map[string]Operation
	// Stopping says whether the instance is stopping, when a change is refused and a read answered.
	Stopping func() bool
	Log      *slog.Logger
}

// An Operation answers one member of the union, whichever projection carried it: once, or by a stream
// the caller or the operation ends.
type Operation struct {
	Answer func(ctx context.Context, actor event.Actor, r *administrativev1.Request) (*administrativev1.Response, *administrativev1.Refusal)
	Stream func(ctx context.Context, actor event.Actor, r *administrativev1.Request, send func(*administrativev1.Response) error) *administrativev1.Refusal
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

var operationOneof = (&administrativev1.Request{}).ProtoReflect().Descriptor().Oneofs().ByName("operation")

// Name is the operation a request names, as the contract writes it — `plugin.enable`, `log.follow` — and
// empty where it names none.
func Name(r *administrativev1.Request) string {
	f := r.ProtoReflect().WhichOneof(operationOneof)
	if f == nil {
		return ""
	}
	return strings.ReplaceAll(string(f.Name()), "_", ".")
}

// Streams says whether an operation is answered by a stream, which decides the method that carries it.
func Streams(name string) bool { return name == "subscribe" || name == "log.follow" }

// operation is what answers a request: the member it names, by the shape the carrying method has.
func (s *Surface) operation(r *administrativev1.Request, stream bool) (string, Operation, *administrativev1.Refusal) {
	if r.GetVersion() != Version {
		return "", Operation{}, refusal("compat.unsupported", fmt.Sprintf("this Core speaks the contract's version %d, and the request states %d", Version, r.GetVersion()))
	}
	name := Name(r)
	if name == "" {
		return "", Operation{}, refusal("operation.malformed", "the request names no operation")
	}
	if stream != Streams(name) {
		if Streams(name) {
			return name, Operation{}, refusal("operation.malformed", name+" is answered by a stream, which a unary call cannot carry")
		}
		return name, Operation{}, refusal("operation.malformed", name+" is answered once, and a stream does not carry it")
	}
	if Changes(name) && s.cfg.Stopping != nil && s.cfg.Stopping() {
		return name, Operation{}, refusal("instance.stopping", "the instance is stopping, and "+name+" would change something")
	}
	op, served := s.cfg.Operations[name]
	if !served || (stream && op.Stream == nil) || (!stream && op.Answer == nil) {
		return name, Operation{}, refusal("operation.unknown", "this Core does not serve "+name)
	}
	return name, op, nil
}

// refused is a refusal carried in the transport's status, which it does not replace.
func refused(r *administrativev1.Refusal) error {
	c := codes.FailedPrecondition
	switch r.GetCode() {
	case "operation.malformed":
		c = codes.InvalidArgument
	case "operation.unknown":
		c = codes.Unimplemented
	}
	st, err := status.New(c, r.GetMessage()).WithDetails(r)
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
	actor := o.actor(ctx)
	_, op, ref := o.operation(r, false)
	if ref != nil {
		return nil, refused(ref)
	}
	resp, ref := op.Answer(ctx, actor, r)
	if ref != nil {
		return nil, refused(ref)
	}
	return resp, nil
}

// Watch answers one request with a stream, and is a connection while the stream lasts.
func (o operator) Watch(r *administrativev1.Request, stream administrativev1.Operator_WatchServer) error {
	ctx := stream.Context()
	actor := o.actor(ctx)
	_, op, ref := o.operation(r, true)
	if ref != nil {
		return refused(ref)
	}
	c := o.opened(OperatorProjection, actor)
	ref = op.Stream(ctx, c.Actor, r, stream.Send)
	reason := Completed
	if ctx.Err() != nil {
		reason = Cancelled
	}
	o.closed(c, reason)
	if ref != nil {
		return refused(ref)
	}
	return nil
}

type shell struct {
	*Surface
	administrativev1.UnimplementedShellServer
}

// Connect holds a connection open: the actor is established once, when it opens, and every request on it
// is that actor's. Calls interleave, each answer carries the identity its caller chose, and every call
// completes explicitly.
func (sh shell) Connect(stream administrativev1.Shell_ConnectServer) error {
	ctx, end := context.WithCancel(stream.Context())
	actor := sh.actor(ctx)
	c := sh.opened(ShellProjection, actor)
	reason := Dropped
	var (
		sending  sync.Mutex
		mu       sync.Mutex
		inFlight = map[string]context.CancelFunc{}
		calls    sync.WaitGroup
	)
	defer func() {
		end()
		calls.Wait()
		sh.closed(c, reason)
	}()
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
	refuse := func(call string, r *administrativev1.Refusal) {
		send(&administrativev1.CoreFrame{Call: call, Carries: &administrativev1.CoreFrame_Refusal{Refusal: r}})
	}
	for {
		f, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			reason = Cancelled
			return nil
		}
		if err != nil {
			return nil
		}
		call := f.GetCall()
		if f.GetCancel() != nil {
			mu.Lock()
			if stop, ok := inFlight[call]; ok {
				stop()
			}
			mu.Unlock()
			continue
		}
		r := f.GetRequest()
		if r == nil {
			continue
		}
		name := Name(r)
		_, op, ref := sh.operation(r, Streams(name))
		if ref != nil {
			refuse(call, ref)
			continue
		}
		mu.Lock()
		if _, busy := inFlight[call]; busy {
			mu.Unlock()
			refuse(call, refusal("operation.malformed", "the call "+call+" is already in flight on this connection"))
			continue
		}
		callCtx, stop := context.WithCancel(ctx)
		inFlight[call] = stop
		mu.Unlock()
		calls.Add(1)
		go func() {
			defer calls.Done()
			defer func() {
				mu.Lock()
				delete(inFlight, call)
				mu.Unlock()
				stop()
			}()
			if !Streams(name) {
				resp, ref := op.Answer(callCtx, actor, r)
				if ref != nil {
					refuse(call, ref)
					return
				}
				send(&administrativev1.CoreFrame{Call: call, Carries: &administrativev1.CoreFrame_Answer{Answer: resp}})
				return
			}
			ref := op.Stream(callCtx, actor, r, func(resp *administrativev1.Response) error {
				if callCtx.Err() != nil {
					return callCtx.Err()
				}
				return send(&administrativev1.CoreFrame{Call: call, Carries: &administrativev1.CoreFrame_Answer{Answer: resp}})
			})
			if ref != nil {
				refuse(call, ref)
				return
			}
			send(&administrativev1.CoreFrame{Call: call, Carries: &administrativev1.CoreFrame_Completion{Completion: &administrativev1.Completion{}}})
		}()
	}
}
