package compose

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// Plugins is the record of the Plugins taken into a project, beside its descriptor and never packaged.
const Plugins = "plugins.yaml"

// pluginClock dates what is taken, and pluginInput is where the author's answer is read from.
var (
	pluginClock           = time.Now
	pluginInput io.Reader = os.Stdin
)

// TakeOptions are what `add plugin` is asked for besides the source.
type TakeOptions struct {
	// As is the unit's name; empty is the program's.
	As string
	// Yes answers the question before the author's build, for continuous integration.
	Yes bool
	// Out receives what is shown and the build's output; Answer is where the answer is read from, the
	// standard input where nil.
	Out    io.Writer
	Answer io.Reader
}

// taking is one Plugin as it is taken: where it came from, and what the record says of it.
type taking struct {
	source, tag, commit string
	clean, tracked      bool
	project             *gate.PluginProject
}

// AddPlugin takes the Plugin whose project is at the path source into the application project in dir:
// it checks the Plugin, shows what it will run and asks, builds it for the descriptor's architecture,
// places the executable and the Manifest, declares the unit with the executable's digest pinned, and
// records the Plugin in plugins.yaml. Any refusal leaves the project as it was. Nothing written grants
// anything. It returns the unit's name.
func AddPlugin(dir, source string, o TakeOptions) (string, error) {
	if kind, err := KindOf(dir); err != nil {
		return "", err
	} else if kind != ApplicationProject {
		return "", refusef("%s is a Plugin project: a Plugin is taken into an application project", dir)
	}
	if url, tag, ok := remote(source); ok {
		return takeRemote(dir, url, tag, o)
	}
	abs, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	if !exists(filepath.Join(abs, PluginFile)) {
		return "", refusef("%s holds no %s at its root: it is not a Plugin project", source, PluginFile)
	}
	t := &taking{source: source}
	t.commit, t.clean, t.tracked = revision(abs)
	return take(dir, abs, t, o)
}

// take is the steps every source shares once it is on disk at path, the source's own facts in t.
func take(dir, path string, t *taking, o TakeOptions) (string, error) {
	report, p := CheckPlugin(path)
	if p == nil {
		return "", refusef("%s", refusalLines(report))
	}
	t.project = p
	arch, err := descriptorArch(dir)
	if err != nil {
		return "", err
	}
	if !slices.Contains(p.Architectures, arch) {
		return "", refusef("the application is for %s, and %s is built for %s: nothing was run", arch, p.ID, strings.Join(p.Architectures, ", "))
	}
	unit := o.As
	if unit == "" {
		unit = p.Program
	}
	program := filepath.ToSlash(filepath.Join(Components, "plugins", unit))
	manifest := filepath.ToSlash(filepath.Join(Manifests, p.ID+".yaml"))
	entries, err := readPlugins(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.ID == p.ID {
			return "", refusef("%s is already taken, as the unit %s: another revision of it is taken with update plugin", p.ID, e.Unit)
		}
	}
	if declared(dir, unit) {
		return "", refusef("the unit %s is declared: name the Plugin's unit with --as", unit)
	}
	for _, at := range []string{program, manifest} {
		if at == program && travelsAsImage(p) {
			continue
		}
		if exists(filepath.Join(dir, filepath.FromSlash(at))) {
			return "", refusef("%s is already in the project, and no Plugin's record accounts for it", at)
		}
	}

	if err := ask(t, o); err != nil {
		return "", err
	}
	built, err := buildPlugin(p, arch, PluginBuildOptions{Stdout: o.Out, Stderr: o.Out})
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(filepath.Dir(built))

	// Placed, then declared: the gate reads the Manifest the unit names, so it is there first, and both
	// files go again if the declaration is refused.
	var placed []string
	undo := func() {
		for _, at := range placed {
			os.Remove(filepath.Join(dir, filepath.FromSlash(at)))
		}
	}
	var image, imageSum string
	if travelsAsImage(p) {
		if imageSum, err = imageDigest(built); err != nil {
			return "", err
		}
		program, image = imagePlace(p, imageSum)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, filepath.FromSlash(program))), 0o755); err != nil {
		return "", err
	}
	if err := copyExecutable(built, filepath.Join(dir, filepath.FromSlash(program))); err != nil {
		return "", err
	}
	placed = append(placed, program)
	kept, err := os.ReadFile(p.Declared.Path)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, filepath.FromSlash(manifest)), kept, 0o644)
	}
	if err != nil {
		undo()
		return "", err
	}
	placed = append(placed, manifest)
	options := UnitOptions{Kind: "plugin", Plugin: p.ID, Exec: program}
	if image != "" {
		options.Exec, options.Image = "", image
	}
	if err := AddUnit(ProjectDocument(dir), unit, options); err != nil {
		undo()
		return "", err
	}
	data, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(program)))
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	if image != "" {
		digest = imageSum
	}
	entry := pluginEntry{ID: p.ID, Unit: unit, Source: t.source, Tag: t.tag, Arch: arch,
		Digest: digest, Taken: pluginClock().UTC().Format(time.RFC3339)}
	if t.tracked {
		entry.Commit = t.commit
		clean := t.clean
		entry.Clean = &clean
	}
	if err := writePlugins(dir, append(entries, entry)); err != nil {
		return "", err
	}
	return unit, nil
}

// ask shows the source, the commit and the command the author's build will run, and asks; Yes answers.
// Anything but yes, an answer not given included, declines.
func ask(t *taking, o TakeOptions) error {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	p := t.project
	commit := t.commit
	switch {
	case !t.tracked:
		commit = "none: the source is not a git tree"
	case !t.clean:
		commit += ", with changes not committed"
	}
	fmt.Fprintf(out, "Taking %s runs its author's build.\n  source:  %s\n", p.ID, t.source)
	if t.tag != "" {
		fmt.Fprintf(out, "  tag:     %s\n", t.tag)
	}
	fmt.Fprintf(out, "  commit:  %s\n  in:      %s\n  command: %s\n", commit, p.BuildIn, p.BuildRun)
	if p.BuildContainer != "" {
		fmt.Fprintf(out, "  inside:  %s\n", p.BuildContainer)
	}
	if o.Yes {
		fmt.Fprintln(out, "Run it? yes (--yes)")
		return nil
	}
	fmt.Fprint(out, "Run it? [y/N] ")
	answer := o.Answer
	if answer == nil {
		answer = pluginInput
	}
	line, _ := bufio.NewReader(answer).ReadString('\n')
	if a := strings.ToLower(strings.TrimSpace(line)); a == "y" || a == "yes" {
		return nil
	}
	fmt.Fprintln(out)
	return refusef("declined: %s's build was not run, and nothing was taken", p.ID)
}

// revision is the commit the source's git tree is at and whether it holds changes not committed; tracked
// is false where the source is not a git tree.
func revision(path string) (commit string, clean, tracked bool) {
	head, err := exec.Command("git", "-C", path, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false, false
	}
	status, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	return strings.TrimSpace(string(head)), err == nil && len(strings.TrimSpace(string(status))) == 0, true
}

// descriptorArch is the architecture the application project's descriptor names.
func descriptorArch(dir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, Descriptor))
	if err != nil {
		return "", err
	}
	var d struct {
		Arch string `yaml:"arch"`
	}
	if err := yaml.Unmarshal(data, &d); err != nil || d.Arch == "" {
		return "", refusef("%s names no arch: a Plugin is built for the application's architecture", Descriptor)
	}
	return d.Arch, nil
}

// declared says whether the descriptor declares the unit.
func declared(dir, unit string) bool {
	data, err := os.ReadFile(filepath.Join(dir, Descriptor))
	if err != nil {
		return false
	}
	var d struct {
		Units map[string]any `yaml:"units"`
	}
	if yaml.Unmarshal(data, &d) != nil {
		return false
	}
	_, there := d.Units[unit]
	return there
}

// pluginEntry is one Plugin's entry in plugins.yaml.
type pluginEntry struct {
	ID     string `yaml:"id"`
	Unit   string `yaml:"unit"`
	Source string `yaml:"source"`
	Tag    string `yaml:"tag"`
	Commit string `yaml:"commit,omitempty"`
	Clean  *bool  `yaml:"clean,omitempty"`
	Arch   string `yaml:"arch"`
	Digest string `yaml:"digest"`
	Taken  string `yaml:"taken"`
}

// MarshalYAML writes a Plugin with no tag as `tag: null`, so that the record says it has none.
func (e pluginEntry) MarshalYAML() (any, error) {
	n := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v *yaml.Node) { n.Content = append(n.Content, scalar(k), v) }
	add("id", scalar(e.ID))
	add("unit", scalar(e.Unit))
	add("source", scalar(e.Source))
	if e.Tag == "" {
		add("tag", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"})
	} else {
		add("tag", scalar(e.Tag))
	}
	if e.Commit != "" {
		add("commit", scalar(e.Commit))
	}
	if e.Clean != nil {
		add("clean", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(*e.Clean)})
	}
	add("arch", scalar(e.Arch))
	add("digest", scalar(e.Digest))
	add("taken", scalar(e.Taken))
	return n, nil
}

func readPlugins(dir string) ([]pluginEntry, error) {
	data, err := os.ReadFile(filepath.Join(dir, Plugins))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Plugins []pluginEntry `yaml:"plugins"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, refusef("%s does not parse: %v", Plugins, err)
	}
	return doc.Plugins, nil
}

// writePlugins writes plugins.yaml whole and renames it into place; with no entry left, there is none.
func writePlugins(dir string, entries []pluginEntry) error {
	path := filepath.Join(dir, Plugins)
	if len(entries) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(struct {
		Plugins []pluginEntry `yaml:"plugins"`
	}{entries}); err != nil {
		return err
	}
	enc.Close()
	tmp := path + ".writing"
	if err := os.WriteFile(tmp, []byte(out.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// untake takes the Plugin taken as unit out of plugins.yaml, where there is one.
func untake(dir, unit string) error {
	entries, err := readPlugins(dir)
	if err != nil || entries == nil {
		return err
	}
	kept := slices.DeleteFunc(slices.Clone(entries), func(e pluginEntry) bool { return e.Unit == unit })
	if len(kept) == len(entries) {
		return nil
	}
	return writePlugins(dir, kept)
}
