// Package streams holds a stream's own transport: created and listened on by the Core before the stream
// is activated, chosen by the two tolerances the stream declares, read for as long as the stream flows,
// and removed on each of the three routes that end a stream.
package streams

import (
	"errors"
	"log/slog"

	"github.com/yoke-project/yoke/internal/core/event"
)

// Transport is what carries a stream's data.
type Transport string

const (
	Ordered Transport = "ordered"
	Framed  Transport = "framed"
)

// Tolerances are what a stream's data tolerates, as its Manifest declares them.
type Tolerances struct{ Loss, Reorder bool }

// The reasons a stream stops.
const (
	Asked        = "asked"
	SessionEnded = "session ended"
	UnitExited   = "unit exited"
)

// Frame is one message read from a stream's transport.
type Frame struct {
	Unit, Stream     string
	Sequence, SentAt uint64
	Payload          []byte
}

// Config is where the transports live and whom the service tells.
type Config struct {
	Root    string
	Log     *slog.Logger
	Deliver func(Frame)
	Publish func(event.Event)
}

// Service holds every stream's transport.
type Service struct{ cfg Config }

func New(cfg Config) *Service { return &Service{cfg: cfg} }

func (s *Service) Open(unitID string, incarnation uint64, stream string, t Tolerances) (Transport, string, error) {
	return "", "", errors.New("not yet")
}

func (s *Service) Close(unitID, stream, reason string) bool { return false }

func (s *Service) CloseAll(unitID, reason string) {}

func (s *Service) Active(unitID string) []string { return nil }

func (s *Service) Address(unitID, stream string) string { return "" }
