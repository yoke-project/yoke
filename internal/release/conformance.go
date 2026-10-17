package release

import (
	"bufio"
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

// A family, by the repository its harnesses are built in and the name its publication lines carry in the
// manifest, in the order the report lists them.
var families = []struct{ repository, published string }{
	{"yoke-sdk-go", "github.com/yoke-project/yoke-sdk-go"},
	{"yoke-sdk-rust", "crates.io/yoke-sdk"},
	{"yoke-sdk-python", "pypi.org/yoke-sdk"},
	{"yoke-sdk-c", "yoke-sdk-c-source"},
	{"yoke-sdk-cpp", "yoke-sdk-cpp-source"},
}

// Measure is one family publication the suite is run against: the family's repository, and the version
// and commit of its newest publication line.
type Measure struct {
	Repository string `json:"repository"`
	Version    string `json:"version"`
	Commit     string `json:"commit"`
}

// Measured reads the manifest and names, for each family that published, its newest publication — what a
// programs release runs the suite against. A family with no publication is not measured.
func Measured(manifest []byte) ([]Measure, error) {
	newest := map[string]Measure{}
	lines := bufio.NewScanner(bytes.NewReader(manifest))
	lines.Buffer(make([]byte, 1<<20), 1<<20)
	for lines.Scan() {
		if len(bytes.TrimSpace(lines.Bytes())) == 0 {
			continue
		}
		var l Line
		if err := json.Unmarshal(lines.Bytes(), &l); err != nil {
			return nil, fmt.Errorf("a line of the manifest cannot be read: %v", err)
		}
		if l.Line != "publication" || !semver.IsValid(l.Version) {
			continue
		}
		for _, f := range families {
			if f.published == l.Published {
				if held, ok := newest[f.repository]; !ok || semver.Compare(l.Version, held.Version) > 0 {
					newest[f.repository] = Measure{Repository: f.repository, Version: l.Version, Commit: l.Commit}
				}
			}
		}
	}
	if err := lines.Err(); err != nil {
		return nil, err
	}
	var measured []Measure
	for _, f := range families {
		if m, ok := newest[f.repository]; ok {
			measured = append(measured, m)
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
