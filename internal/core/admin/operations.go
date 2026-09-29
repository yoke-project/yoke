package admin

import (
	"context"
	"errors"
	"time"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/gate"
)

// Units is the supervisor, as the operations on a unit reach it.
type Units interface {
	// Plugin is the plugin a declared unit runs, empty for a unit of another kind; false for a unit
	// nobody declared.
	Plugin(unit string) (string, bool)
	// Of are the units declared to run a plugin.
	Of(plugin string) []string
	Status(unit string) supervisor.Status
	StartUnit(unit string) error
	StopUnit(unit string) error
	RestartUnit(unit string) error
}

// ErrUnreachable is what Units answers when the backend a unit runs on cannot be reached.
var ErrUnreachable = supervisor.ErrUnreachable

// Sessions are the units' Sessions, as the operations reach them.
type Sessions interface {
	// Open says whether a unit holds a Session now.
	Open(unit string) bool
	Ask(ctx context.Context, unit string, q *pluginv1.Query_Question) (*pluginv1.Query_Answer, error)
	Revoke(unit string, cause pluginv1.SessionMessage_Revoked_Cause, line string) error
}

// Wait is how long the Core waits for a unit's acknowledgement or answer. It is not declarable.
const Wait = 30 * time.Second

// Bound is the most an opaque question or answer may hold.
const Bound = 1 << 20

// Core is what the operations act on: every one terminates here, and none is forwarded to a unit.
type Core struct {
	Registry *registry.Registry
	Manifest func(plugin string) (*gate.Manifest, bool)
	Units    Units
	Sessions Sessions
	Logs     *logstore.Store
	Publish  func(event.Event)
	// Wait replaces the 30 s wait, for a test.
	Wait time.Duration
}

// Operations are the operations the Core serves, by name.
func (c *Core) Operations() map[string]Operation { return map[string]Operation{} }

var _ = errors.New
