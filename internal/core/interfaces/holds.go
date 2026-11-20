package interfaces

import (
	"slices"
	"sync"
)

// Holds counts, per stream of a unit, the deliveries need's channels have open of it: a stream a unit
// follows through its need is held, and another channel's stop is refused while it is. One Holds is shared
// by every surface of an instance.
type Holds struct {
	mu sync.Mutex
	by map[[2]string]map[string]int
}

// NewHolds is a count that holds nothing.
func NewHolds() *Holds { return &Holds{by: map[[2]string]map[string]int{}} }

func (h *Holds) hold(unit, stream, holder string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := [2]string{unit, stream}
	if h.by[key] == nil {
		h.by[key] = map[string]int{}
	}
	h.by[key][holder]++
}

func (h *Holds) drop(unit, stream, holder string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := [2]string{unit, stream}
	if h.by[key][holder]--; h.by[key][holder] <= 0 {
		delete(h.by[key], holder)
	}
}

// Holders are the units holding a stream of a unit, in the order of their names.
func (h *Holds) Holders(unit, stream string) []string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for holder := range h.by[[2]string{unit, stream}] {
		out = append(out, holder)
	}
	slices.Sort(out)
	return out
}
