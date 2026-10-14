package compose

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// Document is what an authoring verb writes: a project's descriptor, or a composition document named by
// its path, checked against the Manifests in a directory.
type Document struct {
	Path      string
	Kind      gate.Kind
	Manifests string
	// Project is the project's directory, where the document is its descriptor: programs named under
	// `components/` are read there for their digest.
	Project string
}

// ProjectDocument is the descriptor of the project in dir.
func ProjectDocument(dir string) Document {
	return Document{Path: filepath.Join(dir, Descriptor), Kind: gate.Descriptor, Manifests: filepath.Join(dir, Manifests), Project: dir}
}

// CompositionDocument is the composition document at path, checked against the Manifests in manifests.
func CompositionDocument(path, manifests string) Document {
	return Document{Path: path, Kind: gate.Composition, Manifests: manifests}
}

// edit applies change to the document and keeps the result only where the gate refuses nothing at or
// beneath the path written: a verb never accepts what the gate refuses, and a refused write leaves the
// document byte for byte as it was. What the gate finds elsewhere is the rest of the project's business.
func (d Document) edit(written string, change func(root *yaml.Node) error) error {
	before, err := os.ReadFile(d.Path)
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(before, &doc); err != nil {
		return refusef("%s does not parse: %v", d.Path, err)
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return refusef("%s is not a mapping", d.Path)
	}
	if err := change(doc.Content[0]); err != nil {
		return err
	}
	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	enc.Close()
	tmp, err := os.CreateTemp(filepath.Dir(d.Path), ".edit-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(out.String()); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	r, _ := gate.Check(gate.Input{Document: gate.Read(tmp.Name(), d.Kind), Moment: gate.Composing, Manifests: d.Manifests})
	var refused []string
	for _, f := range r.Findings {
		if f.Class == gate.Refusal && (f.Location == written || strings.HasPrefix(f.Location, written+".")) {
			refused = append(refused, fmt.Sprintf("%s at %s: %s", f.Code, f.Location, f.Message))
		}
	}
	if len(refused) > 0 {
		return refusef("%s is not written: %s", written, strings.Join(refused, "; "))
	}
	if info, err := os.Stat(d.Path); err == nil {
		os.Chmod(tmp.Name(), info.Mode().Perm())
	}
	return os.Rename(tmp.Name(), d.Path)
}

// at finds the value under key in a mapping, or nil.
func at(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// put writes value under key in a mapping, replacing what was there.
func put(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}

// mapping is the mapping under key, made where it is absent.
func mapping(m *yaml.Node, key string) (*yaml.Node, error) {
	v := at(m, key)
	if v == nil {
		v = &yaml.Node{Kind: yaml.MappingNode}
		put(m, key, v)
	}
	if v.Kind != yaml.MappingNode {
		return nil, refusef("%s is not a mapping", key)
	}
	return v, nil
}

func scalar(s string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Value: s} }

func sequence(items []string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	for _, s := range items {
		n.Content = append(n.Content, scalar(s))
	}
	return n
}

// value reads a value as YAML: a list, a mapping or a scalar as written.
func value(s string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(s), &doc); err != nil || len(doc.Content) == 0 {
		return scalar(s), nil
	}
	return doc.Content[0], nil
}

// Field is the field of the description model a path names, as the gate's schema writes it, or empty.
func Field(kind gate.Kind, path string) string {
	parts := strings.Split(path, ".")
	for _, f := range gate.Fields(kind) {
		pattern := strings.Split(f, ".")
		if len(pattern) != len(parts) {
			continue
		}
		match := true
		for i := range pattern {
			if pattern[i] != "*" && pattern[i] != parts[i] || parts[i] == "" {
				match = false
				break
			}
		}
		if match {
			return f
		}
	}
	return ""
}

// Set writes value at path, which names a field by the path a finding carries.
func Set(d Document, path, raw string) error {
	if Field(d.Kind, path) == "" {
		return refusef("the description model has no field at %s", path)
	}
	v, err := value(raw)
	if err != nil {
		return err
	}
	parts := strings.Split(path, ".")
	return d.edit(path, func(root *yaml.Node) error {
		m := root
		if parts[0] == "arbitration" {
			// A rule is named by its place in the list, and is written by arbitrate before a field of it is.
			rules := at(root, "arbitration")
			n, err := strconv.Atoi(parts[1])
			if err != nil || rules == nil || rules.Kind != yaml.SequenceNode || n < 0 || n >= len(rules.Content) {
				return refusef("there is no rule %s in arbitration", parts[1])
			}
			m, parts = rules.Content[n], parts[2:]
		}
		for _, p := range parts[:len(parts)-1] {
			next, err := mapping(m, p)
			if err != nil {
				return err
			}
			m = next
		}
		put(m, parts[len(parts)-1], v)
		return nil
	})
}

// UnitOptions are what `add unit` writes besides the name.
type UnitOptions struct {
	Kind, Exec, Image, Plugin string
	Args, DependsOn           []string
}

// AddUnit writes a unit declaration. A program under `components/` of a project is read for its digest,
// which is written beside it.
func AddUnit(d Document, name string, o UnitOptions) error {
	var digest string
	if o.Exec != "" && d.Project != "" {
		data, err := os.ReadFile(filepath.Join(d.Project, filepath.FromSlash(o.Exec)))
		if err != nil {
			return refusef("the program %s of the unit %s is not there to be read", o.Exec, name)
		}
		digest = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	}
	return d.edit("units."+name, func(root *yaml.Node) error {
		units, err := mapping(root, "units")
		if err != nil {
			return err
		}
		if at(units, name) != nil {
			return refusef("the unit %s is declared", name)
		}
		u := &yaml.Node{Kind: yaml.MappingNode}
		put(u, "kind", scalar(o.Kind))
		for _, kv := range [][2]string{{"plugin", o.Plugin}, {"exec", o.Exec}, {"digest", digest}, {"image", o.Image}} {
			if kv[1] != "" {
				put(u, kv[0], scalar(kv[1]))
			}
		}
		if len(o.Args) > 0 {
			put(u, "args", sequence(o.Args))
		}
		if len(o.DependsOn) > 0 {
			put(u, "depends_on", sequence(o.DependsOn))
		}
		put(units, name, u)
		return nil
	})
}

// ChannelOptions are what `add channel` writes besides the name.
type ChannelOptions struct {
	Transport, Clients, Unit, OnSuspend string
	Class, Host, Port                   string
}

// AddChannel writes a channel. A routable address is written with both security fields, which are the
// only values it may state.
func AddChannel(d Document, name string, o ChannelOptions) error {
	return d.edit("channels."+name, func(root *yaml.Node) error {
		channels, err := mapping(root, "channels")
		if err != nil {
			return err
		}
		if at(channels, name) != nil {
			return refusef("the channel %s is declared", name)
		}
		c := &yaml.Node{Kind: yaml.MappingNode}
		for _, kv := range [][2]string{{"unit", o.Unit}, {"transport", o.Transport}, {"clients", o.Clients}, {"on_suspend", o.OnSuspend}} {
			if kv[1] != "" {
				put(c, kv[0], scalar(kv[1]))
			}
		}
		if o.Class != "" {
			a := &yaml.Node{Kind: yaml.MappingNode}
			put(a, "class", scalar(o.Class))
			for _, kv := range [][2]string{{"host", o.Host}, {"port", o.Port}} {
				if kv[1] != "" {
					put(a, kv[0], scalar(kv[1]))
				}
			}
			if o.Class == "routable" {
				s := &yaml.Node{Kind: yaml.MappingNode}
				put(s, "transport", scalar("encrypted"))
				put(s, "caller", scalar("authenticated"))
				put(a, "security", s)
			}
			put(c, "address", a)
		}
		put(channels, name, c)
		return nil
	})
}

// ParameterOptions are what `add parameter` writes besides the group and the name.
type ParameterOptions struct {
	Type, Range, Pattern, Default, Label string
	Values                               []string
	Required                             bool
}

// AddParameter writes a parameter declaration, `<group>.<name>`, into a descriptor.
func AddParameter(d Document, qualified string, o ParameterOptions) error {
	group, name, found := strings.Cut(qualified, ".")
	if !found || group == "" || name == "" || strings.Contains(name, ".") {
		return refusef("a parameter is named <group>.<name>, and %q is not", qualified)
	}
	if d.Kind != gate.Descriptor {
		return refusef("a composition document declares no parameters")
	}
	return d.edit("parameters."+qualified, func(root *yaml.Node) error {
		params, err := mapping(root, "parameters")
		if err != nil {
			return err
		}
		g, err := mapping(params, group)
		if err != nil {
			return err
		}
		if at(g, name) != nil {
			return refusef("the parameter %s is declared", qualified)
		}
		p := &yaml.Node{Kind: yaml.MappingNode}
		if o.Type != "" {
			put(p, "type", scalar(o.Type))
		}
		if o.Range != "" {
			r, _ := value(o.Range)
			put(p, "range", r)
		}
		if o.Pattern != "" {
			put(p, "pattern", scalar(o.Pattern))
		}
		if len(o.Values) > 0 {
			put(p, "values", sequence(o.Values))
		}
		put(p, "required", scalar(strconv.FormatBool(o.Required)))
		for _, kv := range [][2]string{{"default", o.Default}, {"label", o.Label}} {
			if kv[1] != "" {
				put(p, kv[0], scalar(kv[1]))
			}
		}
		put(g, name, p)
		return nil
	})
}

// Arbitrate appends one precedence relation: prevails over each of over.
func Arbitrate(d Document, prevails string, over []string) error {
	if len(over) == 0 {
		return refusef("a rule names the channels it prevails over")
	}
	return d.edit("arbitration", func(root *yaml.Node) error {
		rules := at(root, "arbitration")
		if rules == nil {
			rules = &yaml.Node{Kind: yaml.SequenceNode}
			put(root, "arbitration", rules)
		}
		if rules.Kind != yaml.SequenceNode {
			return refusef("arbitration is not a list")
		}
		r := &yaml.Node{Kind: yaml.MappingNode, Style: yaml.FlowStyle}
		put(r, "prevails", scalar(prevails))
		put(r, "over", sequence(over))
		rules.Content = append(rules.Content, r)
		return nil
	})
}
