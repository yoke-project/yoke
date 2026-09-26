package verify

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The record schema's version. It moves when a field is added, removed or given a new meaning, so a
// reader a year from now knows what it is holding.
const schema = 1

// A Record is what one run of one level observed, and the only thing this project keeps of a run.
type Record struct {
	Schema      int               `json:"schema"`
	Repository  string            `json:"repository"`
	Commit      string            `json:"commit"`
	Level       string            `json:"level"`
	Tier        string            `json:"tier"`
	Environment map[string]string `json:"environment"`
	Started     string            `json:"started"`
	Finished    string            `json:"finished"`
	Ran         []Artifact        `json:"ran"`
	State       string            `json:"state"`
	Blocks      bool              `json:"blocks"`
	Cases       []CaseResult      `json:"cases"`
}

// An Artifact is one thing the run involved, as the run states it.
type Artifact struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

// A CaseResult is what one declared case did. It answers to the case's identifier and never to the
// name of the test performing it, which is the map's business and not the record's.
type CaseResult struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Label  string `json:"label"`
	Item   string `json:"item,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// The four results a case takes.
const (
	pass          = "pass"
	fail          = "fail"
	absent        = "absent"
	notApplicable = "not applicable"
)

// The three states a level takes.
const (
	passed     = "passed"
	failed     = "failed"
	notReached = "not reached"
)

// repeated is an option given more than once, in the order it was given.
type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ", ") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

// recordOptions are the things no runner knows: whoever starts the run states them, and nothing about
// a forge or a workflow reaches this tool.
type recordOptions struct {
	level       string
	tier        string
	commit      string
	started     string
	finished    string
	notReached  bool
	results     repeated
	environment repeated
	ran         repeated
}

func (o *recordOptions) register(flags *flag.FlagSet) {
	flags.StringVar(&o.level, "level", "", "the level this run performed: L0 to L5")
	flags.StringVar(&o.tier, "tier", "reference", "the environment tier: reference or complete")
	flags.StringVar(&o.commit, "commit", "", "the commit the run was of; read from the tree when absent")
	flags.StringVar(&o.started, "started", "", "when the run started, in UTC")
	flags.StringVar(&o.finished, "finished", "", "when the run finished, in UTC")
	flags.BoolVar(&o.notReached, "not-reached", false, "the level did not run, because the level before it failed")
	flags.Var(&o.results, "results", "a file of a runner's own output, or of the lines a check writes; may be given more than once")
	flags.Var(&o.environment, "environment", "a dimension the run fixed, as `dimension=value`; may be given more than once")
	flags.Var(&o.ran, "ran", "an artifact the run involved, as `name=version@commit`; may be given more than once")
}

// an outcome is what a run reported about one test or one case.
type outcome struct {
	result string
	detail string
}

// buildRecord turns a run's own output into the record of every case declared for its level.
func buildRecord(root, repository string, descriptions []description, pairs []pair, o recordOptions) (Record, []Finding) {
	var findings []Finding
	at := func(file, format string, a ...any) {
		findings = append(findings, Finding{File: file, Msg: fmt.Sprintf(format, a...)})
	}

	if o.level == "" {
		at("", "no level given, and a record is of one level")
		return Record{}, findings
	}

	byTest := map[string]string{}
	for _, p := range pairs {
		if claimed, taken := byTest[p.test]; taken && claimed != p.id {
			at(p.file, "the test %s is claimed by %s and by %s", p.test, claimed, p.id)
		}
		byTest[p.test] = p.id
	}

	// One run may perform several levels: this level's record takes its own cases, and leaves a result
	// for a case declared at another level to that level's record.
	declared, elsewhere := map[string]stdCase{}, map[string]bool{}
	for _, d := range descriptions {
		for _, c := range d.cases {
			switch {
			case c.struck:
			case value(c, "Level") == o.level:
				declared[c.id] = c
			default:
				elsewhere[c.id] = true
			}
		}
	}

	reported := map[string]outcome{}
	for _, file := range o.results {
		content, err := os.ReadFile(file)
		if err != nil {
			at(file, "the results could not be read: %v", err)
			continue
		}
		for key, got := range readResults(string(content)) {
			id, named := byTest[key]
			if !named {
				id = key
			}
			if _, isDeclared := declared[id]; !isDeclared {
				if !elsewhere[id] {
					at(file, "%s answers for no case any level declares", key)
				}
				continue
			}
			if before, twice := reported[id]; twice && before.result != got.result {
				at(file, "%s is reported %s and %s in the same run", id, before.result, got.result)
			}
			reported[id] = got
		}
	}

	if len(findings) > 0 {
		return Record{}, findings
	}

	commit, err := commitOf(root, o.commit)
	if err != nil {
		at("", "the record names no commit, and a record that names none cannot be joined to anything: %v", err)
		return Record{}, findings
	}
	environment, findings := pairsOf(o.environment, "=", "environment", findings)
	record := Record{
		Schema:      schema,
		Repository:  repository,
		Commit:      commit,
		Level:       o.level,
		Tier:        o.tier,
		Environment: environment,
		Started:     instant(o.started),
		Finished:    instant(o.finished),
		Ran:         artifacts(o.ran),
		State:       passed,
	}

	ids := make([]string, 0, len(declared))
	for id := range declared {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		c := declared[id]
		entry := CaseResult{ID: id, Result: absent}
		entry.Label, entry.Item = label(c)

		switch {
		case excluded(c, environment):
			entry.Result = notApplicable
		default:
			if got, answered := reported[id]; answered {
				entry.Result = got.result
				entry.Detail = got.detail
			}
		}

		if entry.Result == fail || entry.Result == absent {
			record.State = failed
			if entry.Label == "blocking" {
				record.Blocks = true
			}
		}
		record.Cases = append(record.Cases, entry)
	}

	if o.notReached {
		record.State = notReached
	}
	if record.Cases == nil {
		record.Cases = []CaseResult{}
	}
	return record, findings
}

// readResults reads either form a run of this project emits: a Go runner's own machine-readable
// output, keyed by the test's name, or the lines a check writes, keyed by the case's identifier. A
// check has no runner to translate it, so the tool reads what it already prints.
func readResults(content string) map[string]outcome {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			return goResults(content)
		}
		break
	}
	return checkResults(content)
}

// goResults reads `go test -json`. A subtest's output belongs to the test carrying the marker, and
// its own verdict is left to the test it is part of.
func goResults(content string) map[string]outcome {
	type event struct {
		Action string
		Test   string
		Output string
	}

	results := map[string]outcome{}
	details := map[string]*strings.Builder{}

	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil || e.Test == "" {
			continue
		}
		name, _, _ := strings.Cut(e.Test, "/")

		switch e.Action {
		case "output":
			if details[name] == nil {
				details[name] = &strings.Builder{}
			}
			details[name].WriteString(e.Output)
		case "pass":
			if name == e.Test {
				results[name] = outcome{result: pass}
			}
		case "fail":
			if name == e.Test {
				results[name] = outcome{result: fail}
			}
		case "skip":
			if name == e.Test {
				results[name] = outcome{result: fail, detail: "the runner reported it skipped"}
			}
		}
	}

	for name, got := range results {
		if got.result == pass || details[name] == nil {
			continue
		}
		said := strings.TrimSpace(details[name].String())
		if said == "" {
			continue
		}
		if got.detail != "" {
			said = got.detail + ": " + said
		}
		results[name] = outcome{result: got.result, detail: said}
	}
	return results
}

// checkResults reads the lines checks/run.sh writes: `pass  <id>` and `FAIL  <id> — what failed`.
func checkResults(content string) map[string]outcome {
	results := map[string]outcome{}
	for _, line := range strings.Split(content, "\n") {
		verdict, rest, found := strings.Cut(strings.TrimSpace(line), " ")
		if !found {
			continue
		}
		id, detail, _ := strings.Cut(strings.TrimSpace(rest), " — ")
		switch verdict {
		case "pass":
			results[strings.TrimSpace(id)] = outcome{result: pass}
		case "FAIL":
			results[strings.TrimSpace(id)] = outcome{result: fail, detail: strings.TrimSpace(detail)}
		}
	}
	return results
}

// excluded is the one place not applicable is decided: from the description's declaration against the
// environment the run fixed, and never from anything a runner said.
func excluded(c stdCase, environment map[string]string) bool {
	declaration := value(c, "Not applicable in")
	if declaration == "" || declaration == "—" {
		return false
	}
	for _, each := range strings.Split(declaration, " · ") {
		dimension, excludedValue, ok := strings.Cut(each, ": ")
		if !ok {
			continue
		}
		fixed, stated := environment[strings.TrimSpace(dimension)]
		if stated && strings.EqualFold(strings.TrimSpace(fixed), strings.TrimSpace(excludedValue)) {
			return true
		}
	}
	return false
}

// label reads a case's label as the description declared it, with the item a not-blocking one cites.
func label(c stdCase) (string, string) {
	declared := value(c, "Label")
	if rest, isNotBlocking := strings.CutPrefix(declared, "not blocking"); isNotBlocking {
		item, _ := strings.CutPrefix(strings.TrimSpace(rest), "— ")
		return "not blocking", strings.TrimSpace(item)
	}
	return declared, ""
}

func value(c stdCase, name string) string {
	for _, f := range c.fields {
		if f.name == name {
			return f.value
		}
	}
	return ""
}

func pairsOf(given repeated, separator, what string, findings []Finding) (map[string]string, []Finding) {
	pairs := map[string]string{}
	for _, each := range given {
		key, v, ok := strings.Cut(each, separator)
		if !ok {
			findings = append(findings, Finding{Msg: fmt.Sprintf("the %s %q is not key%svalue", what, each, separator)})
			continue
		}
		pairs[strings.TrimSpace(key)] = strings.TrimSpace(v)
	}
	return pairs, findings
}

// artifacts reads `name=version@commit`, which is how a run states what it involved.
func artifacts(given repeated) []Artifact {
	out := []Artifact{}
	for _, each := range given {
		name, rest, _ := strings.Cut(each, "=")
		version, commit, _ := strings.Cut(rest, "@")
		out = append(out, Artifact{Name: name, Version: version, Commit: commit})
	}
	return out
}

func instant(given string) string {
	if given != "" {
		return given
	}
	return time.Now().UTC().Format(time.RFC3339)
}

// commitOf reads the commit from the tree itself, so that nothing about a forge reaches this tool. It
// reads the layouts git makes: `.git` a directory, or a file naming a worktree's directory whose
// references live in a common one; and a reference as a file of its own, or as a line of `packed-refs`.
func commitOf(root, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	gitDir := filepath.Join(root, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return "", fmt.Errorf("%s is not a repository", root)
	}
	if !info.IsDir() {
		pointer, err := os.ReadFile(gitDir)
		if err != nil {
			return "", err
		}
		named, isPointer := strings.CutPrefix(strings.TrimSpace(string(pointer)), "gitdir: ")
		if !isPointer {
			return "", fmt.Errorf("%s names no directory", gitDir)
		}
		if !filepath.IsAbs(named) {
			named = filepath.Join(root, named)
		}
		gitDir = named
	}
	commonDir := gitDir
	if common, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		commonDir = strings.TrimSpace(string(common))
		if !filepath.IsAbs(commonDir) {
			commonDir = filepath.Join(gitDir, commonDir)
		}
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	pointer := strings.TrimSpace(string(head))
	reference, isReference := strings.CutPrefix(pointer, "ref: ")
	if !isReference {
		return commitIn(pointer, "HEAD")
	}
	for _, dir := range []string{gitDir, commonDir} {
		if loose, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(reference))); err == nil {
			return commitIn(strings.TrimSpace(string(loose)), reference)
		}
	}
	if packed, err := os.ReadFile(filepath.Join(commonDir, "packed-refs")); err == nil {
		for _, line := range strings.Split(string(packed), "\n") {
			if sha, name, found := strings.Cut(strings.TrimSpace(line), " "); found && name == reference {
				return commitIn(sha, reference)
			}
		}
	}
	return "", fmt.Errorf("the reference %s names no commit", reference)
}

// commitIn accepts what a reference holds only when it is an object name.
func commitIn(value, from string) (string, error) {
	if len(value) != 40 && len(value) != 64 || strings.Trim(value, "0123456789abcdef") != "" {
		return "", fmt.Errorf("%s holds %q, which is no commit", from, value)
	}
	return value, nil
}
