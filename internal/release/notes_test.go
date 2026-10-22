package release_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/release"
)

// forge keeps the descriptions it is given.
type forge struct {
	bodies    map[string]string
	asked     []string
	described int
	order     *[]string
}

func (f *forge) Description(tag string) (string, error) {
	f.asked = append(f.asked, tag)
	return f.bodies[tag], nil
}

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

// std: yoke:the-release-description.01
func TestTheVerbsPartIsTheComparisonBetweenTheTwoTags(t *testing.T) {
	notes := release.Notes("yoke-project/yoke", "v0.4.0", "v0.5.0")
	if !strings.HasPrefix(notes, "**Full changelog:** [`v0.4.0...v0.5.0`](https://github.com/yoke-project/yoke/compare/v0.4.0...v0.5.0)\n<!-- merged: sha256:") {
		t.Errorf("the comparison is not the part:\n%s", notes)
	}
	if strings.Count(notes, "\n") != 2 || strings.Contains(notes, "What was merged") {
		t.Errorf("the part says more than the comparison:\n%s", notes)
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
	f := &forge{}
	describe(t, root, "v0.3.0", f)
	if !strings.Contains(f.bodies["v0.3.0"], "compare/v0.2.0...v0.3.0") {
		t.Errorf("the description:\n%s", f.bodies["v0.3.0"])
	}
	describe(t, root, "v0.1.0", f)
	if _, described := f.bodies["v0.1.0"]; described || len(f.asked) != 1 {
		t.Errorf("the first release was asked about (%v) or described:\n%s", f.asked, f.bodies["v0.1.0"])
	}
}

// std: yoke:the-release-description.03
func TestADescriptionIsRewrittenOnlyAsTheVerbLeftIt(t *testing.T) {
	root, _ := repository(t, "v0.1.0", "v0.3.0")
	f := &forge{}
	describe(t, root, "v0.3.0", f)
	written := f.bodies["v0.3.0"]

	// Unchanged, and asked again: nothing is written.
	describe(t, root, "v0.3.0", f)
	if f.described != 1 {
		t.Errorf("an unchanged description was written again, %d writes", f.described)
	}

	// A programs tag between the two, placed since: the comparison moves, and the command's part stays.
	git(t, root, "tag", "v0.2.0")
	published := "\n## What was merged since 0.2.0\n\n## Published with this release\n\n- the programs\n"
	f.bodies["v0.3.0"] = written + published
	describe(t, root, "v0.3.0", f)
	if got := f.bodies["v0.3.0"]; !strings.Contains(got, "v0.2.0...v0.3.0") || !strings.HasSuffix(got, published) {
		t.Errorf("the part was not replaced with the section kept:\n%s", got)
	}

	for what, body := range map[string]string{
		"edited":       strings.Replace(f.bodies["v0.3.0"], "Full changelog", "Every change", 1),
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
	root := withCommands(t, "v0.0.9")
	git(t, root, "commit", "-q", "--allow-empty", "-m", "two")
	git(t, root, "tag", "v0.1.0")
	git(t, root, "tag", "proto/v0.1.0")
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
	if len(f.asked) != 1 || f.asked[0] != "v0.1.0" {
		t.Errorf("the forge was asked about %v", f.asked)
	}
	if !strings.Contains(f.bodies["v0.1.0"], "compare/v0.0.9...v0.1.0") {
		t.Errorf("the description:\n%s", f.bodies["v0.1.0"])
	}
}
