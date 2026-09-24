package verify

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A pair is one case and the test performing it: the map a level's run reads its results through.
type pair struct {
	id   string
	test string
	file string
}

// The marker is `std: <identifier>`, and it is the whole of a comment line — the one construct every
// language of the project has. A comment that opens the
// line is what tells a marker from a fixture quoting one, which is a file this very tool has.
var marker = regexp.MustCompile(`^std:\s*([A-Za-z0-9_-]+:[A-Za-z0-9._-]+)`)

// What opens a comment, in the six languages the project writes tests in and the two its documents use.
var openers = []string{"//", "/*", "*", "#", "--", ";", "<!--", "%"}

// markerOn reads the identifier a comment line names, or the empty string when the line is not one.
func markerOn(line string) string {
	rest := strings.TrimSpace(line)
	for _, opener := range openers {
		if after, ok := strings.CutPrefix(rest, opener); ok {
			found := marker.FindStringSubmatch(strings.TrimSpace(after))
			if found == nil {
				return ""
			}
			return found[1]
		}
	}
	return ""
}

// The test a marker marks is the first thing named before a bracket on the lines that follow it —
// `func TestX(`, `def test_x(`, `fn x(`, `void x(`, `check_x(`.
var named = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*\(`)

// How far after a marker the test it marks may be, in lines: a marker and its test are adjacent,
// and a wider window would attach a marker to something further down the file.
const window = 4

// scanMarkers reads every marker in the tree and checks the correspondence in both directions: one
// case, one test. It returns the map it checked, which is only emitted when nothing is wrong.
func scanMarkers(root string, descriptions []description) ([]pair, []Finding) {
	var pairs []pair
	var findings []Finding

	live := map[string]string{}   // a live case, and the description declaring it
	struck := map[string]string{} // a struck case, and the same
	for _, d := range descriptions {
		for _, c := range d.cases {
			if c.struck {
				struck[c.id] = d.path
			} else {
				live[c.id] = d.path
			}
		}
	}

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
		if strings.HasSuffix(d.Name(), ".std.md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil || bytes.IndexByte(content, 0) >= 0 {
			return err
		}
		pairs = append(pairs, markersIn(relative(root, path), string(content))...)
		return nil
	})
	if err != nil {
		findings = append(findings, Finding{Msg: fmt.Sprintf("yoke-verify: %v", err)})
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].id != pairs[j].id {
			return pairs[i].id < pairs[j].id
		}
		return pairs[i].file < pairs[j].file
	})

	carried := map[string][]pair{}
	for _, p := range pairs {
		carried[p.id] = append(carried[p.id], p)
	}

	for _, p := range pairs {
		switch {
		case p.test == "":
			findings = append(findings, Finding{File: p.file,
				Msg: fmt.Sprintf("the marker %s names no test; a marker sits on the test performing the case", p.id)})
		case struck[p.id] != "":
			findings = append(findings, Finding{File: p.file,
				Msg: fmt.Sprintf("the marker %s names a struck case, declared in %s; a struck case keeps its number and takes no test", p.id, struck[p.id])})
		case live[p.id] == "":
			findings = append(findings, Finding{File: p.file,
				Msg: fmt.Sprintf("the marker %s names a case no description declares", p.id)})
		}
	}

	for id, ps := range carried {
		if len(ps) < 2 {
			continue
		}
		var where []string
		for _, p := range ps {
			where = append(where, fmt.Sprintf("%s (%s)", p.file, p.test))
		}
		findings = append(findings, Finding{File: ps[0].file,
			Msg: fmt.Sprintf("%d tests carry the marker %s: %s; one case is performed by one test", len(ps), id, strings.Join(where, ", "))})
	}

	ids := make([]string, 0, len(live))
	for id := range live {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if len(carried[id]) == 0 {
			findings = append(findings, Finding{File: live[id],
				Msg: fmt.Sprintf("no test carries the marker %s; a case reaches a report through its test", id)})
		}
	}

	return pairs, findings
}

// markersIn reads one file's markers, each with the test it marks.
func markersIn(file, content string) []pair {
	var pairs []pair
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		id := markerOn(line)
		if id == "" {
			continue
		}
		pairs = append(pairs, pair{id: id, test: testAfter(lines, i), file: file})
	}
	return pairs
}

func testAfter(lines []string, from int) string {
	for i := from + 1; i < len(lines) && i <= from+window; i++ {
		if markerOn(lines[i]) != "" {
			return ""
		}
		if found := named.FindStringSubmatch(lines[i]); found != nil {
			return found[1]
		}
	}
	return ""
}
