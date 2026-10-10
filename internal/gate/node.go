package gate

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// node is a parsed YAML node. Every scalar is typed by the schema and never by the notation: `no` is
// the string `no` wherever a string is expected, so what is read is always the text as written.
type node struct{ n *yaml.Node }

func parse(b []byte) (*node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		// An empty document is an empty mapping, which the shape then finds wanting.
		return &node{&yaml.Node{Kind: yaml.MappingNode}}, nil
	}
	if len(doc.Content) != 1 {
		return nil, errors.New("more than one document")
	}
	if err := unique(doc.Content[0]); err != nil {
		return nil, err
	}
	return &node{doc.Content[0]}, nil
}

// unique refuses a mapping that writes a key twice, at any depth: YAML has no such mapping, and the
// parser keeps one of the two without saying which.
func unique(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if seen[key.Value] {
				return fmt.Errorf("the key %q is written twice, the second time at line %d", key.Value, key.Line)
			}
			seen[key.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := unique(child); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) isMapping() bool  { return n.n.Kind == yaml.MappingNode }
func (n *node) isSequence() bool { return n.n.Kind == yaml.SequenceNode }
func (n *node) isScalar() bool   { return n.n.Kind == yaml.ScalarNode && n.n.Tag != "!!null" }

// text is what was written, for a message.
func (n *node) text() string {
	if n.isScalar() {
		return n.n.Value
	}
	switch n.n.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	}
	return "nothing"
}

type entry struct {
	key   string
	value *node
}

// entries are a mapping's pairs, in the order they were written.
func (n *node) entries() []entry {
	var out []entry
	for i := 0; i+1 < len(n.n.Content); i += 2 {
		out = append(out, entry{n.n.Content[i].Value, &node{n.n.Content[i+1]}})
	}
	return out
}

func (n *node) items() []*node {
	var out []*node
	for _, c := range n.n.Content {
		out = append(out, &node{c})
	}
	return out
}

// reference is a substitution: `${name}` or `${bind.<key>}`.
var reference = regexp.MustCompile(`\$\{([^}]*)\}`)

func hasReference(s string) bool { return reference.MatchString(s) }

func isInteger(s string) bool {
	_, err := strconv.ParseUint(s, 10, 63)
	return err == nil
}
