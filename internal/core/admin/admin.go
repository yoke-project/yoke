// Package admin is the administrative surface: one contract carried by two projections, the operator's
// on operator.sock and the shell's on shell.sock, reached by whoever can open the socket and attributed
// to the account the kernel names at the other end.
package admin

import (
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Projection is which of the surface's two a connection arrived on.
type Projection string

const (
	OperatorProjection Projection = "operator"
	ShellProjection    Projection = "shell"
)

// A Connection is a person or a tool attached to the surface.
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
type Surface struct{}

// New makes the surface.
func New(cfg Config) *Surface { return &Surface{} }

// Liveness is the probe every server of the surface is built with.
func Liveness() keepalive.ServerParameters { return keepalive.ServerParameters{} }

// Operator is the server of the operator projection.
func (s *Surface) Operator() *grpc.Server { return grpc.NewServer() }

// Shell is the server of the shell projection.
func (s *Surface) Shell() *grpc.Server { return grpc.NewServer() }

// Connections are the connections open now.
func (s *Surface) Connections() []Connection { return nil }
