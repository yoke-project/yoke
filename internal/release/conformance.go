package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/semver"
)

// The families, by the repository their harnesses are built in, in the order the report lists them.
var families = []string{"yoke-sdk-go", "yoke-sdk-rust", "yoke-sdk-python", "yoke-sdk-c", "yoke-sdk-cpp"}

// Measure is one family release the suite is run against: the family's repository, and the version and
// commit of its newest release tag.
type Measure struct {
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
}

// Measured reads the tags each repository carries, as an object of repository name to the list of its
// tags, each a name and the commit it names, and names for each family its newest release tag — what a
// programs release runs the suite against. A tag is a release's when its name is a version; a family with
// none, and a repository that is no family, are not measured.
func Measured(tags []byte) ([]Measure, error) {
	var carried map[string][]struct {
		Name   string `json:"name"`
		Commit string `json:"commit"`
	}
	if err := json.Unmarshal(tags, &carried); err != nil {
		return nil, fmt.Errorf("the tags cannot be read: %v", err)
	}
	var measured []Measure
	for _, repository := range families {
		var newest *Measure
		for _, tag := range carried[repository] {
			if !semver.IsValid(tag.Name) {
				continue
			}
			if newest == nil || semver.Compare(tag.Name, newest.Version) > 0 {
				newest = &Measure{Repository: repository, Version: tag.Name, Commit: tag.Commit}
			}
		}
		if newest != nil {
			measured = append(measured, *newest)
		}
	}
	return measured, nil
}

// joined is the report a release carries: per family measured, the reports its harnesses' runs wrote, or
// why there are none.
type joined struct {
	Version  string   `json:"version"`
	Families []family `json:"families"`
	OK       bool     `json:"ok"`
}

type family struct {
	Measure
	Reports []json.RawMessage `json:"reports"`
	Error   string            `json:"error,omitempty"`
}

// Join reads the directory a release's measuring left — one directory per family, holding what was
// measured, the reports under reports/, and an error file where the run stopped — and joins the reports
// into the one the release carries, in the order measured. A family that wrote no report and gave no
// reason is named as one whose script wrote none.
func Join(dir, version string, order []Measure) ([]byte, error) {
	j := joined{Version: version, Families: []family{}, OK: true}
	for _, m := range order {
		at := filepath.Join(dir, m.Repository)
		f := family{Measure: m, Reports: []json.RawMessage{}}
		if data, err := os.ReadFile(filepath.Join(at, "measured.json")); err == nil {
			if err := json.Unmarshal(data, &f.Measure); err != nil {
				return nil, fmt.Errorf("what was measured of %s cannot be read: %v", m.Repository, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		names, err := filepath.Glob(filepath.Join(at, "reports", "*.json"))
		if err != nil {
			return nil, err
		}
		slices.Sort(names)
		for _, name := range names {
			data, err := os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			var r struct {
				OK bool `json:"ok"`
			}
			if err := json.Unmarshal(data, &r); err != nil {
				return nil, fmt.Errorf("the report %s of %s is not JSON: %v", filepath.Base(name), m.Repository, err)
			}
			f.Reports = append(f.Reports, json.RawMessage(bytes.TrimSpace(data)))
			j.OK = j.OK && r.OK
		}
		if why, err := os.ReadFile(filepath.Join(at, "error")); err == nil {
			f.Error = strings.TrimSpace(string(why))
		} else if len(f.Reports) == 0 {
			f.Error = "its conformance script wrote no report"
		}
		if f.Error != "" {
			j.OK = false
		}
		j.Families = append(j.Families, f)
	}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Planned reads what a release's measuring set out to measure, written as measured.json at the top of
// the directory it left, so a family whose run left nothing is still listed and named.
func Planned(dir string) ([]Measure, error) {
	data, err := os.ReadFile(filepath.Join(dir, "measured.json"))
	if err != nil {
		return nil, fmt.Errorf("what was to be measured cannot be read: %v", err)
	}
	var order []Measure
	if err := json.Unmarshal(data, &order); err != nil {
		return nil, fmt.Errorf("what was to be measured cannot be read: %v", err)
	}
	return order, nil
}
