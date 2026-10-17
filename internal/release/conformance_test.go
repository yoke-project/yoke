package release_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/release"
)

// std: yoke:the-conformance-report.01
func TestWhatIsMeasuredIsEachFamilysNewestPublication(t *testing.T) {
	manifest := strings.Join([]string{
		`{"line":"publication","published":"github.com/yoke-project/yoke-sdk-go","version":"v0.5.0","commit":"g5"}`,
		`{"line":"publication","published":"crates.io/yoke-sdk","version":"v0.4.1","commit":"r41"}`,
		`{"line":"publication","published":"github.com/yoke-project/yoke-sdk-go","version":"v0.10.0","commit":"g10"}`,
		`{"line":"publication","published":"crates.io/yoke-proto","version":"v0.9.0","commit":"p9"}`,
		`{"line":"publication","published":"yoke-sdk-c-source","version":"v0.5.0","commit":"c5"}`,
		`{"line":"publication","published":"github.com/yoke-project/yoke-sdk-go","version":"v0.6.0","commit":"g6"}`,
		`{"line":"publication","published":"crates.io/yoke-sdk","version":"v0.3.0","commit":"r3"}`,
		`{"line":"publication","published":"yoke-conformance-linux-amd64","version":"v0.7.0","commit":"y7"}`,
		`{"line":"publication","published":"github.com/yoke-project/yoke","version":"v0.7.0","commit":"y7"}`,
	}, "\n") + "\n"
	measured, err := release.Measured([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	want := []release.Measure{
		{Repository: "yoke-sdk-go", Version: "v0.10.0", Commit: "g10"},
		{Repository: "yoke-sdk-rust", Version: "v0.4.1", Commit: "r41"},
		{Repository: "yoke-sdk-c", Version: "v0.5.0", Commit: "c5"},
	}
	if len(measured) != len(want) {
		t.Fatalf("measured %+v", measured)
	}
	for i := range want {
		if measured[i] != want[i] {
			t.Errorf("entry %d is %+v, want %+v", i, measured[i], want[i])
		}
	}
}

// measuredAt writes one measured family's directory: what was measured, its reports and, where its run
// stopped, why.
func measuredAt(t *testing.T, dir string, m release.Measure, reports map[string]string, why string) {
	t.Helper()
	at := filepath.Join(dir, m.Repository)
	os.MkdirAll(filepath.Join(at, "reports"), 0o755)
	data, _ := json.Marshal(m)
	os.WriteFile(filepath.Join(at, "measured.json"), data, 0o644)
	for name, report := range reports {
		os.WriteFile(filepath.Join(at, "reports", name), []byte(report), 0o644)
	}
	if why != "" {
		os.WriteFile(filepath.Join(at, "error"), []byte(why+"\n"), 0o644)
	}
}

const passing = `{"ran":{"core":"yoke-core","core_version":"v0.8.0","harnesses":[{"contract":"plugin","language":"go","sdk":"yoke-sdk-go 0.6.0","version":1,"unit":""}]},"cells":[{"case":"yoke:plugin.01","language":"go","result":"pass"}],"ok":true}`

// std: yoke:the-conformance-report.02
func TestTheJoinedReportHoldsEachFamilysReports(t *testing.T) {
	dir := t.TempDir()
	measuredAt(t, dir, release.Measure{Repository: "yoke-sdk-go", Version: "v0.6.0", Commit: "g6"},
		map[string]string{"yoke-go-plugin-harness.json": passing, "yoke-go-admin-harness.json": passing}, "")
	measuredAt(t, dir, release.Measure{Repository: "yoke-sdk-rust", Version: "v0.4.1", Commit: "r41"}, nil, "the harness does not build")
	measuredAt(t, dir, release.Measure{Repository: "yoke-sdk-c", Version: "v0.5.0", Commit: "c5"}, nil, "")
	order := []release.Measure{{Repository: "yoke-sdk-go"}, {Repository: "yoke-sdk-rust"}, {Repository: "yoke-sdk-c"}}
	data, err := release.Join(dir, "v0.8.0", order)
	if err != nil {
		t.Fatal(err)
	}
	var joined struct {
		Version  string `json:"version"`
		Families []struct {
			Repository string            `json:"repository"`
			Version    string            `json:"version"`
			Commit     string            `json:"commit"`
			Reports    []json.RawMessage `json:"reports"`
			Error      string            `json:"error"`
		} `json:"families"`
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(data, &joined); err != nil {
		t.Fatalf("the joined report is not JSON: %v\n%s", err, data)
	}
	if joined.Version != "v0.8.0" || len(joined.Families) != 3 || joined.OK {
		t.Fatalf("joined %s", data)
	}
	g, r, c := joined.Families[0], joined.Families[1], joined.Families[2]
	if g.Repository != "yoke-sdk-go" || g.Version != "v0.6.0" || g.Commit != "g6" || len(g.Reports) != 2 || g.Error != "" ||
		!strings.Contains(string(g.Reports[0]), `"yoke:plugin.01"`) {
		t.Errorf("the Go family's entry is %+v", g)
	}
	if r.Repository != "yoke-sdk-rust" || r.Commit != "r41" || len(r.Reports) != 0 || r.Error != "the harness does not build" {
		t.Errorf("the Rust family's entry is %+v", r)
	}
	if c.Repository != "yoke-sdk-c" || len(c.Reports) != 0 || !strings.Contains(c.Error, "wrote no report") {
		t.Errorf("the C family's entry is %+v", c)
	}
}

// std: yoke:the-conformance-report.03
func TestAProgramsTagHandsTheReportOverBesideItsFiles(t *testing.T) {
	root := withCommands(t, "v0.1.0")
	dir := t.TempDir()
	go6 := release.Measure{Repository: "yoke-sdk-go", Version: "v0.6.0", Commit: "g6"}
	measuredAt(t, dir, go6, map[string]string{"yoke-go-plugin-harness.json": passing}, "")
	plan, _ := json.Marshal([]release.Measure{go6})
	os.WriteFile(filepath.Join(dir, "measured.json"), plan, 0o644)
	s := &shelf{}
	serve := func(m, v string) (string, error) {
		if m == "example.com/yk/proto" {
			return treeHash(t, root, m, v, "proto/"+v, "proto"), nil
		}
		return treeHash(t, root, m, v, v, ""), nil
	}
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Out: &out, Err: &errs, Proxy: serve,
		Today:     func() time.Time { return time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC) },
		Artifacts: []release.Artifact{{Name: "yk-tools", Packages: []string{"./cmd/a"}}},
		Releases:  "https://forge.example/yk/releases/tag/", Upload: s.upload,
		Conformance: dir})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs.String())
	}
	report, handed := s.files["yoke-conformance-report-0.1.0.json"]
	if !handed || !strings.Contains(string(report), `"yoke-sdk-go"`) || !strings.Contains(string(report), `"yoke:plugin.01"`) {
		t.Fatalf("the upload held %v; the report: %s", keys(s.files), report)
	}
	sum := sha256.Sum256(report)
	_, lines, _, _ := parse(t, out.String())
	named := 0
	for _, l := range lines {
		digests, _ := l["digests"].([]any)
		if l["published"] == "yoke-conformance-report" && len(digests) == 1 && digests[0] == "sha256:"+hex.EncodeToString(sum[:]) &&
			l["where"] == "https://forge.example/yk/releases/tag/v0.1.0" {
			named++
		}
	}
	if named != 1 {
		t.Errorf("%d lines name the report: %s", named, out.String())
	}
}
