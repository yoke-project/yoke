// Package scope is what a unit may do once admitted: for each kind of object a capability governs —
// a stream, a command, a query, an occurrence — what its Manifest declares and what its grant allows.
//
// It is computed once, at admission, and read for the rest of the Session. A check is exact
// membership: no prefix, no case folding and no hierarchy is evaluated when a message is checked.
package scope

import pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"

// Kind is a kind of object a capability governs.
type Kind string

// The four kinds a message can name.
const (
	Stream     Kind = "stream"
	Command    Kind = "command"
	Query      Kind = "query"
	Occurrence Kind = "occurrence"
)

// Scope is the declared and the granted, by kind. Its zero value declares and grants nothing.
type Scope struct {
	declared, granted map[Kind]map[string]bool
}

// Declare records an object the Manifest declares.
func (s *Scope) Declare(k Kind, id string) { s.declared = add(s.declared, k, id) }

// Grant records an object the grant allows.
func (s *Scope) Grant(k Kind, id string) { s.granted = add(s.granted, k, id) }

func add(m map[Kind]map[string]bool, k Kind, id string) map[Kind]map[string]bool {
	if m == nil {
		m = map[Kind]map[string]bool{}
	}
	if m[k] == nil {
		m[k] = map[string]bool{}
	}
	m[k][id] = true
	return m
}

// Check is CODE_UNSPECIFIED for an object granted, scope.withheld for one declared and not granted, and
// scope.undeclared for one never declared.
func (s *Scope) Check(k Kind, id string) pluginv1.Code {
	switch {
	case s != nil && s.granted[k][id]:
		return pluginv1.Code_CODE_UNSPECIFIED
	case s != nil && s.declared[k][id]:
		return pluginv1.Code_CODE_SCOPE_WITHHELD
	default:
		return pluginv1.Code_CODE_SCOPE_UNDECLARED
	}
}
