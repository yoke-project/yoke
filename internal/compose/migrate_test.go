package compose

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// withSteps replaces the composer's chain for the length of a test.
func withSteps(t *testing.T, c composerModels) {
	t.Helper()
	was := models
	models = c
	t.Cleanup(func() { models = was })
}

// rename is a step moving the value under one top-level key to another.
func rename(from, to string) step {
	return func(root *yaml.Node) ([]Change, []Left) {
		v := at(root, from)
		if v == nil {
			return nil, nil
		}
		drop(root, from)
		put(root, to, v)
		return []Change{{Path: from, Old: v.Value}, {Path: to, New: v.Value}}, nil
	}
}

// rewritten is the descriptor in dir with the replacements given applied.
func rewritten(t *testing.T, dir string, replace ...string) {
	t.Helper()
	file := filepath.Join(dir, Descriptor)
	data, _ := os.ReadFile(file)
	s := string(data)
	for i := 0; i+1 < len(replace); i += 2 {
		if !strings.Contains(s, replace[i]) {
			t.Fatalf("the descriptor holds no %q:\n%s", replace[i], s)
		}
		s = strings.Replace(s, replace[i], replace[i+1], 1)
	}
	os.WriteFile(file, []byte(s), 0o644)
}

// std: yoke:migrate.01
func TestWithOneModelTheChainHasNoSteps(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	before, _ := os.ReadFile(file)
	for _, args := range [][]string{{"migrate"}, {"migrate", "--to", "1"}} {
		code, out := run(append(args, "--dir", dir)...)
		if code != Done || !strings.Contains(out, "already at model 1") {
			t.Errorf("%v: %d %s", args, code, out)
		}
	}
	for _, c := range []struct {
		to   string
		want string
	}{{"2", "newest model this composer implements is 1"}, {"0", "backwards"}} {
		code, out := run("migrate", "--to", c.to, "--dir", dir)
		if code != Refused || !strings.Contains(out, c.want) {
			t.Errorf("--to %s: %d %s", c.to, code, out)
		}
	}
	if after, _ := os.ReadFile(file); !bytes.Equal(before, after) {
		t.Error("the descriptor changed")
	}
	older := authored(t)
	rewritten(t, older, "model: 1\n", "model: 0\n")
	unstated := authored(t)
	rewritten(t, unstated, "model: 1\n", "")
	for _, c := range []struct {
		dir  string
		want string
	}{{older, "no step carries from model 0"}, {unstated, "states no model"}} {
		before, _ := os.ReadFile(filepath.Join(c.dir, Descriptor))
		code, out := run("migrate", "--dir", c.dir)
		if code != Refused || !strings.Contains(out, c.want) {
			t.Errorf("%s: %d %s", c.want, code, out)
		}
		if after, _ := os.ReadFile(filepath.Join(c.dir, Descriptor)); !bytes.Equal(before, after) {
			t.Errorf("%s: the descriptor changed", c.want)
		}
	}
}

// std: yoke:migrate.02
func TestTheStepsAreAppliedInOrder(t *testing.T) {
	withSteps(t, composerModels{
		newest: 3,
		steps: map[int]step{
			1: rename("core", "engine"),
			2: func(root *yaml.Node) ([]Change, []Left) {
				v := at(root, "engine")
				split := &yaml.Node{Kind: yaml.MappingNode}
				put(split, "range", scalar(v.Value))
				put(split, "channel", scalar("stable"))
				put(root, "engine", split)
				return []Change{{Path: "engine", Old: v.Value}, {Path: "engine.range", New: v.Value}, {Path: "engine.channel", New: "stable"}}, nil
			},
		},
		validate: func(string) gate.Report { return gate.Report{} },
	})
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	core := written(t, file, "core")
	code, out := run("migrate", "--to", "2", "--dir", dir)
	if code != Done || written(t, file, "model") != 2 || written(t, file, "engine") != core || written(t, file, "core") != nil {
		t.Fatalf("--to 2: %d %s", code, out)
	}
	for _, line := range []string{"changed core: " + core.(string) + " -> (absent)", "changed engine: (absent) -> " + core.(string), "changed model: 1 -> 2"} {
		if !strings.Contains(out, line) {
			t.Errorf("--to 2 does not report %q:\n%s", line, out)
		}
	}
	code, out = run("migrate", "--dir", dir)
	if code != Done || written(t, file, "model") != 3 || written(t, file, "engine.range") != core || written(t, file, "engine.channel") != "stable" {
		t.Fatalf("to the newest: %d %s", code, out)
	}
	for _, line := range []string{"changed engine.range: (absent) -> " + core.(string), "changed engine.channel: (absent) -> stable", "changed model: 2 -> 3"} {
		if !strings.Contains(out, line) {
			t.Errorf("to the newest does not report %q:\n%s", line, out)
		}
	}
}

// std: yoke:migrate.03
func TestTheResultIsValidatedAndWhatCannotBeCarriedIsNamed(t *testing.T) {
	older := func() string {
		dir := advisedProject(t)
		rewritten(t, dir, "model: 1\n", "model: 0\n", "core:", "engine:")
		os.WriteFile(filepath.Join(dir, Blocks), []byte("placements: []\n"), 0o644)
		os.WriteFile(filepath.Join(dir, Advisories), []byte("waived: []\n"), 0o644)
		return dir
	}
	with := func(s step) {
		withSteps(t, composerModels{newest: gate.DescriptionModel, steps: map[int]step{0: s}, validate: models.validate})
	}

	with(rename("engine", "core"))
	dir := older()
	code, out := run("migrate", "--dir", dir)
	if code != Done {
		t.Fatalf("the step that carries: %d %s", code, out)
	}
	for _, line := range []string{"changed engine: ", "changed core: (absent) -> ", "changed model: 0 -> 1"} {
		if !strings.Contains(out, line) {
			t.Errorf("does not report %q:\n%s", line, out)
		}
	}
	if code, out := run("check", "--dir", dir); code != Done {
		t.Errorf("check after the migration: %d %s", code, out)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, Blocks)); string(data) != "placements: []\n" {
		t.Error("blocks.yaml was touched")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, Advisories)); string(data) != "waived: []\n" {
		t.Error("advisories.yaml was touched")
	}
	if packaged, _ := filepath.Glob(filepath.Join(dir, "*.yoke")); len(packaged) > 0 {
		t.Errorf("a package was built: %v", packaged)
	}

	for _, c := range []struct {
		step step
		want []string
	}{
		{rename("engine", "kore"), []string{"kore", "defect in the step"}},
		{func(root *yaml.Node) ([]Change, []Left) {
			return nil, []Left{{Path: "version", What: "whether this is a new version"}, {Path: "arch", What: "which architecture is meant"}}
		}, []string{"left version: whether this is a new version", "left arch: which architecture is meant", "unchanged"}},
	} {
		with(c.step)
		dir := older()
		before, _ := os.ReadFile(filepath.Join(dir, Descriptor))
		code, out := run("migrate", "--dir", dir)
		if code != Refused {
			t.Errorf("%v: exited %d: %s", c.want, code, out)
		}
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("does not name %q:\n%s", w, out)
			}
		}
		if after, _ := os.ReadFile(filepath.Join(dir, Descriptor)); !bytes.Equal(before, after) {
			t.Errorf("%v: the descriptor changed", c.want)
		}
	}
}
