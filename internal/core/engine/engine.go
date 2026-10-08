// Package engine reaches the container engine through the interface it serves.
package engine

import (
	"context"
	"errors"
)

// API is the one version of the engine's API the Core speaks.
const API = "1.41"

// The engines, as the Core names them.
const (
	Podman = "podman"
	Docker = "docker"
)

// Engine is an engine reached: which one, whether it runs rootless, and the version it states.
type Engine struct {
	Kind     string
	Rootless bool
	Version  string
}

// Refused is an address the Core cannot speak to.
type Refused struct{ Address string }

func (r *Refused) Error() string { return "refused " + r.Address }

// Unreachable is an engine that did not answer.
type Unreachable struct {
	Address string
	Err     error
}

func (u *Unreachable) Error() string { return "unreachable " + u.Address }

// Unsupported is an engine that does not serve the version the Core speaks.
type Unsupported struct{ Min, Max string }

func (u *Unsupported) Error() string { return "unsupported" }

// Reach reaches the engine at address.
func Reach(ctx context.Context, address string) (*Engine, error) {
	return nil, errors.New("not yet")
}
