package interfaces

import (
	"context"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
)

// Operation answers one member of the union: once, or by a stream the Core ends or the caller cancels.
type Operation struct {
	Answer func(context.Context, *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal)
	Stream func(context.Context, *interfacev1.Request, func(*interfacev1.Response) error) *interfacev1.Refusal
}

// Config is what a surface serves.
type Config struct {
	Operations map[string]Operation
	// Picture is the opening picture. Optional.
	Picture func() *interfacev1.Snapshot
}

// Surface is the local projection's terminator.
type Surface struct {
	interfacev1.UnimplementedInterfaceServer
	cfg Config
}

func NewSurface(cfg Config) *Surface { return &Surface{cfg: cfg} }
