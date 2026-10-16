package compose

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// Blocks is the record of the placements made in a project, beside its descriptor and never packaged.
const Blocks = "blocks.yaml"

// block is a fragment of declaration as a block file holds it.
type block struct {
	Name       string    `yaml:"block"`
	Model      *int      `yaml:"model"`
	Unit       yaml.Node `yaml:"unit"`
	Parameters yaml.Node `yaml:"parameters"`
	Channels   yaml.Node `yaml:"channels"`
}

// blockKeys are the keys a block holds, and the only ones.
var blockKeys = map[string]bool{"block": true, "model": true, "unit": true, "parameters": true, "channels": true}

// readBlock reads the block a source names: a file, or a directory holding exactly one. Nothing is
// searched beyond the path. It returns the block, the bytes read and the file they came from.
func readBlock(source string) (*block, []byte, string, error) {
	info, err := os.Stat(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, "", refusef("no block is at %s, and nothing else is searched", source)
	}
	if err != nil {
		return nil, nil, "", err
	}
	file := source
	if info.IsDir() {
		var found []string
		entries, err := os.ReadDir(source)
		if err != nil {
			return nil, nil, "", err
		}
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
				found = append(found, e.Name())
			}
		}
		if len(found) != 1 {
			return nil, nil, "", refusef("the directory %s holds %d blocks, %s, and a source names one", source, len(found), strings.Join(found, ", "))
		}
		file = filepath.Join(source, found[0])
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, "", err
	}
	var top map[string]any
	if err := yaml.Unmarshal(data, &top); err != nil {
		return nil, nil, "", refusef("the block %s does not parse: %v", file, err)
	}
	for k := range top {
		if !blockKeys[k] {
			return nil, nil, "", refusef("the block %s holds %s, and a block holds block, model, unit, parameters and channels", file, k)
		}
	}
	var b block
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, nil, "", refusef("the block %s does not parse: %v", file, err)
	}
	if b.Name == "" || b.Model == nil || b.Unit.Kind != yaml.MappingNode {
		return nil, nil, "", refusef("the block %s names no block, states no model, or holds no unit", file)
	}
	return &b, data, file, nil
}

// placedName is a channel's name once placed: one named after the block's unit is named after the placed
// unit, and any other keeps its name.
func placedName(name, from, to string) string {
	if name == from {
		return to
	}
	if strings.HasPrefix(name, from+"-") {
		return to + name[len(from):]
	}
	return name
}

// AddBlock places the block a source names into the project in dir, as the unit as: the unit under that
// name, its parameter group renamed with it, and its channels following; then records the placement in
// blocks.yaml. The block does not survive the act. Nothing is written unless the gate refuses nothing
// at what the expansion wrote.
func AddBlock(dir, source, as string, now time.Time) error {
	b, data, _, err := readBlock(source)
	if err != nil {
		return err
	}
	if *b.Model != gate.DescriptionModel {
		return refusef("the block %s is written against model %d and the project against model %d; a block is never migrated on the way in", b.Name, *b.Model, gate.DescriptionModel)
	}
	if b.Parameters.Kind == yaml.MappingNode && at(&b.Parameters, "shared") != nil {
		return refusef("the block %s declares parameters into shared, and a block declares into its own group alone", b.Name)
	}
	d := ProjectDocument(dir)
	unit := clone(&b.Unit)
	if exec := at(unit, "exec"); exec != nil && exec.Value != "" {
		program, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(exec.Value)))
		if err != nil {
			return refusef("the program %s of the block %s is not there to be read", exec.Value, b.Name)
		}
		put(unit, "digest", scalar(fmt.Sprintf("sha256:%x", sha256.Sum256(program))))
	}
	written := []string{"units." + as}
	if b.Parameters.Kind == yaml.MappingNode && len(b.Parameters.Content) > 0 {
		written = append(written, "parameters."+as)
	}
	type channel struct {
		name string
		decl *yaml.Node
	}
	var channels []channel
	if b.Channels.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(b.Channels.Content); i += 2 {
			decl := clone(b.Channels.Content[i+1])
			if u := at(decl, "unit"); u != nil && u.Value == b.Name {
				put(decl, "unit", scalar(as))
			}
			name := placedName(b.Channels.Content[i].Value, b.Name, as)
			channels = append(channels, channel{name, decl})
			written = append(written, "channels."+name)
		}
	}
	err = d.editAt(written, func(root *yaml.Node) error {
		units, err := mapping(root, "units")
		if err != nil {
			return err
		}
		chans, err := mapping(root, "channels")
		if err != nil {
			return err
		}
		for _, name := range append([]string{as}, names(channels, func(c channel) string { return c.name })...) {
			if at(units, name) != nil {
				return refusef("the name %s is already a unit's", name)
			}
			if at(chans, name) != nil {
				return refusef("the name %s is already a channel's", name)
			}
		}
		put(units, as, unit)
		if len(b.Parameters.Content) > 0 {
			params, err := mapping(root, "parameters")
			if err != nil {
				return err
			}
			if at(params, as) != nil {
				return refusef("the parameter group %s is already declared", as)
			}
			put(params, as, clone(&b.Parameters))
		}
		for _, c := range channels {
			put(chans, c.name, c.decl)
		}
		if len(chans.Content) == 0 {
			drop(root, "channels")
		}
		return nil
	})
	if err != nil {
		return err
	}
	return record(dir, map[string]string{
		"block":     b.Name,
		"placed_as": as,
		"source":    source,
		"digest":    fmt.Sprintf("sha256:%x", sha256.Sum256(data)),
		"placed_at": now.UTC().Format(time.RFC3339),
	})
}

// unrecord takes the placement made as unit out of blocks.yaml, where there is one: the record says what
// the project holds and where it came from, and the unit is no longer held.
func unrecord(dir, unit string) error {
	path := filepath.Join(dir, Blocks)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return refusef("%s does not parse: %v", path, err)
	}
	if len(doc.Content) == 0 {
		return nil
	}
	list := at(doc.Content[0], "placements")
	if list == nil || list.Kind != yaml.SequenceNode {
		return nil
	}
	kept := list.Content[:0]
	for _, p := range list.Content {
		if as := at(p, "placed_as"); as == nil || as.Value != unit {
			kept = append(kept, p)
		}
	}
	if len(kept) == len(list.Content) {
		return nil
	}
	list.Content = kept
	return writeBlocks(path, &doc)
}

// record appends one placement to blocks.yaml, written whole and then renamed into place.
func record(dir string, placement map[string]string) error {
	path := filepath.Join(dir, Blocks)
	var doc yaml.Node
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return refusef("%s does not parse: %v", path, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	list := at(doc.Content[0], "placements")
	if list == nil {
		list = &yaml.Node{Kind: yaml.SequenceNode}
		put(doc.Content[0], "placements", list)
	}
	entry := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range []string{"block", "placed_as", "source", "digest", "placed_at"} {
		put(entry, k, scalar(placement[k]))
	}
	list.Content = append(list.Content, entry)
	return writeBlocks(path, &doc)
}

// writeBlocks writes blocks.yaml whole, and then renames it into place.
func writeBlocks(path string, doc *yaml.Node) error {
	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	enc.Close()
	tmp := path + ".writing"
	if err := os.WriteFile(tmp, []byte(out.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SaveBlock writes the unit of the project in dir as a block at destination, which must not exist: the
// unit, the parameters of its group and the channels it serves. The project is left untouched.
func SaveBlock(dir, unit, destination string) error {
	data, err := os.ReadFile(filepath.Join(dir, Descriptor))
	if err != nil {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return refusef("the descriptor in %s does not parse as a mapping", dir)
	}
	root := doc.Content[0]
	var u *yaml.Node
	if units := at(root, "units"); units != nil {
		u = at(units, unit)
	}
	if u == nil {
		return refusef("the project declares no unit %s", unit)
	}
	if _, err := os.Lstat(destination); err == nil {
		return refusef("%s is already there, and a block is saved only where nothing is", destination)
	}
	out := &yaml.Node{Kind: yaml.MappingNode}
	put(out, "block", scalar(unit))
	put(out, "model", scalar(fmt.Sprint(gate.DescriptionModel)))
	saved := clone(u)
	drop(saved, "digest")
	put(out, "unit", saved)
	params := &yaml.Node{Kind: yaml.MappingNode}
	if ps := at(root, "parameters"); ps != nil {
		if g := at(ps, unit); g != nil {
			params = clone(g)
		}
	}
	put(out, "parameters", params)
	chans := &yaml.Node{Kind: yaml.MappingNode}
	if cs := at(root, "channels"); cs != nil {
		var named []string
		for i := 0; i+1 < len(cs.Content); i += 2 {
			if s := at(cs.Content[i+1], "unit"); s != nil && s.Value == unit {
				named = append(named, cs.Content[i].Value)
			}
		}
		sort.Strings(named)
		for _, n := range named {
			put(chans, n, clone(at(cs, n)))
		}
	}
	put(out, "channels", chans)
	var b strings.Builder
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{out}}); err != nil {
		return err
	}
	enc.Close()
	f, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// clone is a deep copy of a node, so a block's declarations are written into a document and never shared
// with what was read.
func clone(n *yaml.Node) *yaml.Node {
	c := *n
	c.Content = nil
	for _, k := range n.Content {
		c.Content = append(c.Content, clone(k))
	}
	return &c
}

// drop removes key from a mapping.
func drop(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

func names[T any](items []T, name func(T) string) []string {
	var out []string
	for _, i := range items {
		out = append(out, name(i))
	}
	return out
}
