package release

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"golang.org/x/mod/semver"
)

// Forge is what the verb asks of the forge to describe a programs release: the release's description as
// it stands, and as it is replaced.
type Forge interface {
	Description(tag string) (string, error)
	Describe(tag, body string) error
}

// mark ends the part of a description the verb writes, naming the digest of what precedes it; what
// follows it is another party's.
var mark = regexp.MustCompile(`(?m)^<!-- merged: (sha256:[0-9a-f]{64}) -->\n?`)

// previousTag is the programs tag before the one given: the greatest `vX.Y.Z` among the tree's tags
// that is lower; empty where there is none.
func previousTag(root, tag string) (string, error) {
	listed, err := gitIn(root, "tag", "--list", "v*")
	if err != nil {
		return "", err
	}
	previous := ""
	for _, t := range strings.Fields(listed) {
		if semver.IsValid(t) && semver.Canonical(t) == t && semver.Compare(t, tag) < 0 &&
			(previous == "" || semver.Compare(t, previous) > 0) {
			previous = t
		}
	}
	return previous, nil
}

// Notes is the verb's part of a programs release's description: the comparison between the two tags.
// What was merged, in this repository and in every other public one, is the release command's to
// write, after it: the verb reads only its own repository, and the work of a release lands in others. It
// ends with the mark.
func Notes(repository, previous, tag string) string {
	notes := fmt.Sprintf("**Full changelog:** [`%s...%s`](https://github.com/%s/compare/%s...%s)\n", previous, tag, repository, previous, tag)
	return notes + fmt.Sprintf("<!-- merged: %s -->\n", digestOf(notes))
}

// digestOf is the digest a mark names: of the part's text, its line ends and its surrounding blank
// space being the forge's to normalise.
func digestOf(part string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(strings.ReplaceAll(part, "\r\n", "\n"))))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// described replaces the verb's part of a description with notes, keeping whatever another party wrote
// after it. A description that is empty takes the notes; one whose part is as the verb left it has the
// part replaced; any other — the part edited, or written by hand — is left as it is, and false says so.
func described(current, notes string) (string, bool) {
	if strings.TrimSpace(current) == "" {
		return notes, true
	}
	normal := strings.ReplaceAll(current, "\r\n", "\n")
	at := mark.FindStringSubmatchIndex(normal)
	if at == nil || digestOf(normal[:at[0]]) != normal[at[2]:at[3]] {
		return current, false
	}
	return notes + normal[at[1]:], true
}

// Describe writes the verb's part of the description of the release a programs tag names.
func (cfg Config) Describe(tag string) error {
	previous, err := previousTag(cfg.Root, tag)
	if err != nil || previous == "" {
		// The first programs release compares with nothing.
		return err
	}
	current, err := cfg.Forge.Description(tag)
	if err != nil {
		return err
	}
	body, ok := described(current, Notes(cfg.Repository, previous, tag))
	if !ok {
		fmt.Fprintf(cfg.Err, "release: the description of %s was edited since it was written, and is left as it is\n", tag)
		return nil
	}
	if body == current {
		return nil
	}
	return cfg.Forge.Describe(tag, body)
}

// GitHub is the forge `yoke` is on, asked through `gh`.
type GitHub struct {
	Repository string // owner/name
}

func (g GitHub) gh(args ...string) ([]byte, error) {
	out, err := exec.Command("gh", args...).Output()
	if err != nil {
		var said string
		if e, ok := err.(*exec.ExitError); ok {
			said = string(e.Stderr)
		}
		return nil, fmt.Errorf("gh %s: %v\n%s", args[0], err, said)
	}
	return out, nil
}

func (g GitHub) Description(tag string) (string, error) {
	out, err := g.gh("release", "view", tag, "-R", g.Repository, "--json", "body", "--jq", ".body")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func (g GitHub) Describe(tag, body string) error {
	cmd := exec.Command("gh", "release", "edit", tag, "-R", g.Repository, "--notes-file", "-")
	cmd.Stdin = strings.NewReader(body)
	if said, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("gh release edit: %v\n%s", err, said)
	}
	return nil
}
