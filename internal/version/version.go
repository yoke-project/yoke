// Package version reads an application's version and a range of versions, and orders them.
//
// A version is a semantic version, read strictly: three numeric parts without leading zeros, an optional
// pre-release and optional build metadata, and nothing else — no `v` and no shorthand. A range is
// comparators separated by spaces, all of which must hold; a version inside a comparator may be written
// with fewer than three parts, standing for its zeros.
package version

import (
	"debug/buildinfo"
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// The grammar of semver.org 2.0.0, unchanged.
var strict = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// Version is a semantic version.
type Version struct {
	raw string
}

// Parse reads s as a semantic version, strictly.
func Parse(s string) (Version, error) {
	if !strict.MatchString(s) {
		return Version{}, fmt.Errorf("%q is not a semantic version", s)
	}
	return Version{raw: s}, nil
}

// MustParse is Parse for a version known to be valid, and panics on one that is not.
func MustParse(s string) Version {
	v, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return v
}

// String is the version as it was written.
func (v Version) String() string { return v.raw }

// Compare returns -1, 0 or 1 as v sorts below, with or above w. Build metadata takes no part.
func (v Version) Compare(w Version) int { return semver.Compare("v"+v.raw, "v"+w.raw) }

// IsPrerelease says whether v carries a pre-release.
func (v Version) IsPrerelease() bool { return semver.Prerelease("v"+v.raw) != "" }

type comparator struct {
	op string
	v  Version
}

func (c comparator) holds(v Version) bool {
	n := v.Compare(c.v)
	switch c.op {
	case ">=":
		return n >= 0
	case ">":
		return n > 0
	case "<=":
		return n <= 0
	case "<":
		return n < 0
	default:
		return n == 0
	}
}

// Range is a range expression over versions.
type Range struct {
	raw         string
	comparators []comparator
}

// ParseRange reads s as a range: comparators separated by spaces, each an operator — `>=`, `>`, `<=`, `<`
// or `=` — followed by a version of one, two or three parts.
func ParseRange(s string) (Range, error) {
	r := Range{raw: s}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return Range{}, fmt.Errorf("%q is not a range: it has no comparator", s)
	}
	for _, f := range fields {
		op := ""
		for _, o := range []string{">=", "<=", ">", "<", "="} {
			if strings.HasPrefix(f, o) {
				op = o
				break
			}
		}
		if op == "" {
			return Range{}, fmt.Errorf("%q is not a range: %q has no operator", s, f)
		}
		v, ok := partial(strings.TrimPrefix(f, op))
		if !ok {
			return Range{}, fmt.Errorf("%q is not a range: %q is not a version", s, f)
		}
		r.comparators = append(r.comparators, comparator{op, v})
	}
	return r, nil
}

// partial reads a version of one, two or three parts, the absent ones zero.
func partial(s string) (Version, bool) {
	if v, err := Parse(s); err == nil {
		return v, true
	}
	parts := strings.Split(s, ".")
	if len(parts) > 2 {
		return Version{}, false
	}
	for _, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 64); err != nil || (len(p) > 1 && p[0] == '0') {
			return Version{}, false
		}
	}
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	v, err := Parse(strings.Join(parts, "."))
	return v, err == nil
}

// String is the range as it was written.
func (r Range) String() string { return r.raw }

// Contains says whether every comparator of the range holds for v.
func (r Range) Contains(v Version) bool {
	for _, c := range r.comparators {
		if !c.holds(v) {
			return false
		}
	}
	return len(r.comparators) > 0
}

// OfBinary is the version a Go program states in the build information stamped into it, read without
// executing it: what a Core will run as, before it runs.
func OfBinary(path string) (Version, error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return Version{}, fmt.Errorf("the program %s states no version: %v", path, err)
	}
	return stated(path, info.Main.Version)
}

// Running is the version this program states of itself.
func Running() (Version, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return Version{}, errors.New("this program carries no build information")
	}
	return stated("this program", info.Main.Version)
}

func stated(what, v string) (Version, error) {
	parsed, err := Parse(strings.TrimPrefix(v, "v"))
	if err != nil {
		return Version{}, fmt.Errorf("%s states no version a range can hold: %q", what, v)
	}
	return parsed, nil
}
