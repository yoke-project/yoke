package engine

import (
	"context"
	"errors"
	"io"
)

// Launch is what a container is created with.
type Launch struct {
	Image       string
	Args        []string
	Env         []string
	Directory   string
	Instance    string
	Unit        string
	Incarnation int
	UID, GID    int
}

// Absent is an image the engine does not hold.
type Absent struct{ Image string }

func (a *Absent) Error() string { return "absent " + a.Image }

// Event is one of the engine's events about a container.
type Event struct {
	ID       string
	Action   string
	ExitCode int
}

var errNotYet = errors.New("not yet")

func (e *Engine) Create(ctx context.Context, l Launch) (string, error) { return "", errNotYet }
func (e *Engine) Attach(ctx context.Context, id string, stdout, stderr io.Writer) (<-chan struct{}, error) {
	return nil, errNotYet
}
func (e *Engine) Start(ctx context.Context, id string) error          { return errNotYet }
func (e *Engine) Signal(ctx context.Context, id, signal string) error { return errNotYet }
func (e *Engine) Remove(ctx context.Context, id string) error         { return errNotYet }
func (e *Engine) Events(ctx context.Context, instance string) (<-chan Event, error) {
	return nil, errNotYet
}

// Slice is the instance's control group.
func Slice(instance string) string { return "" }
