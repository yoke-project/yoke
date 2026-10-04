// Package interfaces is the interface surface's side of the Core: the channels a deployment declares,
// bound at step 9 by the class of their address, each answering in the projection it declares.
package interfaces

import (
	"errors"
	"os"

	"github.com/yoke-project/yoke/internal/gate"
)

// Bound are a deployment's channels, bound.
type Bound struct{}

// Bind binds every channel declared.
func Bind(root string, mode os.FileMode, channels []gate.Channel) (*Bound, error) {
	return nil, errors.New("not yet")
}

// Address is where a channel is bound.
func (b *Bound) Address(name string) string { return "" }

// Close unbinds every channel.
func (b *Bound) Close() error { return nil }
