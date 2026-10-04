package interfaces

import (
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/gate"
)

// Arbiter decides which channels are suspended.
type Arbiter struct{}

// NewArbiter is an arbiter over the channels and rules declared.
func NewArbiter(channels []gate.Channel, rules []gate.Rule, publish func(event.Event)) *Arbiter {
	return &Arbiter{}
}
