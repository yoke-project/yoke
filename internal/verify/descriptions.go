package verify

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The eight fields of a case, in the order a description fixes and no other.
var theFields = []string{
	"Cites", "Level", "Method", "Not applicable in", "Label", "Precondition", "Action", "Expected",
}

// The levels a description written by hand may name: L2 is the suite's own rendering.
var theLevels = []string{"L0", "L1", "L3", "L4", "L5"}

// The five environment dimensions, with the values each takes.
var theDimensions = map[string][]string{
	"architecture":             {"amd64", "arm64"},
	"deployment form":          {"service", "application"},
	"unit backend":             {"host", "container"},
	"container engine":         {"podman", "docker"},
	"control-group delegation": {"present", "absent"},
}

type field struct{ name, value string }

type stdCase struct {
	id     string
	title  string
	struck bool
	fields []field
}

type description struct {
	path    string // relative to the root, with forward slashes
	feature string
	h1      string
	rows    []field
	cases   []stdCase
}

// readDescriptions reads every <feature>.std.md under the root, in path order.
func readDescriptions(root string) ([]description, []Finding) {
	var descriptions []description
	var findings []Finding

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".std.md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		descriptions = append(descriptions, parseDescription(relative(root, path), string(content)))
		return nil
	})
	if err != nil {
		findings = append(findings, Finding{Msg: fmt.Sprintf("yoke-verify: %v", err)})
	}

	sort.Slice(descriptions, func(i, j int) bool { return descriptions[i].path < descriptions[j].path })
	return descriptions, findings
}

func relative(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

// parseDescription reads the form and never judges it: every rule is checked in checkDescriptions,
// so a reader of this function sees what a description is and not what it must be.
func parseDescription(path, content string) description {
	d := description{
		path:    path,
		feature: strings.TrimSuffix(filepath.Base(path), ".std.md"),
	}

	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "# "):
			if d.h1 == "" {
				d.h1 = strings.TrimSpace(line[2:])
			}

		case strings.HasPrefix(line, "## "):
			c := parseHeading(line[3:])
			i = parseTable(lines, i+1, &c)
			d.cases = append(d.cases, c)

		case strings.HasPrefix(line, "|") && len(d.cases) == 0:
			if name, value, ok := parseRow(line); ok {
				d.rows = append(d.rows, field{name, value})
			}
		}
	}
	return d
}

func parseHeading(heading string) stdCase {
	c := stdCase{}
	heading = strings.TrimSpace(heading)
	if strings.HasPrefix(heading, "~~") {
		c.struck = true
		heading = heading[2:]
		if end := strings.Index(heading, "~~"); end >= 0 {
			heading = heading[:end]
		}
	}
	id, title, found := strings.Cut(heading, " — ")
	c.id = strings.TrimSpace(id)
	if found {
		c.title = strings.TrimSpace(title)
	}
	return c
}

// parseTable reads the rows following a case's heading and returns the index of the last line it used.
func parseTable(lines []string, from int, c *stdCase) int {
	i := from
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
			return i - 1
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		if name, value, ok := parseRow(line); ok {
			c.fields = append(c.fields, field{name, value})
		}
	}
	return i - 1
}

// parseRow reads `| **Name** | value |`, which is how both tables of a description carry a pair.
func parseRow(line string) (name, value string, ok bool) {
	cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	if len(cells) != 2 {
		return "", "", false
	}
	name = strings.TrimSpace(cells[0])
	if !strings.HasPrefix(name, "**") || !strings.HasSuffix(name, "**") {
		return "", "", false
	}
	return strings.Trim(name, "*"), strings.TrimSpace(cells[1]), true
}

// checkDescriptions is every rule the form states about a description, and the only place any of
// them is enforced.
func checkDescriptions(descriptions []description, repository string) []Finding {
	var findings []Finding

	for _, d := range descriptions {
		at := func(format string, a ...any) {
			findings = append(findings, Finding{File: d.path, Msg: fmt.Sprintf(format, a...)})
		}

		if d.h1 == "" {
			at("no H1 names the feature")
		}
		if len(d.rows) != 2 || d.rows[0].name != "Feature" || d.rows[1].name != "Planning item" {
			at("the opening table is not the two rows Feature and Planning item")
		} else {
			for _, row := range d.rows {
				if row.value == "" {
					at("the row %s is empty", row.name)
				}
			}
		}

		taken := map[string]string{}
		for _, c := range d.cases {
			repo, feature, number, ok := splitIdentifier(c.id)
			switch {
			case !ok:
				at("%s is not an identifier of the form <repository>:<feature>.<nn>", c.id)
			default:
				if repo != repository {
					at("%s names the repository %s, and this one is %s", c.id, repo, repository)
				}
				if feature != d.feature {
					at("%s names the feature %s, and this file is %s", c.id, feature, d.feature)
				}
				if !isNumber(number) || len(number) < 2 {
					at("%s is numbered %s, and a number is at least two digits", c.id, number)
				}
			}

			if where, again := taken[c.id]; again {
				at("%s is used twice: %s, and again here — a number is never reused, struck or not", c.id, where)
			} else {
				taken[c.id] = c.title
			}

			if c.struck {
				if len(c.fields) > 0 {
					at("%s is struck and carries a table; a struck case keeps its number and nothing else", c.id)
				}
				continue
			}
			findings = append(findings, checkFields(d.path, c)...)
		}
	}
	return findings
}

// checkFields checks the eight fields of a live case: that they are the eight, in order, once each,
// and that every value is one the field allows.
func checkFields(path string, c stdCase) []Finding {
	var findings []Finding
	at := func(format string, a ...any) {
		findings = append(findings, Finding{File: path, Msg: fmt.Sprintf(format, a...)})
	}

	seen := map[string]int{}
	for _, f := range c.fields {
		seen[f.name]++
	}
	for name, count := range seen {
		if !known(name) {
			at("%s carries the field %s, which is not one of the eight", c.id, name)
		} else if count > 1 {
			at("%s carries the field %s %d times", c.id, name, count)
		}
	}
	for _, name := range theFields {
		if seen[name] == 0 {
			at("%s carries no field %s", c.id, name)
		}
	}

	var order []string
	for _, f := range c.fields {
		if known(f.name) && seen[f.name] == 1 {
			order = append(order, f.name)
		}
	}
	var want []string
	for _, name := range theFields {
		if seen[name] == 1 {
			want = append(want, name)
		}
	}
	for i := range order {
		if order[i] != want[i] {
			at("%s carries its fields out of order: %s where %s belongs", c.id, order[i], want[i])
			break
		}
	}

	for _, f := range c.fields {
		if message := checkValue(f); message != "" {
			at("%s: %s", c.id, message)
		}
	}
	return findings
}

func checkValue(f field) string {
	switch f.name {
	case "Level":
		if !contains(theLevels, f.value) {
			return fmt.Sprintf("the field Level holds %q, and a description names one of %s — L2 exists only in the suite's own rendering",
				f.value, strings.Join(theLevels, ", "))
		}

	case "Method":
		if f.value != "test" && f.value != "check" {
			return fmt.Sprintf("the field Method holds %q, and a method is test or check", f.value)
		}

	case "Not applicable in":
		if f.value == "—" {
			return ""
		}
		for _, pair := range strings.Split(f.value, " · ") {
			dimension, value, ok := strings.Cut(pair, ": ")
			values, declared := theDimensions[strings.TrimSpace(dimension)]
			if !ok || !declared {
				return fmt.Sprintf("the field Not applicable in holds %q, and no environment declares that dimension", f.value)
			}
			if !contains(values, strings.ToLower(strings.TrimSpace(value))) {
				return fmt.Sprintf("the field Not applicable in holds %q, and %s takes %s",
					f.value, dimension, strings.Join(values, " or "))
			}
		}

	case "Label":
		if f.value == "blocking" {
			return ""
		}
		rest, isNotBlocking := strings.CutPrefix(f.value, "not blocking")
		if !isNotBlocking {
			return fmt.Sprintf("the field Label holds %q, and a label is blocking, or not blocking citing an item", f.value)
		}
		item, cited := strings.CutPrefix(rest, " — ")
		if !cited || !isItem(item) {
			return fmt.Sprintf("the field Label holds %q, and a not-blocking case cites the item it waits for, as <owner>/<repository>#<number>", f.value)
		}

	default:
		if f.value == "" {
			return fmt.Sprintf("the field %s is empty", f.name)
		}
	}
	return ""
}

func splitIdentifier(id string) (repository, feature, number string, ok bool) {
	repository, rest, ok := strings.Cut(id, ":")
	if !ok || repository == "" {
		return "", "", "", false
	}
	feature, number, ok = cutLast(rest, ".")
	if !ok || feature == "" || number == "" {
		return "", "", "", false
	}
	return repository, feature, number, true
}

func cutLast(s, sep string) (before, after string, found bool) {
	i := strings.LastIndex(s, sep)
	if i < 0 {
		return s, "", false
	}
	return s[:i], s[i+len(sep):], true
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isItem reads <owner>/<repository>#<number>, the form a planning item is written in.
func isItem(s string) bool {
	owner, rest, ok := strings.Cut(s, "/")
	if !ok || owner == "" {
		return false
	}
	repository, number, ok := strings.Cut(rest, "#")
	return ok && repository != "" && isNumber(number)
}

func known(name string) bool { return contains(theFields, name) }

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// An EmittedCase is one case as the tool read it, for a reader that resolves what the tool cannot:
// the citations come out as they were written, because what one means is known where the corpora are
// and never here.
type EmittedCase struct {
	ID     string            `json:"id"`
	File   string            `json:"file"`
	Title  string            `json:"title"`
	Struck bool              `json:"struck,omitempty"`
	Cites  []string          `json:"cites,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
}

// emitted turns what was parsed into what is emitted, in the order the descriptions give.
func emitted(descriptions []description) []EmittedCase {
	cases := []EmittedCase{}
	for _, d := range descriptions {
		for _, c := range d.cases {
			each := EmittedCase{ID: c.id, File: d.path, Title: c.title, Struck: c.struck}
			if !c.struck {
				each.Fields = map[string]string{}
				for _, f := range c.fields {
					each.Fields[f.name] = f.value
				}
				for _, citation := range strings.Split(value(c, "Cites"), " · ") {
					if citation = strings.TrimSpace(citation); citation != "" {
						each.Cites = append(each.Cites, citation)
					}
				}
			}
			cases = append(cases, each)
		}
	}
	return cases
}
