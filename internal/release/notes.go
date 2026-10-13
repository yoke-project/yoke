package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// Pull is one pull request merged since the previous programs tag, and the issues it closes.
type Pull struct {
	Number int
	Title  string
	Closes []Issue
}

// Issue is an issue a pull request closes: where it is, whether that repository is private, and its
// type — `Bug`, `Task`, `Feature`, or none.
type Issue struct {
	Repository string // owner/name
	Number     int
	Private    bool
	Type       string
}

// Forge is what the verb asks of the forge to describe a programs release: what was merged between two
// tags, and the release's description as it stands and as it is replaced.
type Forge interface {
	Merged(previous, tag string) ([]Pull, error)
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

// Notes is the verb's part of a programs release's description: what was merged, grouped by the type
// of the issues each pull request closes, and the comparison between the two tags. A pull request
// closing a `Bug` is a fix, one closing a `Task` or a `Feature` is a change, and one closing none is
// listed apart; an issue in a private repository counts and is not named. It ends with the mark.
func Notes(repository, previous, tag string, pulls []Pull) string {
	var changes, fixes, apart []string
	for _, p := range pulls {
		bug, typed := false, false
		var named []string
		for _, i := range p.Closes {
			bug = bug || i.Type == "Bug"
			typed = typed || i.Type == "Bug" || i.Type == "Task" || i.Type == "Feature"
			switch {
			case i.Private:
			case i.Repository == repository:
				named = append(named, fmt.Sprintf("#%d", i.Number))
			default:
				named = append(named, fmt.Sprintf("%s#%d", i.Repository, i.Number))
			}
		}
		line := fmt.Sprintf("- %s (#%d", strings.TrimSpace(p.Title), p.Number)
		if len(named) > 0 {
			line += ", closes " + strings.Join(named, ", ")
		}
		line += ")"
		switch {
		case bug:
			fixes = append(fixes, line)
		case typed:
			changes = append(changes, line)
		default:
			apart = append(apart, line)
		}
	}
	var b strings.Builder
	if previous == "" {
		fmt.Fprintf(&b, "## What was merged up to %s\n", strings.TrimPrefix(tag, "v"))
	} else {
		fmt.Fprintf(&b, "## What was merged since %s\n", strings.TrimPrefix(previous, "v"))
	}
	for _, group := range []struct {
		heading string
		lines   []string
	}{{"Changes", changes}, {"Fixes", fixes}, {"Not tied to an issue", apart}} {
		if len(group.lines) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n### %s\n\n%s\n", group.heading, strings.Join(group.lines, "\n"))
	}
	if len(pulls) == 0 {
		b.WriteString("\nNothing was merged.\n")
	}
	if previous != "" {
		fmt.Fprintf(&b, "\n**Full changelog:** [`%s...%s`](https://github.com/%s/compare/%s...%s)\n", previous, tag, repository, previous, tag)
	}
	fmt.Fprintf(&b, "<!-- merged: %s -->\n", digestOf(b.String()))
	return b.String()
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
	if err != nil {
		return err
	}
	pulls, err := cfg.Forge.Merged(previous, tag)
	if err != nil {
		return fmt.Errorf("what was merged since %s cannot be read: %w", previous, err)
	}
	current, err := cfg.Forge.Description(tag)
	if err != nil {
		return err
	}
	body, ok := described(current, Notes(cfg.Repository, previous, tag, pulls))
	if !ok {
		fmt.Fprintf(cfg.Err, "release: the description of %s was edited since it was written, and is left as it is\n", tag)
		return nil
	}
	if body == current {
		return nil
	}
	return cfg.Forge.Describe(tag, body)
}

// GitHub is the forge `yoke` is on, asked through `gh`: the pull requests that merged the commits
// between two tags, each with the issues it closes, read from the forge's own records.
type GitHub struct {
	Repository string // owner/name
	Root       string // the checkout, which holds both tags
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

func (g GitHub) Merged(previous, tag string) ([]Pull, error) {
	span := tag
	if previous != "" {
		span = previous + ".." + tag
	}
	listed, err := gitIn(g.Root, "log", "--format=%H", span)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	var pulls []Pull
	for _, commit := range strings.Fields(listed) {
		out, err := g.gh("api", "repos/"+g.Repository+"/commits/"+commit+"/pulls", "--jq", ".[] | select(.merged_at != null) | .number")
		if err != nil {
			return nil, err
		}
		for _, n := range strings.Fields(string(out)) {
			number, err := strconv.Atoi(n)
			if err != nil || seen[number] {
				continue
			}
			seen[number] = true
			p, err := g.pull(number)
			if err != nil {
				return nil, err
			}
			pulls = append(pulls, p)
		}
	}
	// Oldest first, as they were merged.
	for i, j := 0, len(pulls)-1; i < j; i, j = i+1, j-1 {
		pulls[i], pulls[j] = pulls[j], pulls[i]
	}
	return pulls, nil
}

const pullQuery = `query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      title
      closingIssuesReferences(first: 20) {
        nodes { number issueType { name } repository { nameWithOwner isPrivate } }
      }
    }
  }
}`

func (g GitHub) pull(number int) (Pull, error) {
	owner, name, _ := strings.Cut(g.Repository, "/")
	out, err := g.gh("api", "graphql", "-f", "query="+pullQuery, "-F", "owner="+owner, "-F", "name="+name, "-F", "number="+strconv.Itoa(number))
	if err != nil {
		return Pull{}, err
	}
	var answer struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					Title   string
					Closing struct {
						Nodes []struct {
							Number     int
							IssueType  *struct{ Name string } `json:"issueType"`
							Repository struct {
								NameWithOwner string
								IsPrivate     bool
							}
						}
					} `json:"closingIssuesReferences"`
				} `json:"pullRequest"`
			}
		}
	}
	if err := json.Unmarshal(out, &answer); err != nil {
		return Pull{}, err
	}
	pr := answer.Data.Repository.PullRequest
	p := Pull{Number: number, Title: pr.Title}
	for _, n := range pr.Closing.Nodes {
		i := Issue{Repository: n.Repository.NameWithOwner, Number: n.Number, Private: n.Repository.IsPrivate}
		if n.IssueType != nil {
			i.Type = n.IssueType.Name
		}
		p.Closes = append(p.Closes, i)
	}
	return p, nil
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
