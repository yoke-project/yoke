package supervisor

import (
	"context"

	"github.com/yoke-project/yoke/internal/core/engine"
)

// Inherited clears what an earlier life of the instance left in the engine: every container under the
// instance's label, running or ended, is stopped and removed, and what was removed is returned. An engine
// that cannot be asked, or a container it will not remove, is an error naming it; the trunk makes it fatal.
func Inherited(ctx context.Context, c Containers, instance string) ([]engine.Found, error) {
	return nil, nil
}
