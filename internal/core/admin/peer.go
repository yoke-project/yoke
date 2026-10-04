package admin

import (
	"context"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/peer"
)

// hostAccounts resolves an account's number through the host's account database.
var hostAccounts = peer.HostAccounts

// actor is the operator the channel of ctx established: the account's name, or its number marked
// unresolved where no name resolves.
func (s *Surface) actor(ctx context.Context) event.Actor {
	if name, ok := peer.Account(ctx, s.accounts); ok {
		return event.Actor{Class: event.ByOperator, Person: name}
	}
	return event.Actor{Class: event.ByOperator}
}
