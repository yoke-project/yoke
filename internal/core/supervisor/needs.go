package supervisor

// expansion is what a launch's needs expand into: the variables handed, and what a container is given.
type expansion struct {
	env     []string
	devices []string
	mounts  []string
	groups  []int
	network bool
}

// expand expands a unit's needs for one launch. A need that cannot be expanded is an error naming it.
func (s *Supervisor) expand(u Unit, contained bool) (expansion, error) {
	return expansion{}, nil
}
