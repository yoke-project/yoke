package conformance_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/conformance"
)

// The test binary is also the harness these tests drive: TEST_HARNESS says so, and TEST_HARNESS_LOG is
// where it writes down every line it received, so a test can read what reached it.
func TestMain(m *testing.M) {
	if os.Getenv("TEST_HARNESS") != "" {
		os.Exit(harness())
	}
	code := m.Run()
	if coreBinary != "" {
		os.Remove(coreBinary)
	}
	os.Exit(code)
}

const manifest = "manifest: 1\nid: com.example.harness\nprotocol: 1\n"

// harness is a harness for the suite's own tests: it says hello, describes itself, echoes, notifies,
// refuses, and recognises nothing else.
func harness() int {
	conn, err := net.Dial("unix", os.Getenv("CONFORMANCE_SOCKET"))
	if err != nil {
		return 1
	}
	log, _ := os.OpenFile(os.Getenv("TEST_HARNESS_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	send := func(v map[string]any) { b, _ := json.Marshal(v); conn.Write(append(b, '\n')) }
	send(map[string]any{"type": "hello", "contract": "plugin", "language": "go", "sdk": "test-harness 0.0.1", "version": 1, "unit": os.Getenv("YOKE_UNIT")})
	lines := bufio.NewScanner(conn)
	for lines.Scan() {
		fmt.Fprintf(log, "%s %s\n", os.Getenv("YOKE_UNIT"), lines.Text())
		var d struct {
			Type, ID, Verb string
			Args           map[string]any
		}
		json.Unmarshal(lines.Bytes(), &d)
		switch {
		case d.Type == "finish":
			// A harness takes a moment to leave, as one that closes its library's Session does.
			time.Sleep(300 * time.Millisecond)
			fmt.Fprintf(log, "%s finished\n", os.Getenv("YOKE_UNIT"))
			return 0
		case d.Verb == "describe":
			send(map[string]any{"type": "result", "id": d.ID, "value": map[string]any{"manifest": manifest}})
		case d.Verb == "echo":
			send(map[string]any{"type": "result", "id": d.ID, "value": d.Args})
		case d.Verb == "notify":
			send(map[string]any{"type": "observation", "kind": "notified", "fields": map[string]any{"why": "asked"}})
			send(map[string]any{"type": "result", "id": d.ID, "value": map[string]any{}})
		case d.Verb == "refuse":
			send(map[string]any{"type": "result", "id": d.ID, "refusal": "admission.auth.consumed"})
		default:
			send(map[string]any{"type": "result", "id": d.ID, "unrecognised": true})
		}
	}
	return 0
}

var coreBinary string

func core(t *testing.T) string {
	t.Helper()
	if coreBinary == "" {
		coreBinary = filepath.Join(os.TempDir(), fmt.Sprintf("yoke-core-suite-test-%d", os.Getpid()))
		if said, err := exec.Command("go", "build", "-o", coreBinary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
			t.Fatalf("yoke-core does not build: %v\n%s", err, said)
		}
	}
	return coreBinary
}

func config(t *testing.T, cases ...conformance.Case) (conformance.Config, string) {
	t.Helper()
	log := filepath.Join(t.TempDir(), "harness.log")
	self, _ := os.Executable()
	return conformance.Config{
		Core: core(t), Harness: self, Cases: cases, Out: &bytes.Buffer{},
		HarnessEnv: map[string]string{"TEST_HARNESS": "1", "TEST_HARNESS_LOG": log},
	}, log
}

// unitHello is a case requiring the harness the Core launched to have said hello with its unit.
var unitHello = conformance.Case{ID: "yoke:toy.01", Title: "the unit says hello", Issues: "nothing", Requires: "a hello with a unit",
	Run: func(r *conformance.Run) conformance.Outcome {
		h, err := r.Unit()
		if err != nil {
			return conformance.Fail("", "the harness the Core launched says hello", err.Error())
		}
		if h.Hello().Unit == "" {
			return conformance.Fail("", "a unit in the hello", "none")
		}
		return conformance.Pass()
	}}

// std: yoke:the-conformance-suite.01
func TestARunDescribesComposesStartsDrivesAndReports(t *testing.T) {
	var seen map[string]string
	var tree string
	inspect := conformance.Case{ID: "yoke:toy.02", Title: "the tree", Issues: "nothing", Requires: "the tree as composed",
		Run: func(r *conformance.Run) conformance.Outcome {
			tree = r.Tree()
			seen = map[string]string{}
			filepath.Walk(tree, func(path string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() && info.Mode()&os.ModeSocket == 0 {
					rel, _ := filepath.Rel(tree, path)
					b, _ := os.ReadFile(path)
					seen[rel] = string(b)
				}
				return nil
			})
			return conformance.Pass()
		}}
	cfg, _ := config(t, unitHello, inspect)
	report, err := conformance.Execute(cfg)
	if err != nil || !report.OK {
		t.Fatalf("the run gave %v and %+v\n%s", err, report, cfg.Out)
	}
	var composition, manifestFound, executable bool
	for rel, content := range seen {
		switch {
		case strings.HasSuffix(rel, "com.example.harness/manifest.yaml") && content == manifest:
			manifestFound = true
		case strings.HasSuffix(rel, filepath.Join("plugins", "com.example.harness")):
			executable = true
		case strings.Contains(content, "units:") && strings.Contains(content, "com.example.harness") && strings.Contains(content, "CONFORMANCE_SOCKET"):
			composition = true
		}
	}
	if _, found := seen["core.yaml"]; !found || !composition || !manifestFound || !executable {
		t.Errorf("the tree held %v", keys(seen))
	}
	if _, err := os.Stat(tree); !os.IsNotExist(err) {
		t.Errorf("the tree %s is still there", tree)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// std: yoke:the-conformance-suite.02
func TestTheCoresReadinessIsTold(t *testing.T) {
	cfg, _ := config(t, unitHello)
	never := filepath.Join(t.TempDir(), "never-ready")
	os.WriteFile(never, []byte("#!/bin/sh\necho starting, and nothing more >&2\nsleep 60\n"), 0o755)
	cfg.Core, cfg.ReadyWithin = never, time.Second
	out := &bytes.Buffer{}
	cfg.Out = out
	if code := conformance.Main(cfg); code == 0 {
		t.Error("a run whose Core never became ready exited zero")
	}
	if said := out.String(); !strings.Contains(said, "not ready") || !strings.Contains(said, "starting, and nothing more") || strings.Contains(said, "yoke:toy.01") {
		t.Errorf("the run said:\n%s", said)
	}
}

// std: yoke:the-conformance-suite.03
func TestTheControlProtocol(t *testing.T) {
	var observed []string
	notify := conformance.Case{ID: "yoke:toy.03", Title: "an observation", Issues: "notify", Requires: "an observation, then the result",
		Run: func(r *conformance.Run) conformance.Outcome {
			h, err := r.Unit()
			if err != nil {
				return conformance.Fail("notify", "a harness", err.Error())
			}
			res := h.Do("notify", nil)
			for _, o := range res.Before {
				observed = append(observed, o.Kind)
			}
			return conformance.Pass()
		}}
	cfg, log := config(t, notify)
	report, err := conformance.Execute(cfg)
	if err != nil || !report.OK {
		t.Fatalf("the run gave %v and %+v", err, report)
	}
	if len(observed) != 1 || observed[0] != "notified" {
		t.Errorf("the case observed %v before the result", observed)
	}
	var withUnit, describing bool
	for _, h := range report.Ran.Harnesses {
		if h.Contract != "plugin" || h.Language != "go" || h.SDK != "test-harness 0.0.1" || h.Version != 1 {
			t.Errorf("a hello was %+v", h)
		}
		withUnit = withUnit || h.Unit != ""
		describing = describing || h.Unit == ""
	}
	if !withUnit || !describing {
		t.Errorf("the hellos were %+v", report.Ran.Harnesses)
	}
	b, _ := os.ReadFile(log)
	received := string(b)
	if !strings.Contains(received, `"verb":"notify"`) || !strings.Contains(received, `"id":`) || strings.Count(received, "finished") != 2 {
		t.Errorf("the harnesses received:\n%s", received)
	}
}

// std: yoke:the-conformance-suite.04
func TestAnUnrecognisedVerbIsAbsentAndARefusalIsItsCode(t *testing.T) {
	unknown := conformance.Case{ID: "yoke:toy.04", Title: "unknown", Issues: "fly", Requires: "anything",
		Run: func(r *conformance.Run) conformance.Outcome {
			h, _ := r.Unit()
			if res := h.Do("fly", nil); res.Unrecognised {
				return conformance.Absent("fly")
			}
			return conformance.Pass()
		}}
	refused := conformance.Case{ID: "yoke:toy.05", Title: "refused", Issues: "refuse", Requires: "admission.auth.consumed",
		Run: func(r *conformance.Run) conformance.Outcome {
			h, _ := r.Unit()
			if res := h.Do("refuse", nil); res.Refusal != "admission.auth.consumed" {
				return conformance.Fail("refuse", "admission.auth.consumed", res.Refusal)
			}
			return conformance.Pass()
		}}
	cfg, _ := config(t, unknown, refused)
	report, _ := conformance.Execute(cfg)
	if report.OK || len(report.Rows) != 2 || report.Rows[0].Result != "absent" || report.Rows[1].Result != "pass" {
		t.Errorf("the report is %+v", report)
	}
}

// std: yoke:the-conformance-suite.05
func TestTheTable(t *testing.T) {
	failing := conformance.Case{ID: "yoke:toy.06", Title: "failing", Issues: "echo", Requires: "the echo of 1",
		Run: func(r *conformance.Run) conformance.Outcome {
			h, _ := r.Unit()
			res := h.Do("echo", map[string]any{"n": 2})
			return conformance.Fail("echo n=2", "n=1", fmt.Sprint(res.Value["n"]))
		}}
	unknown := conformance.Case{ID: "yoke:toy.07", Title: "unknown", Issues: "fly", Requires: "anything",
		Run: func(r *conformance.Run) conformance.Outcome {
			h, _ := r.Unit()
			h.Do("fly", nil)
			return conformance.Absent("fly")
		}}
	cfg, _ := config(t, unitHello, failing, unknown)
	out := &bytes.Buffer{}
	cfg.Out = out
	report, _ := conformance.Execute(cfg)
	want := []string{"pass", "fail", "absent"}
	for i, row := range report.Rows {
		if row.Result != want[i] || row.Language != "go" {
			t.Errorf("row %d is %+v", i, row)
		}
	}
	said := out.String()
	for _, line := range []string{"pass  yoke:toy.01", "FAIL  yoke:toy.06 — ", "FAIL  yoke:toy.07 — "} {
		if !strings.Contains(said, line) {
			t.Errorf("the run did not print %q:\n%s", line, said)
		}
	}
	// Nothing else the run prints may begin as a result does, or the record writer reads it as one.
	for _, line := range strings.Split(said, "\n") {
		verdict, _, _ := strings.Cut(line, " ")
		if (verdict == "pass" || verdict == "FAIL") && !strings.HasPrefix(line, verdict+"  yoke:") {
			t.Errorf("the run printed %q, which reads as a result", line)
		}
	}
	if d := report.Rows[1].Detail; !strings.Contains(d, "echo n=2") || !strings.Contains(d, "n=1") || !strings.Contains(d, "2") {
		t.Errorf("the failure reports %q", d)
	}
}

// std: yoke:the-conformance-suite.06
func TestTheExitStatusIsZeroOnlyWhenEveryCellPasses(t *testing.T) {
	cfg, _ := config(t, unitHello)
	if code := conformance.Main(cfg); code != 0 {
		t.Errorf("a run where every case passed exited %d:\n%s", code, cfg.Out)
	}
	absent := conformance.Case{ID: "yoke:toy.08", Title: "absent", Issues: "fly", Requires: "anything",
		Run: func(r *conformance.Run) conformance.Outcome { return conformance.Absent("fly") }}
	cfg, _ = config(t, unitHello, absent)
	if code := conformance.Main(cfg); code == 0 {
		t.Error("a run with a case absent exited zero")
	}
}

// std: yoke:the-conformance-suite.07
func TestARunSaysWhatRan(t *testing.T) {
	cfg, _ := config(t, unitHello)
	report, err := conformance.Execute(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if report.Ran.Core != cfg.Core || report.Ran.CoreVersion == "" || len(report.Ran.Harnesses) == 0 {
		t.Errorf("the run says %+v", report.Ran)
	}
	if said := cfg.Out.(*bytes.Buffer).String(); !strings.Contains(said, report.Ran.CoreVersion) || !strings.Contains(said, "test-harness 0.0.1") {
		t.Errorf("the run printed:\n%s", said)
	}
}
