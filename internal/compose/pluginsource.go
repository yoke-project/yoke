package compose

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// semverTag is a tag the composer can order: vX.Y.Z, with a pre-release and build metadata as SemVer
// writes them.
var semverTag = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// remote splits a source that is a repository, <url>@<tag>, into the two; ok is false for a path. A
// repository named with no tag has an empty tag.
func remote(source string) (url, tag string, ok bool) {
	if info, err := os.Stat(source); err == nil && info.IsDir() {
		return "", "", false
	}
	if !strings.Contains(source, "://") && !regexp.MustCompile(`^[^/@]+@[^/:]+:`).MatchString(source) {
		return "", "", false
	}
	at := strings.LastIndex(source, "@")
	if at < 0 || strings.ContainsAny(source[at+1:], "/:") {
		return source, "", true
	}
	return source[:at], source[at+1:], true
}

// resolve is the commit the repository's tag names, peeled where the tag is annotated.
func resolve(url, tag string) (string, error) {
	if !semverTag.MatchString(tag) {
		if tag == "" {
			return "", refusef("%s names no tag: a Plugin is taken from a repository at a tag, <url>@vX.Y.Z", url)
		}
		return "", refusef("the tag %q is not a version the composer can order: a Plugin's tag is SemVer, vX.Y.Z", tag)
	}
	out, err := exec.Command("git", "ls-remote", "--tags", url, "refs/tags/"+tag, "refs/tags/"+tag+"^{}").Output()
	if err != nil {
		return "", refusef("%s cannot be obtained: %v", url, gitSaid(err))
	}
	var plain, peeled string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[1] {
		case "refs/tags/" + tag:
			plain = fields[0]
		case "refs/tags/" + tag + "^{}":
			peeled = fields[0]
		}
	}
	if peeled != "" {
		return peeled, nil
	}
	if plain == "" {
		return "", refusef("%s has no tag %s", url, tag)
	}
	return plain, nil
}

func gitSaid(err error) string {
	if exit, ok := err.(*exec.ExitError); ok && len(exit.Stderr) > 0 {
		return strings.TrimSpace(string(exit.Stderr))
	}
	return err.Error()
}

// obtain clones the repository into a directory of its own, at the commit given; the caller removes it.
func obtain(url, commit string) (string, error) {
	dir, err := os.MkdirTemp("", "yoke-plugin-source-")
	if err != nil {
		return "", err
	}
	for _, args := range [][]string{
		{"clone", "--quiet", "--no-checkout", url, dir},
		{"-C", dir, "-c", "advice.detachedHead=false", "checkout", "--quiet", commit},
	} {
		if err := exec.Command("git", args...).Run(); err != nil {
			os.RemoveAll(dir)
			return "", refusef("%s cannot be obtained at %s: %v", url, commit, gitSaid(err))
		}
	}
	return dir, nil
}

// takeRemote takes the Plugin a repository holds at a tag.
func takeRemote(dir, url, tag string, o TakeOptions) (string, error) {
	commit, err := resolve(url, tag)
	if err != nil {
		return "", err
	}
	path, err := obtain(url, commit)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(path)
	if !exists(filepath.Join(path, PluginFile)) {
		return "", refusef("%s at %s holds no %s at its root: it is not a Plugin project", url, tag, PluginFile)
	}
	return take(dir, path, &taking{source: url, tag: tag, commit: commit, clean: true, tracked: true}, o)
}

// UpdatePlugin takes the Plugin taken as id again, from the same source: a repository at the tag given,
// which it requires, or a path as it now stands, which takes no tag. The
// executable, the Manifest, the pinned digest and the record are replaced, and the unit keeps its name
// and its other declarations. Any refusal leaves the project as it was.
func UpdatePlugin(dir, id, tag string, o TakeOptions) error {
	entries, err := readPlugins(dir)
	if err != nil {
		return err
	}
	i := -1
	for j, e := range entries {
		if e.ID == id {
			i = j
		}
	}
	if i < 0 {
		return refusef("%s is not taken into this project: it is taken with add plugin", id)
	}
	e := entries[i]
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	var path, commit string
	clean, tracked := true, true
	if e.Tag == "" {
		if tag != "" {
			return refusef("%s was taken from a path, which has no tags: update it without --to", id)
		}
		if path, err = pathOf(dir, e.Source); err != nil {
			return err
		}
		commit, clean, tracked = revision(path)
	} else {
		if tag == "" {
			return refusef("%s was taken from a repository: name the tag to take with --to", id)
		}
		if commit, err = resolve(e.Source, tag); err != nil {
			return err
		}
		if tag == e.Tag && commit != e.Commit {
			fmt.Fprintf(out, "The tag %s now names %s; it named %s when %s was taken.\n", tag, commit, e.Commit, id)
		}
		if path, err = obtain(e.Source, commit); err != nil {
			return err
		}
		defer os.RemoveAll(path)
	}
	report, p := CheckPlugin(path)
	if p == nil {
		return refusef("%s", refusalLines(report))
	}
	if p.ID != id {
		return refusef("%s is %s, not %s", e.Source, p.ID, id)
	}
	arch, err := descriptorArch(dir)
	if err != nil {
		return err
	}
	if !slices.Contains(p.Architectures, arch) {
		return refusef("the application is for %s, and %s is built for %s: nothing was run", arch, id, strings.Join(p.Architectures, ", "))
	}
	if err := ask(&taking{source: e.Source, tag: tag, commit: commit, clean: clean, tracked: tracked, project: p}, o); err != nil {
		return err
	}
	built, err := buildPlugin(p, arch, PluginBuildOptions{Stdout: o.Out, Stderr: o.Out})
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(built))

	program := filepath.Join(dir, Components, "plugins", e.Unit)
	manifest := filepath.Join(dir, Manifests, id+".yaml")
	oldProgram, _ := os.ReadFile(program)
	oldManifest, _ := os.ReadFile(manifest)
	restore := func() {
		os.WriteFile(program, oldProgram, 0o755)
		os.WriteFile(manifest, oldManifest, 0o644)
	}
	if err := copyExecutable(built, program); err != nil {
		restore()
		return err
	}
	kept, err := os.ReadFile(p.Declared.Path)
	if err == nil {
		err = os.WriteFile(manifest, kept, 0o644)
	}
	if err != nil {
		restore()
		return err
	}
	data, _ := os.ReadFile(program)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	if err := Set(ProjectDocument(dir), "units."+e.Unit+".digest", digest); err != nil {
		restore()
		return err
	}
	e.Tag, e.Digest, e.Taken = tag, digest, pluginClock().UTC().Format(time.RFC3339)
	e.Commit, e.Clean = "", nil
	if tracked {
		e.Commit, e.Clean = commit, &clean
	}
	entries[i] = e
	return writePlugins(dir, entries)
}

// pathOf is the Plugin project a recorded path names: as add plugin read it, or beside the application.
func pathOf(dir, source string) (string, error) {
	candidates := []string{source}
	if !filepath.IsAbs(source) {
		candidates = append(candidates, filepath.Join(dir, source))
	}
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil && exists(filepath.Join(abs, PluginFile)) {
			return abs, nil
		}
	}
	return "", refusef("%s holds no %s at its root: it is not a Plugin project", source, PluginFile)
}

// ListPlugins writes what was taken into the project: identity, unit, source, tag and commit, and for a
// tag whether it still names that commit. It changes nothing.
func ListPlugins(dir string, out io.Writer) error {
	entries, err := readPlugins(dir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintf(out, "no Plugin is taken into %s\n", dir)
		return nil
	}
	for _, e := range entries {
		fmt.Fprintf(out, "%s  unit %s\n  source: %s\n", e.ID, e.Unit, e.Source)
		commit := e.Commit
		if commit == "" {
			commit = "none: the source was not a git tree"
		} else if e.Clean != nil && !*e.Clean {
			commit += ", with changes not committed"
		}
		if e.Tag == "" {
			fmt.Fprintf(out, "  tag:    none: taken from a path\n  commit: %s\n", commit)
			continue
		}
		fmt.Fprintf(out, "  tag:    %s\n  commit: %s\n", e.Tag, commit)
		switch now, err := resolve(e.Source, e.Tag); {
		case err != nil:
			fmt.Fprintf(out, "  the tag cannot be read now: %v\n", err)
		case now == e.Commit:
			fmt.Fprintf(out, "  the tag still names it\n")
		default:
			fmt.Fprintf(out, "  the tag %s now names %s, and %s was taken\n", e.Tag, now, e.Commit)
		}
	}
	return nil
}
