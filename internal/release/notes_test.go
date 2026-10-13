package release_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/release"
)

// forge answers what was merged, and keeps the descriptions it is given.
type forge struct {
	pulls     []release.Pull
	asked     [][2]string
	bodies    map[string]string
	described int
	order     *[]string
}

func (f *forge) Merged(previous, tag string) ([]release.Pull, error) {
	f.asked = append(f.asked, [2]string{previous, tag})
	return f.pulls, nil
}

func (f *forge) Description(tag string) (string, error) { return f.bodies[tag], nil }

func (f *forge) Describe(tag, body string) error {
	if f.bodies == nil {
		f.bodies = map[string]string{}
	}
	f.bodies[tag], f.described = body, f.described+1
	if f.order != nil {
		*f.order = append(*f.order, "described "+tag)
	}
	return nil
}

func task(n int) release.Issue {
	return release.Issue{Repository: "yoke-project/yoke", Number: n, Type: "Task"}
}

// std: yoke:the-release-description.01
func TestWhatWasMergedIsGroupedByTheTypeOfTheIssuesClosed(t *testing.T) {
	notes := release.Notes("yoke-project/yoke", "v0.4.0", "v0.5.0", []release.Pull{
		{Number: 270, Title: "yoke app run", Closes: []release.Issue{task(229)}},
		{Number: 271, Title: "A start that never happened", Closes: []release.Issue{{Repository: "yoke-project/yoke", Number: 267, Type: "Bug"}, task(229)}},
		{Number: 272, Title: "A comment reworded"},
		{Number: 273, Title: "A family follows", Closes: []release.Issue{{Repository: "yoke-project/yoke-sdk-go", Number: 60, Type: "Feature"}}},
		{Number: 274, Title: "Planned elsewhere", Closes: []release.Issue{{Repository: "yoke-project/yoke-ledger", Number: 9, Type: "Task", Private: true}}},
	})
	section := func(heading string) string {
		_, rest, _ := strings.Cut(notes, "### "+heading+"\n")
		body, _, _ := strings.Cut(rest, "\n\n")
		return body
	}
	for heading, want := range map[string][]string{
		"Changes":              {"- yoke app run (#270, closes #229)", "- A family follows (#273, closes yoke-project/yoke-sdk-go#60)", "- Planned elsewhere (#274)"},
		"Fixes":                {"- A start that never happened (#271, closes #267, #229)"},
		"Not tied to an issue": {"- A comment reworded (#272)"},
	} {
		got := strings.Split(strings.TrimSpace(section(heading)), "\n")
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("%s is\n%s\nwant\n%s", heading, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
	if !strings.Contains(notes, "## What was merged since 0.4.0") ||
		!strings.Contains(notes, "[`v0.4.0...v0.5.0`](https://github.com/yoke-project/yoke/compare/v0.4.0...v0.5.0)") {
		t.Errorf("the heading or the comparison is missing:\n%s", notes)
	}
	if strings.Contains(notes, "yoke-ledger") {
		t.Errorf("the notes name a private repository:\n%s", notes)
	}
}

func describe(t *testing.T, root, tag string, f *forge) string {
	t.Helper()
	var errs bytes.Buffer
	if err := (release.Config{Root: root, Repository: "yoke-project/yoke", Forge: f, Err: &errs}).Describe(tag); err != nil {
		t.Fatalf("describing %s: %v", tag, err)
	}
	return errs.String()
}

// std: yoke:the-release-description.02
func TestThePreviousProgramsTagIsTheGreatestLowerOne(t *testing.T) {
	root, _ := repository(t, "v0.1.0", "v0.2.0", "proto/v0.9.0", "v0.3.0")
	f := &forge{pulls: []release.Pull{{Number: 1, Title: "One", Closes: []release.Issue{task(2)}}}}
	describe(t, root, "v0.3.0", f)
	if len(f.asked) != 1 || f.asked[0] != [2]string{"v0.2.0", "v0.3.0"} {
		t.Errorf("the forge was asked %v", f.asked)
	}
	if !strings.Contains(f.bodies["v0.3.0"], "compare/v0.2.0...v0.3.0") {
		t.Errorf("the description:\n%s", f.bodies["v0.3.0"])
	}
	describe(t, root, "v0.1.0", f)
	if f.asked[1] != [2]string{"", "v0.1.0"} || strings.Contains(f.bodies["v0.1.0"], "compare/") {
		t.Errorf("the first release was asked %v and described\n%s", f.asked[1], f.bodies["v0.1.0"])
	}
}

// std: yoke:the-release-description.03
func TestADescriptionIsRewrittenOnlyAsTheVerbLeftIt(t *testing.T) {
	root, _ := repository(t, "v0.2.0", "v0.3.0")
	f := &forge{pulls: []release.Pull{{Number: 1, Title: "One", Closes: []release.Issue{task(2)}}}}
	describe(t, root, "v0.3.0", f)
	written := f.bodies["v0.3.0"]

	// Unchanged, and asked again: nothing is written.
	describe(t, root, "v0.3.0", f)
	if f.described != 1 {
		t.Errorf("an unchanged description was written again, %d writes", f.described)
	}

	f.pulls = append(f.pulls, release.Pull{Number: 3, Title: "Three", Closes: []release.Issue{task(4)}})
	published := "\n## Published with this release\n\n- the programs\n"
	f.bodies["v0.3.0"] = written + published
	describe(t, root, "v0.3.0", f)
	if got := f.bodies["v0.3.0"]; !strings.Contains(got, "Three (#3") || !strings.HasSuffix(got, published) {
		t.Errorf("the part was not replaced with the section kept:\n%s", got)
	}

	for what, body := range map[string]string{
		"edited":       strings.Replace(f.bodies["v0.3.0"], "Three", "Three, reworded", 1),
		"hand-written": "## What changed\n\nWritten by hand.\n",
	} {
		f.bodies["v0.3.0"], f.described = body, 0
		said := describe(t, root, "v0.3.0", f)
		if f.described != 0 || f.bodies["v0.3.0"] != body || !strings.Contains(said, "left as it is") {
			t.Errorf("a description %s was rewritten (%d), or the run said %q", what, f.described, said)
		}
	}
}

// std: yoke:the-release-description.04
func TestAProgramsTagsRunDescribesItsReleaseAfterTheFiles(t *testing.T) {
	root := withCommands(t, "v0.1.0", "proto/v0.1.0")
	var order []string
	f := &forge{order: &order}
	s := &shelf{}
	upload := func(tag string, files []string) error {
		order = append(order, "uploaded "+tag)
		return s.upload(tag, files)
	}
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Out: &out, Err: &errs,
		Proxy: func(m, v string) (string, error) {
			if m == "example.com/yk/proto" {
				return treeHash(t, root, m, v, "proto/"+v, "proto"), nil
			}
			return treeHash(t, root, m, v, v, ""), nil
		},
		Today:     func() time.Time { return time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC) },
		Artifacts: []release.Artifact{{Name: "yk-tools", Packages: []string{"./cmd/a"}}},
		Source:    "yk", Releases: "https://forge.example/yk/releases/tag/", Upload: upload,
		Forge: f, Repository: "example/yk"})
	if code != 0 {
		t.Fatalf("the run exited %d:\n%s", code, errs.String())
	}
	if strings.Join(order, ", ") != "uploaded v0.1.0, described v0.1.0" {
		t.Errorf("the order was %v", order)
	}
	if _, asked := f.bodies["proto/v0.1.0"]; asked || len(f.asked) != 1 {
		t.Errorf("the forge was asked about %v", f.asked)
	}
	if !strings.Contains(f.bodies["v0.1.0"], "What was merged up to 0.1.0") {
		t.Errorf("the description:\n%s", f.bodies["v0.1.0"])
	}
}
