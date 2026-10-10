package supervisor

import (
	"context"
	"errors"
	"fmt"

	"github.com/yoke-project/yoke/internal/core/engine"
)

// Inherited clears what an earlier life of the instance left in the engine: every container under the
// instance's label, running or ended, is stopped and removed, and what was removed is returned. An engine
// that cannot be asked, or a container it will not remove, is an error naming it; the trunk makes it fatal.
// Every container found is asked to be removed, whatever became of the others.
func Inherited(ctx context.Context, c Containers, instance string) ([]engine.Found, error) {
	found, err := c.List(ctx, instance)
	if err != nil {
		return nil, err
	}
	var removed []engine.Found
	var failed []error
	for _, f := range found {
		// The removal is forced, which is the stop: what an earlier life left is debris and not state, and
		// nothing is owed a stop window by a Core that never launched it.
		if err := c.Remove(ctx, f.ID); err != nil {
			failed = append(failed, fmt.Errorf("the container %s, unit %s incarnation %d, is not removed: %w", f.ID, f.Unit, f.Incarnation, err))
			continue
		}
		removed = append(removed, f)
	}
	return removed, errors.Join(failed...)
}
