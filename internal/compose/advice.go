package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// Advice is the composer's class of finding: is this what you meant. The gate never raises it.
const Advice gate.Class = "advice"

// Advisories is the record of the advisory items a project waived, beside its descriptor and never
// packaged.
const Advisories = "advisories.yaml"

// The advisory set as it stands. An entry is added when a trap has been met rather than imagined.
const (
	RetentionBudgetSmall    = "retention.budget.small"
	OneshotRestartUnchecked = "oneshot.restart.unchecked"
	ArbitrationPanelStarved = "arbitration.panel.starved"
	ParameterLabelOverclaim = "parameter.label.overclaims"
	DataRangeUnstated       = "data.range.unstated"
)

var advisorySet = []string{RetentionBudgetSmall, OneshotRestartUnchecked, ArbitrationPanelStarved, ParameterLabelOverclaim, DataRangeUnstated}

// A retention figure under a tenth of its default is small.
const (
	smallAge     = 24 * time.Hour
	smallBytes   = 5_000_000
	smallEntries = 10_000
)

// protective are the words a label claims a protection with. The list is part of the entry, and a word is
// added when a label carrying it misled somebody.
var protective = []string{"only", "secure", "safe", "private", "protected", "isolated", "restricted", "encrypted", "authenticated"}

var reference = regexp.MustCompile(`\$\{([^}]*)\}`)

// Advise is the gate's report on the project in dir and the advisory findings on it, which are raised
// only where the gate refused nothing.
func Advise(dir string) (gate.Report, []gate.Finding) {
	r, d := check(dir)
	if r.Refused() || d == nil {
		return r, nil
	}
	raw := map[string]any{}
	if data, err := os.ReadFile(filepath.Join(dir, Descriptor)); err == nil {
		yaml.Unmarshal(data, &raw)
	}
	var out []gate.Finding
	advise := func(code, location, format string, args ...any) {
		out = append(out, gate.Finding{Code: code, Class: Advice, Document: filepath.Join(dir, Descriptor), Location: location, Message: fmt.Sprintf(format, args...)})
	}
	for _, name := range sortedNames(d.Units) {
		u := d.Units[name]
		if u.Kind == "plugin" {
			for _, f := range []struct {
				figure string
				small  bool
			}{
				{"age", u.Policy.RetentionAge > 0 && u.Policy.RetentionAge < smallAge},
				{"bytes", u.Policy.RetentionBytes > 0 && u.Policy.RetentionBytes < smallBytes},
				{"entries", u.Policy.RetentionEntries > 0 && u.Policy.RetentionEntries < smallEntries},
			} {
				if !f.small {
					continue
				}
				at := "policy.retention." + f.figure
				if writes(raw, "units", name, "policy", "retention", f.figure) {
					at = "units." + name + "." + at
				}
				advise(RetentionBudgetSmall, at, "the unit %s keeps under a tenth of the default %s of its own history, and cleaning evicts its own group first: the history somebody diagnosing it will want", name, f.figure)
			}
		}
		if u.Kind == "oneshot" && u.OnFailure {
			advise(OneshotRestartUnchecked, "units."+name+".policy.restart.on_failure", "the oneshot %s is restarted on failure, so the deployment will run it twice; whether that is safe is a judgement only its author can make", name)
		}
		if d.Head.Data == "" && slices.ContainsFunc(u.Needs, func(n gate.Need) bool { return n.Class == "storage" }) {
			advise(DataRangeUnstated, "units."+name, "the unit %s keeps storage while the head states no data range, so the first update puts new code in front of old state with nothing checked", name)
		}
	}
	for i, rule := range d.Arbitration {
		if prevails, ok := d.Channels[rule.Prevails]; !ok || localSocket(prevails) {
			continue
		}
		for _, over := range rule.Over {
			if c, ok := d.Channels[over]; ok && localSocket(c) {
				advise(ArbitrationPanelStarved, fmt.Sprintf("arbitration.%d", i), "the channel %s, not a local socket, suspends the local socket %s: the operator at the instrument finds the panel suspended by something remote", rule.Prevails, over)
				break
			}
		}
	}
	for _, name := range sortedNames(d.Channels) {
		c := d.Channels[name]
		if localSocket(c) || c.Address.Class == "routable" && c.Address.Caller == "authenticated" {
			continue
		}
		for _, m := range reference.FindAllStringSubmatch(c.Address.Port, -1) {
			group := name
			p, ok := d.Parameters[group][m[1]]
			if !ok {
				group = "shared"
				p, ok = d.Parameters[group][m[1]]
			}
			if word := claims(p.Label); ok && word != "" {
				advise(ParameterLabelOverclaim, "parameters."+group+"."+m[1]+".label", "the label of %s.%s says %q of the channel %s, which is bound on %s and protects nothing the word could mean", group, m[1], word, name, c.Address.Class)
			}
		}
	}
	return r, out
}

func localSocket(c gate.Channel) bool { return c.Address == nil || c.Address.Class == "local" }

// claims is the protective word a label holds, as a whole word in any case, or "".
func claims(label string) string {
	for _, w := range strings.FieldsFunc(strings.ToLower(label), func(r rune) bool { return !unicode.IsLetter(r) }) {
		if slices.Contains(protective, w) {
			return w
		}
	}
	return ""
}

// writes says whether the document writes the field at the path.
func writes(doc map[string]any, path ...string) bool {
	var v any = doc
	for _, p := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		if v, ok = m[p]; !ok {
			return false
		}
	}
	return true
}

func sortedNames[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Waiver is one advisory item a project silenced: the code, the place, why, and when.
type Waiver struct {
	Code     string `yaml:"code"`
	At       string `yaml:"at"`
	Reason   string `yaml:"reason"`
	WaivedAt string `yaml:"waived_at"`
	// Stale is whether it matches nothing advised now. It is read, and never written.
	Stale bool `yaml:"-"`
}

type waivers struct {
	Waived []Waiver `yaml:"waived"`
}

func readWaivers(dir string) (waivers, error) {
	var w waivers
	data, err := os.ReadFile(filepath.Join(dir, Advisories))
	if errors.Is(err, fs.ErrNotExist) {
		return w, nil
	}
	if err != nil {
		return w, err
	}
	if err := yaml.Unmarshal(data, &w); err != nil {
		return w, refusef("%s does not parse: %v", filepath.Join(dir, Advisories), err)
	}
	return w, nil
}

// Waive records that the project in dir silences the advisory code at a place, for a reason. The code
// must be one of the set, the reason not empty, the code advised at that place now, and not waived there
// already.
func Waive(dir, code, at, reason string, now time.Time) error {
	if !slices.Contains(advisorySet, code) {
		return refusef("%s is not an advisory; the set is %s", code, strings.Join(advisorySet, ", "))
	}
	if strings.TrimSpace(reason) == "" {
		return refusef("a waiver carries a reason, and the value of the record is in it")
	}
	_, advised := Advise(dir)
	if !slices.ContainsFunc(advised, func(f gate.Finding) bool { return f.Code == code && f.Location == at }) {
		return refusef("%s is not advised at %s, and a waiver of nothing would be stale on the day it was written", code, at)
	}
	w, err := readWaivers(dir)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(w.Waived, func(v Waiver) bool { return v.Code == code && v.At == at }) {
		return refusef("%s at %s is already waived", code, at)
	}
	w.Waived = append(w.Waived, Waiver{Code: code, At: at, Reason: reason, WaivedAt: now.UTC().Format(time.RFC3339)})
	var out strings.Builder
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(w); err != nil {
		return err
	}
	enc.Close()
	path := filepath.Join(dir, Advisories)
	if err := os.WriteFile(path+".writing", []byte(out.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(path+".writing", path)
}

// Waivers is every waiver the project in dir holds, in the order written, each marked stale where it
// matches nothing advised now.
func Waivers(dir string) ([]Waiver, error) {
	w, err := readWaivers(dir)
	if err != nil {
		return nil, err
	}
	_, advised := Advise(dir)
	for i, v := range w.Waived {
		w.Waived[i].Stale = !slices.ContainsFunc(advised, func(f gate.Finding) bool { return f.Code == v.Code && f.Location == v.At })
	}
	return w.Waived, nil
}

// Unwaived is the advice a project's waivers leave in the report, and how many items they silenced.
func Unwaived(dir string, advised []gate.Finding) ([]gate.Finding, int, error) {
	w, err := readWaivers(dir)
	if err != nil {
		return nil, 0, err
	}
	var out []gate.Finding
	waived := 0
	for _, f := range advised {
		if slices.ContainsFunc(w.Waived, func(v Waiver) bool { return v.Code == f.Code && v.At == f.Location }) {
			waived++
			continue
		}
		out = append(out, f)
	}
	return out, waived, nil
}
