package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// Change is one field a step changed: its path, and its value before and after, empty where absent.
type Change struct {
	Path, Old, New string
}

// Left is one place a step could not carry, and what a person has to decide there.
type Left struct {
	Path, What string
}

// A step carries a descriptor from one model to the next. It changes how the application is written and
// never what it is, and what it cannot carry mechanically it names rather than guesses.
type step func(root *yaml.Node) ([]Change, []Left)

// composerModels is what this composer knows of the description model: the newest it implements, the
// step from each older one to the next, and how a result is validated against the newest.
type composerModels struct {
	newest   int
	steps    map[int]step
	validate func(descriptor string) gate.Report
}

// models is this composer's. With one model the chain has no steps.
var models = composerModels{
	newest: gate.DescriptionModel,
	steps:  map[int]step{},
	validate: func(descriptor string) gate.Report {
		r, _ := gate.Check(gate.Input{Document: gate.Read(descriptor, gate.Descriptor), Moment: gate.Composing, Manifests: filepath.Join(filepath.Dir(descriptor), Manifests)})
		return r
	},
}

// Migration is what a migration did: the models it carried between, every field it changed, and every
// place it left for a person.
type Migration struct {
	From, To int
	Changed  []Change
	Left     []Left
}

// ToNewest asks Migrate for the newest model this composer implements.
const ToNewest = -1

// Migrate carries the descriptor of the project in dir to the model to, or to the newest this composer
// implements where to is ToNewest, one step per model in order. The result is kept only where no place was
// left and the gate refuses nothing at or beneath a field a step changed, which would be a defect in the
// step; otherwise the project is left as it was. What the gate finds elsewhere is the project's business,
// as it is for an authoring verb. Nothing beside the
// descriptor is touched.
func Migrate(dir string, to int) (Migration, error) {
	m := models
	path := filepath.Join(dir, Descriptor)
	before, err := os.ReadFile(path)
	if err != nil {
		return Migration{}, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(before, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return Migration{}, refusef("the descriptor in %s does not parse as a mapping", dir)
	}
	root := doc.Content[0]
	stated := at(root, "model")
	if stated == nil {
		return Migration{}, refusef("the descriptor states no model, so there is nothing to carry it from")
	}
	from, err := strconv.Atoi(stated.Value)
	if err != nil {
		return Migration{}, refusef("the descriptor's model %q is not a number", stated.Value)
	}
	if to == ToNewest {
		to = m.newest
	}
	switch {
	case to > m.newest:
		return Migration{}, refusef("the newest model this composer implements is %d, and %d is not one it can carry to", m.newest, to)
	case from > m.newest:
		return Migration{}, refusef("the descriptor is written against model %d, later than the newest this composer implements, %d", from, m.newest)
	case from != m.newest && m.steps[from] == nil:
		return Migration{}, refusef("no step carries from model %d, so a descriptor written against it is not carried", from)
	case to < from:
		return Migration{}, refusef("the descriptor is at model %d, and a migration to %d would carry it backwards", from, to)
	}
	out := Migration{From: from, To: to}
	for n := from; n < to; n++ {
		if m.steps[n] == nil {
			return Migration{}, refusef("no step carries from model %d, so a descriptor written against it is not carried", n)
		}
	}
	for n := from; n < to; n++ {
		changed, left := m.steps[n](root)
		out.Changed = append(out.Changed, changed...)
		out.Left = append(out.Left, left...)
	}
	if len(out.Left) > 0 {
		return out, refusef("%d places cannot be carried and are left for a person; the project is unchanged", len(out.Left))
	}
	if from == to {
		return out, nil
	}
	put(root, "model", scalar(strconv.Itoa(to)))
	out.Changed = append(out.Changed, Change{Path: "model", Old: strconv.Itoa(from), New: strconv.Itoa(to)})
	var text strings.Builder
	enc := yaml.NewEncoder(&text)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return out, err
	}
	enc.Close()
	tmp, err := os.CreateTemp(dir, ".migrate-*.yaml")
	if err != nil {
		return out, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text.String()); err != nil {
		tmp.Close()
		return out, err
	}
	tmp.Close()
	var refused []string
	for _, f := range m.validate(tmp.Name()).Findings {
		if f.Class == gate.Refusal && (f.Location == "" || slices.ContainsFunc(out.Changed, func(c Change) bool {
			return f.Location == c.Path || strings.HasPrefix(f.Location, c.Path+".")
		})) {
			refused = append(refused, fmt.Sprintf("%s at %s: %s", f.Code, f.Location, f.Message))
		}
	}
	if len(refused) > 0 {
		return out, refusef("the gate refuses the result against model %d, a defect in the step, and the project is unchanged: %s", to, strings.Join(refused, "; "))
	}
	if info, err := os.Stat(path); err == nil {
		os.Chmod(tmp.Name(), info.Mode().Perm())
	}
	return out, os.Rename(tmp.Name(), path)
}

// report prints what a migration changed and left, in the order the steps produced them.
func (m Migration) report(w interface{ Write([]byte) (int, error) }) {
	shown := func(v string) string {
		if v == "" {
			return "(absent)"
		}
		return v
	}
	for _, c := range m.Changed {
		fmt.Fprintf(w, "changed %s: %s -> %s\n", c.Path, shown(c.Old), shown(c.New))
	}
	for _, l := range m.Left {
		fmt.Fprintf(w, "left %s: %s\n", l.Path, l.What)
	}
}
