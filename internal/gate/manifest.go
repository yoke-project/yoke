package gate

import (
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"
)

// ManifestModel is the version of the Manifest model this host implements.
const ManifestModel = 1

// Protocols are the versions of the plugin surface this Core speaks: the version of the contract the
// definitions carry.
var Protocols = []int{int(pluginv1.Contract_CONTRACT_VERSION)}

// Manifest is what a Plugin declares about itself before it runs. Everything in it is a claim, and a
// claim grants nothing.
type Manifest struct {
	Path         string
	ID           string
	Protocol     int
	Needs        []Need
	Streams      []Stream
	Commands     []string
	Queries      []string
	Occurrences  []string
	Capabilities []Capability
}

// Stream is a stream the Plugin may publish. Both tolerances default to false, as a written value.
type Stream struct {
	ID               string
	ToleratesLoss    bool
	ToleratesReorder bool
}

// Capability is what an operator may grant, governing exactly one object.
type Capability struct {
	Name    string
	Governs Object
}

// Object is what a capability governs: a stream, a command, a query, an occurrence or a surface.
type Object struct {
	Kind string
	ID   string
}

// removed are the fields of an earlier model, refused rather than ignored, with where each statement
// lives now.
var removed = map[string]string{
	"endpoint":  "the socket path is derived from the unit's identity",
	"autostart": "whether a copy comes up with its deployment is said on the unit, in the composing document",
	"digest":    "the expected identity of the executable is in the descriptor, or the system's",
}

// CheckManifest checks a Manifest on its own and returns the report and, when nothing was refused, what
// it declares.
func CheckManifest(doc Document) (Report, *Manifest) {
	c := &checker{doc: doc}
	c.run([]phase{
		{PhaseReading, c.readingManifest},
		{PhaseShape, c.manifestShape},
		{PhaseInternalJoins, c.manifestJoins},
		{PhaseCrossDocument, notGiven},
		{PhaseHostFacts, notGiven},
		{PhaseWeaker, func() bool { return true }},
	})
	if c.report.Refused() {
		return c.report, nil
	}
	return c.report, c.manifest
}

// readingManifest reads the document and its `manifest` version first: a model this host does not
// implement ends the read before anything else is interpreted.
func (c *checker) readingManifest() bool {
	c.reading()
	if c.report.Refused() {
		return true
	}
	if !c.root.isMapping() {
		c.refuse("field.type", "", "the Manifest is %s, not a mapping", c.root.text())
		return true
	}
	for _, e := range c.root.entries() {
		if e.key != "manifest" {
			continue
		}
		if !e.value.isScalar() || !isInteger(e.value.n.Value) {
			c.refuse("field.type", "manifest", "manifest is %q, not an integer", e.value.text())
			return true
		}
		if n, _ := strconv.Atoi(e.value.n.Value); n != ManifestModel {
			c.refuse("plugin.manifest.model", "manifest", "the Manifest is written against model %d, and this host implements %d: it is refused rather than interpreted", n, ManifestModel)
		}
		return true
	}
	c.refuse("field.required", "manifest", "the Manifest does not say which model it is written against")
	return true
}

// expectedID is the identity the path a Manifest was read from names.
func expectedID(path string) string {
	base := filepath.Base(path)
	if base == "manifest.yaml" {
		return filepath.Base(filepath.Dir(path))
	}
	return strings.TrimSuffix(base, ".yaml")
}

func (c *checker) manifestShape() bool {
	m := &Manifest{Path: c.doc.Path}
	c.manifest = m
	fields := map[string]*node{}
	for _, e := range c.root.entries() {
		if where, gone := removed[e.key]; gone {
			c.refuse("manifest.field.removed", e.key, "%s belongs to an earlier model and is refused: %s", e.key, where)
			continue
		}
		if !slices.Contains([]string{"manifest", "id", "protocol", "needs", "streams", "commands", "queries", "occurrences", "capabilities"}, e.key) {
			c.refuse("key.unknown", e.key, "the key %s is not part of a Manifest", e.key)
			continue
		}
		fields[e.key] = e.value
	}
	if v, there := fields["id"]; !there {
		c.refuse("field.required", "id", "the Manifest names no plugin")
	} else if id, ok := c.str(v, "id", false); ok {
		if !identifier.MatchString(id) {
			c.refuse("field.value", "id", "the id %q is not a namespaced identifier of dot-separated lowercase labels", id)
		} else if want := expectedID(c.doc.Path); want != id {
			c.refuse("plugin.manifest.id", "id", "the Manifest says it is %s, and the path it was read from names %s", id, want)
		}
		m.ID = id
	}
	if v, there := fields["protocol"]; !there {
		c.refuse("field.required", "protocol", "the Manifest does not say which protocol its binary speaks")
	} else if s, ok := c.str(v, "protocol", false); ok {
		if !isInteger(s) {
			c.refuse("field.type", "protocol", "protocol is %q, not an integer", s)
		} else if p, _ := strconv.Atoi(s); !slices.Contains(Protocols, p) {
			c.refuse("plugin.protocol.unsupported", "protocol", "the binary speaks protocol %d, and this Core speaks %v", p, Protocols)
		} else {
			m.Protocol = p
		}
	}
	if v, there := fields["needs"]; there {
		if needs, ok := c.strings(v, "needs", false); ok {
			for i, written := range needs {
				if need, ok := c.need(written, fmt.Sprintf("needs.%d", i)); ok {
					m.Needs = append(m.Needs, need)
				}
			}
		}
	}
	list := func(key string, extra []string, read func(entry map[string]*node, at string)) {
		v, there := fields[key]
		if !there {
			return
		}
		if !v.isSequence() {
			c.refuse("field.type", key, "%s is %s, not a list", key, v.text())
			return
		}
		for i, item := range v.items() {
			at := fmt.Sprintf("%s.%d", key, i)
			if !c.mapping(item, at) {
				continue
			}
			entry := map[string]*node{}
			for _, e := range item.entries() {
				if !slices.Contains(append([]string{"id"}, extra...), e.key) {
					c.refuse("key.unknown", joined(at, e.key), "the key %s is not part of an entry of %s", e.key, key)
					continue
				}
				entry[e.key] = e.value
			}
			read(entry, at)
		}
	}
	id := func(entry map[string]*node, at string) (string, bool) {
		v, there := entry["id"]
		if !there {
			c.refuse("field.required", joined(at, "id"), "the entry has no id")
			return "", false
		}
		return c.str(v, joined(at, "id"), false)
	}
	list("streams", []string{"tolerates_loss", "tolerates_reorder"}, func(entry map[string]*node, at string) {
		s := Stream{}
		if v, ok := id(entry, at); ok {
			if !nameOK(v) || v == "." || v == ".." {
				c.refuse("name.not_a_path_component", joined(at, "id"), "the stream %q is not a path component: no separator, no null byte, neither . nor ..", v)
			}
			s.ID = v
		}
		if v, there := entry["tolerates_loss"]; there {
			s.ToleratesLoss, _ = c.boolean(v, joined(at, "tolerates_loss"))
		}
		if v, there := entry["tolerates_reorder"]; there {
			s.ToleratesReorder, _ = c.boolean(v, joined(at, "tolerates_reorder"))
		}
		m.Streams = append(m.Streams, s)
	})
	for key, into := range map[string]*[]string{"commands": &m.Commands, "queries": &m.Queries, "occurrences": &m.Occurrences} {
		list(key, nil, func(entry map[string]*node, at string) {
			if v, ok := id(entry, at); ok {
				*into = append(*into, v)
			}
		})
	}
	if v, there := fields["capabilities"]; there {
		if !v.isSequence() {
			c.refuse("field.type", "capabilities", "capabilities is %s, not a list", v.text())
		} else {
			for i, item := range v.items() {
				if capability, ok := c.capability(item, fmt.Sprintf("capabilities.%d", i)); ok {
					m.Capabilities = append(m.Capabilities, capability)
				}
			}
		}
	}
	return true
}

func (c *checker) capability(n *node, at string) (Capability, bool) {
	capability := Capability{}
	if !c.mapping(n, at) {
		return capability, false
	}
	var governs *node
	named := false
	for _, e := range n.entries() {
		switch e.key {
		case "name":
			capability.Name, named = c.str(e.value, joined(at, "name"), false)
		case "governs":
			governs = e.value
		case "description", "label":
			c.refuse("manifest.field.removed", joined(at, e.key), "a capability's %s is refused: what a permission means is written in the shared vocabulary, not by the party it constrains", e.key)
		default:
			c.refuse("key.unknown", joined(at, e.key), "the key %s is not part of a capability", e.key)
		}
	}
	if _, there := findKey(n, "name"); !there {
		c.refuse("field.required", joined(at, "name"), "the capability has no name")
	}
	if governs == nil {
		c.refuse("field.required", joined(at, "governs"), "the capability governs nothing")
		return capability, false
	}
	if !c.mapping(governs, joined(at, "governs")) {
		return capability, false
	}
	var objects []Object
	for _, e := range governs.entries() {
		if !slices.Contains([]string{"stream", "command", "query", "occurrence", "surface"}, e.key) {
			c.refuse("key.unknown", joined(joined(at, "governs"), e.key), "the key %s is not an object a capability governs", e.key)
			continue
		}
		if v, ok := c.str(e.value, joined(joined(at, "governs"), e.key), false); ok {
			objects = append(objects, Object{Kind: e.key, ID: v})
		}
	}
	if len(objects) != 1 {
		c.refuse("capability.governs.count", joined(at, "governs"), "the capability %s governs %d objects, and a capability governs exactly one", capability.Name, len(objects))
		return capability, false
	}
	capability.Governs = objects[0]
	return capability, named
}

func findKey(n *node, key string) (*node, bool) {
	for _, e := range n.entries() {
		if e.key == key {
			return e.value, true
		}
	}
	return nil, false
}

// manifestJoins joins the capabilities to the four lists in both directions.
func (c *checker) manifestJoins() bool {
	m := c.manifest
	declared := map[Object]bool{}
	var order []Object
	where := map[Object]string{}
	add := func(kind, list string, ids []string) {
		for i, id := range ids {
			o := Object{kind, id}
			declared[o] = true
			order = append(order, o)
			where[o] = fmt.Sprintf("%s.%d", list, i)
		}
	}
	var streams []string
	for _, s := range m.Streams {
		streams = append(streams, s.ID)
	}
	add("stream", "streams", streams)
	add("command", "commands", m.Commands)
	add("query", "queries", m.Queries)
	add("occurrence", "occurrences", m.Occurrences)
	governed := map[Object]bool{}
	for i, capability := range m.Capabilities {
		o := capability.Governs
		governed[o] = true
		if o.Kind != "surface" && !declared[o] {
			c.refuse("capability.governs.undeclared", fmt.Sprintf("capabilities.%d.governs.%s", i, o.Kind), "the capability %s governs the %s %s, which no list declares", capability.Name, o.Kind, o.ID)
		}
	}
	for _, o := range order {
		if !governed[o] {
			c.refuse("capability.ungoverned", where[o], "the %s %s is declared and no capability governs it", o.Kind, o.ID)
		}
	}
	return true
}
