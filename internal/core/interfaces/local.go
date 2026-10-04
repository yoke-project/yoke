package interfaces

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"

	"github.com/yoke-project/yoke/internal/core/bus"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// Version is the interface contract's version this Core speaks.
const Version = 1

// Operation answers one member of the union: once, or by a stream the Core ends or the caller cancels.
type Operation struct {
	Answer func(context.Context, *interfacev1.Request) (*interfacev1.Response, *interfacev1.Refusal)
	Stream func(context.Context, *interfacev1.Request, func(*interfacev1.Response) error) *interfacev1.Refusal
}

// Standing is the call identity of the subscription that stands from the moment a channel attaches.
const Standing = "standing"

// Units are the units a deployment declares, as the supervisor observes them.
type Units interface {
	IDs() []string
	Kind(id string) unit.Kind
	Status(id string) supervisor.Status
}

// Confirmation is what a confirmed subscription is held to: confirmed every interval, stale after the
// tolerance's worth of intervals missed, on the channels named in an arbitration rule.
type Confirmation struct {
	Every     time.Duration
	Tolerance int
	Required  map[string]bool
}

// Config is what a surface serves.
type Config struct {
	// Operations are the members of the union served, by the name the contract writes them with.
	Operations map[string]Operation
	// Picture is the opening picture, where nothing below computes one. Optional.
	Picture func() *interfacev1.Snapshot

	// Channel is the channel this surface serves.
	Channel gate.Channel
	// Bus is what the standing subscription and the picture's sequence are taken from. Optional.
	Bus *bus.Bus
	// Publish is told what the channel concludes about itself. Optional.
	Publish func(event.Event)
	// Instance, Units, Granted and Active are what the records are made of. Optional.
	Instance func() *interfacev1.InstanceRecord
	Units    Units
	Granted  func(unit string) (streams, commands, queries []string)
	Active   func(unit string) []string
	// Confirm is what a confirmed subscription is held to. Optional.
	Confirm *Confirmation
	// Accounts resolves an account's number to its name; the host's database where nil.
	Accounts func(uid string) (string, error)
}

// Surface is the local projection's terminator: one bidirectional stream per attachment, many calls on it.
type Surface struct {
	interfacev1.UnimplementedInterfaceServer
	cfg Config
}

// NewSurface is a terminator serving the operations given.
func NewSurface(cfg Config) *Surface { return &Surface{cfg: cfg} }

var operationOneof = (&interfacev1.Request{}).ProtoReflect().Descriptor().Oneofs().ByName("operation")

// Name is the operation a request names, as the contract writes it — `read`, `stream.start` — and empty
// where it names none.
func Name(r *interfacev1.Request) string {
	f := r.ProtoReflect().WhichOneof(operationOneof)
	if f == nil {
		return ""
	}
	return strings.ReplaceAll(string(f.Name()), "_", ".")
}

func refusal(code, message string) *interfacev1.Refusal {
	return &interfacev1.Refusal{Code: code, Message: message}
}

// Attach serves one attachment. The Core's first frame is the opening; then every call is answered in
// frames carrying its identity, and completes with its one answer, its refusal, or a completion saying
// who ended it. A refusal belongs to its call and ends nothing else.
func (s *Surface) Attach(stream interfacev1.Interface_AttachServer) error {
	ctx, end := context.WithCancel(stream.Context())
	var (
		sending  sync.Mutex
		mu       sync.Mutex
		inFlight = map[string]context.CancelFunc{}
		calls    sync.WaitGroup
	)
	defer func() {
		end()
		calls.Wait()
	}()
	send := func(f *interfacev1.CoreFrame) error {
		sending.Lock()
		defer sending.Unlock()
		return stream.Send(f)
	}
	refuse := func(call string, r *interfacev1.Refusal) {
		send(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Refusal{Refusal: r}})
	}
	picture := &interfacev1.Snapshot{}
	if s.cfg.Picture != nil {
		picture = s.cfg.Picture()
	}
	if err := send(&interfacev1.CoreFrame{Carries: &interfacev1.CoreFrame_Opening{Opening: &interfacev1.Opening{Picture: picture, Version: Version}}}); err != nil {
		return err
	}
	for {
		f, err := stream.Recv()
		if err != nil {
			return nil
		}
		call := f.GetCall()
		if f.GetCancel() != nil {
			mu.Lock()
			if stop, busy := inFlight[call]; busy {
				stop()
			}
			mu.Unlock()
			continue
		}
		r := f.GetRequest()
		if r.GetVersion() != Version {
			refuse(call, refusal("compat.unsupported", fmt.Sprintf("this Core speaks the contract's version %d, and the request states %d", Version, r.GetVersion())))
			continue
		}
		name := Name(r)
		if name == "" {
			refuse(call, refusal("operation.malformed", "the request names no operation"))
			continue
		}
		mu.Lock()
		_, busy := inFlight[call]
		mu.Unlock()
		if busy {
			refuse(call, refusal("operation.malformed", "the call "+call+" is already in flight on this attachment"))
			continue
		}
		op, served := s.cfg.Operations[name]
		if !served || (op.Answer == nil && op.Stream == nil) {
			refuse(call, refusal("operation.unknown", "this Core does not serve "+name))
			continue
		}
		callCtx, stop := context.WithCancel(ctx)
		mu.Lock()
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
			if op.Answer != nil {
				resp, ref := op.Answer(callCtx, r)
				if ref != nil {
					refuse(call, ref)
					return
				}
				send(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Answer{Answer: resp}})
				return
			}
			ref := op.Stream(callCtx, r, func(resp *interfacev1.Response) error {
				if callCtx.Err() != nil {
					return callCtx.Err()
				}
				return send(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Answer{Answer: resp}})
			})
			if ref != nil {
				refuse(call, ref)
				return
			}
			by := interfacev1.Completion_BY_CORE
			if callCtx.Err() != nil && ctx.Err() == nil {
				by = interfacev1.Completion_BY_CALLER
			}
			send(&interfacev1.CoreFrame{Call: call, Carries: &interfacev1.CoreFrame_Completion{Completion: &interfacev1.Completion{By: by}}})
		}()
	}
}
