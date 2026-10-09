package engine

import (
	"context"
	"errors"
)

// Found is a container found under an instance's label: its identity, the unit and the incarnation it
// is, and whether it runs or the status it ended with.
type Found struct {
	ID          string
	Unit        string
	Incarnation int
	Running     bool
	ExitCode    int
}

var errNotYet = errors.New("not yet")

func (e *Engine) List(ctx context.Context, instance string) ([]Found, error) { return nil, errNotYet }
func (e *Engine) Returned(ctx context.Context) (<-chan struct{}, error)      { return nil, errNotYet }
