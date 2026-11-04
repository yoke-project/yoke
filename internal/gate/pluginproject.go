package gate

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// PluginProjectModel is the version of the Plugin project's file this composer implements.
const PluginProjectModel = 1

// Families are the languages a Plugin project's file may name, each a family of the project.
var Families = []string{"go", "rust", "python", "c", "cpp"}

// Spelling is one architecture as each ecosystem a Plugin's build may run in writes it.
type Spelling struct {
	Go, Rust, Deb string
}

// Architectures are the architectures a Plugin is built for, in uname names, with their spellings: the
// set the build contract can tell a build, and so the set a Plugin project's file may list.
var Architectures = map[string]Spelling{
	"x86_64":  {Go: "amd64", Rust: "x86_64-unknown-linux-gnu", Deb: "amd64"},
	"aarch64": {Go: "arm64", Rust: "aarch64-unknown-linux-gnu", Deb: "arm64"},
}

// PluginProject is what a Plugin project's file says, and the Manifest it names. Every path is relative
// to the project's root, as written.
type PluginProject struct {
	Path, ID, Language, Program, Manifest string
	BuildIn, BuildRun, BuildContainer     string
	Architectures                         []string
	// Declared is the Manifest the file names, as the gate read it.
	Declared *Manifest
}

// Root is the directory the project's file is at.
func (p *PluginProject) Root() string { return filepath.Dir(p.Path) }

// CheckPluginProject checks a Plugin project's file and the Manifest it names, as a descriptor is checked,
// and returns the report and, when nothing was refused, what the file says.
func CheckPluginProject(doc Document) (Report, *PluginProject) {
	c := &checker{doc: doc}
	var p *PluginProject
	c.run([]phase{
		{PhaseReading, c.readingPluginProject},
		{PhaseShape, func() bool { p = c.pluginProjectShape(); return true }},
		{PhaseInternalJoins, func() bool { c.pluginProjectJoins(p); return true }},
		{PhaseCrossDocument, notGiven},
		{PhaseHostFacts, notGiven},
		{PhaseWeaker, func() bool { return true }},
	})
	if c.report.Refused() {
		return c.report, nil
	}
	return c.report, p
}

// readingPluginProject reads the file and its model first: a model this composer does not implement ends
// the read before anything else is interpreted.
func (c *checker) readingPluginProject() bool {
	c.reading()
	if c.report.Refused() {
		return true
	}
	if !c.root.isMapping() {
		c.refuse("field.type", "", "the Plugin project's file %s is %s, not a mapping", c.doc.Path, c.root.text())
		return true
	}
	for _, e := range c.root.entries() {
		if e.key != "plugin" {
			continue
		}
		if !e.value.isScalar() || !isInteger(e.value.n.Value) {
			c.refuse("field.type", "plugin", "plugin is %q, not an integer", e.value.text())
			return true
		}
		if n, _ := strconv.Atoi(e.value.n.Value); n != PluginProjectModel {
			c.refuse("plugin_project.model", "plugin", "the file is written against model %d, and this composer implements %d: it is refused rather than interpreted", n, PluginProjectModel)
		}
		return true
	}
	c.refuse("field.required", "plugin", "the file does not say which model it is written against")
	return true
}

// inside is a path relative to the project's root that stays inside it, cleaned; ok is false otherwise.
func inside(p string) (string, bool) {
	if p == "" || path.IsAbs(p) || filepath.IsAbs(p) {
		return "", false
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

func (c *checker) pluginProjectShape() *PluginProject {
	p := &PluginProject{Path: c.doc.Path}
	fields := map[string]*node{}
	for _, e := range c.root.entries() {
		switch e.key {
		case "version":
			c.refuse("plugin_project.version", "version", "the file carries no version: a Plugin's version is the tag its source is published at")
		case "plugin", "id", "language", "program", "manifest", "build", "architectures":
			fields[e.key] = e.value
		default:
			c.refuse("key.unknown", e.key, "the key %s is not part of a Plugin project's file", e.key)
		}
	}
	required := func(key string) (*node, bool) {
		n, there := fields[key]
		if !there {
			c.refuse("field.required", key, "the file does not say %s", key)
		}
		return n, there
	}
	if n, there := required("id"); there {
		if id, ok := c.str(n, "id", false); ok {
			if identifier.MatchString(id) {
				p.ID = id
			} else {
				c.refuse("field.value", "id", "the id %q is not a namespaced identifier of dot-separated lowercase labels", id)
			}
		}
	}
	if n, there := required("language"); there {
		if s, ok := c.str(n, "language", false); ok {
			if slices.Contains(Families, s) {
				p.Language = s
			} else {
				c.refuse("field.value", "language", "the language %q is not a family; the families are %s", s, strings.Join(Families, ", "))
			}
		}
	}
	if n, there := required("program"); there {
		if s, ok := c.str(n, "program", false); ok {
			if s == "" || s == "." || s == ".." || strings.ContainsAny(s, `/\`) {
				c.refuse("field.value", "program", "the program %q is not a file name: the build writes it under $YOKE_OUT, with no directory", s)
			} else {
				p.Program = s
			}
		}
	}
	if n, there := required("manifest"); there {
		if s, ok := c.str(n, "manifest", false); ok {
			if _, in := inside(s); in {
				p.Manifest = s
			} else {
				c.refuse("path.outside", "manifest", "the manifest %q is not a path inside the project", s)
			}
		}
	}
	if n, there := required("build"); there {
		c.pluginBuild(n, p)
	} else {
		// The build's own fields are named too, so that the author sees which of them is wanted.
		c.refuse("field.required", "build.in", "the build does not say the directory it runs in")
		c.refuse("field.required", "build.run", "the build does not say the command it runs")
	}
	if n, there := required("architectures"); there {
		c.pluginArchitectures(n, p)
	}
	return p
}

func (c *checker) pluginBuild(n *node, p *PluginProject) {
	if !n.isMapping() {
		c.refuse("field.type", "build", "build is %s, not a mapping", n.text())
		return
	}
	build := map[string]*node{}
	for _, e := range n.entries() {
		if slices.Contains([]string{"in", "run", "container"}, e.key) {
			build[e.key] = e.value
		} else {
			c.refuse("key.unknown", "build."+e.key, "the key build.%s is not part of a Plugin's build", e.key)
		}
	}
	if v, there := build["in"]; !there {
		c.refuse("field.required", "build.in", "the build does not say the directory it runs in")
	} else if s, ok := c.str(v, "build.in", false); ok {
		if _, in := inside(s); in {
			p.BuildIn = s
		} else {
			c.refuse("path.outside", "build.in", "build.in %q is not a directory inside the project", s)
		}
	}
	if v, there := build["run"]; !there {
		c.refuse("field.required", "build.run", "the build does not say the command it runs")
	} else if s, ok := c.str(v, "build.run", true); ok {
		if strings.TrimSpace(s) == "" {
			c.refuse("field.value", "build.run", "build.run is empty")
		} else {
			p.BuildRun = s
		}
	}
	if v, there := build["container"]; there {
		if s, ok := c.str(v, "build.container", false); ok && s != "" {
			if pinnedImage.MatchString(s) {
				p.BuildContainer = s
			} else {
				c.refuse("unit.image.tagged", "build.container", "the image %q is not referenced by its digest", s)
			}
		}
	}
}

func (c *checker) pluginArchitectures(n *node, p *PluginProject) {
	list, ok := c.strings(n, "architectures", false)
	if !ok {
		return
	}
	if len(list) == 0 {
		c.refuse("field.value", "architectures", "architectures lists none: a Plugin is built for at least one")
	}
	known := make([]string, 0, len(Architectures))
	for a := range Architectures {
		known = append(known, a)
	}
	slices.Sort(known)
	seen := map[string]bool{}
	for _, a := range list {
		switch _, spelt := Architectures[a]; {
		case seen[a]:
			c.refuse("field.value", "architectures", "architectures lists %s twice", a)
		case spelt:
			seen[a] = true
			p.Architectures = append(p.Architectures, a)
		case unameOf(a) != "":
			c.refuse("field.value", "architectures", "architectures lists %s, which is an ecosystem's spelling: write the uname name, %s", a, unameOf(a))
		default:
			c.refuse("field.value", "architectures", "architectures lists %s, an architecture the build has no spelling for; it can be told %s", a, strings.Join(known, " or "))
		}
	}
}

// unameOf is the uname name an ecosystem's spelling stands for, or empty.
func unameOf(spelling string) string {
	for u, s := range Architectures {
		if s.Go == spelling || s.Deb == spelling || s.Rust == spelling {
			return u
		}
	}
	return ""
}

// pluginProjectJoins checks what the file names against the tree: the build's directory exists, the
// Manifest is inside it, and the Manifest is one the gate accepts, of the file's identity.
func (c *checker) pluginProjectJoins(p *PluginProject) {
	root := filepath.Dir(c.doc.Path)
	if p.BuildIn != "" {
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(p.BuildIn))); err != nil || !info.IsDir() {
			c.refuse("path.absent", "build.in", "build.in %q is not a directory in the project", p.BuildIn)
			return
		}
	}
	if p.Manifest == "" {
		return
	}
	if p.BuildIn != "" {
		in, _ := inside(p.BuildIn)
		m, _ := inside(p.Manifest)
		if in != "." && !strings.HasPrefix(m, in+"/") {
			c.refuse("path.outside", "manifest", "the manifest %q is not inside build.in, %q: the Manifest is kept with the source the build reads", p.Manifest, p.BuildIn)
			return
		}
	}
	at := filepath.Join(root, filepath.FromSlash(p.Manifest))
	if _, err := os.Stat(at); err != nil {
		c.refuse("path.absent", "manifest", "the manifest %q is not in the project: the Manifest is kept in the source, generated by the author's build", p.Manifest)
		return
	}
	report, m := CheckManifest(Read(at, Descriptor))
	c.report.Findings = append(c.report.Findings, report.Findings...)
	if m == nil {
		return
	}
	p.Declared = m
	if p.ID != "" && m.ID != p.ID {
		c.refuse("plugin_project.id", "id", "the file's id is %s and the Manifest's is %s: they are one identity", p.ID, m.ID)
	}
}
