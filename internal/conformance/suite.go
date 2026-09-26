// Package conformance is the suite: one program that drives a real Core, never a fixture, and measures
// a library through its harness.
//
// A run describes, composes, starts, drives and reports. It launches the harness once to describe
// itself, writes `core.yaml`, a composition naming the harness as a unit and the Manifest it described
// into a temporary tree, executes the Core and waits to be told it is ready, runs its cases against the
// harness the Core launched, and stops everything and removes the tree. A harness is reached over a
// line-oriented control protocol on a Unix socket whose path it finds in CONFORMANCE_SOCKET. The table
// holds pass, fail or absent per case and language, and absent is a failure.
package conformance

import (
	"bufio"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"
)

// The three values a cell can hold. There is no fourth.
const (
	cellPass   = "pass"
	cellFail   = "fail"
	cellAbsent = "absent"
)

// Case is one case of a contract: what it cites, what it issues, what it requires, and how it is run.
type Case struct {
	ID           string
	Title        string
	Contract     string
	Cites        []string
	Precondition string
	Issues       string
	Requires     string
	Run          func(*Run) Outcome
}

// Outcome is a case's result, and on a failure what was issued, required and observed.
type Outcome struct {
	Result string
	Detail string
}

// Pass is a case that held.
func Pass() Outcome { return Outcome{Result: cellPass} }

// Fail is a case that did not hold: the directive it issued, what it required, and what it observed.
func Fail(directive, required, observed string) Outcome {
	return Outcome{Result: cellFail, Detail: fmt.Sprintf("issued %s; required %s; observed %s", directive, required, observed)}
}

// Absent is a case the library does not implement: its harness did not recognise the verb.
func Absent(directive string) Outcome {
	return Outcome{Result: cellAbsent, Detail: fmt.Sprintf("the harness does not recognise %s", directive)}
}

// Config is what a run is given.
type Config struct {
	Core        string // the yoke-core binary
	Harness     string // the harness binary
	HarnessEnv  map[string]string
	Cases       []Case
	ReadyWithin time.Duration // how long the Core has to say it is ready; 30 s by default
	Out         io.Writer
}

// Hello is what a harness says first.
type Hello struct {
	Contract string `json:"contract"`
	Language string `json:"language"`
	SDK      string `json:"sdk"`
	Version  int    `json:"version"`
	Unit     string `json:"unit"`
}

// Row is one cell of the table: a case, under a language.
type Row struct {
	Case     string
	Language string
	Result   string
	Detail   string
}

// Ran is the record of what ran, so a table is answerable a year later.
type Ran struct {
	Core        string
	CoreVersion string
	Harnesses   []Hello
}

// Report is a run's table and its record.
type Report struct {
	Rows []Row
	Ran  Ran
	OK   bool // every cell a pass
}

// Main runs the suite and returns its exit status: zero only when every cell passes.
func Main(cfg Config) int {
	report, err := Execute(cfg)
	if err != nil {
		fmt.Fprintln(cfg.Out, "yoke-conformance:", err)
		return 1
	}
	if !report.OK {
		return 1
	}
	return 0
}

// Execute performs one run.
func Execute(cfg Config) (Report, error) {
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	if cfg.ReadyWithin == 0 {
		cfg.ReadyWithin = 30 * time.Second
	}
	report := Report{Ran: Ran{Core: cfg.Core, CoreVersion: coreVersion(cfg.Core)}}
	// The tree is kept short: every socket path of the instance is derived under it.
	tree, err := os.MkdirTemp("", "yc")
	if err != nil {
		return report, err
	}
	defer os.RemoveAll(tree)
	control, err := listen(filepath.Join(tree, "control.sock"))
	if err != nil {
		return report, err
	}
	defer control.close()

	// Describe: the harness, launched by the suite, returns the Manifest its library generates.
	describing, err := control.launch(cfg)
	if err != nil {
		return report, err
	}
	h, err := control.next("", 15*time.Second)
	if err != nil {
		describing.Process.Kill()
		return report, fmt.Errorf("the harness did not say hello: %w", err)
	}
	report.Ran.Harnesses = append(report.Ran.Harnesses, h.Hello())
	described := h.Do("describe", nil)
	h.finish()
	describing.Wait()
	text, _ := described.Value["manifest"].(string)
	var head struct{ ID string }
	if err := yaml.Unmarshal([]byte(text), &head); err != nil || head.ID == "" {
		return report, fmt.Errorf("the harness described no Manifest with an identity: %q", text)
	}

	// Compose.
	paths := map[string]string{}
	for _, d := range []string{"run", "state", "plugins.d", "plugins"} {
		paths[d] = filepath.Join(tree, d)
		os.MkdirAll(paths[d], 0o755)
	}
	os.MkdirAll(filepath.Join(paths["plugins.d"], head.ID), 0o755)
	if err := os.WriteFile(filepath.Join(paths["plugins.d"], head.ID, "manifest.yaml"), []byte(text), 0o644); err != nil {
		return report, err
	}
	harness, _ := filepath.Abs(cfg.Harness)
	if err := os.Symlink(harness, filepath.Join(paths["plugins"], head.ID)); err != nil {
		return report, err
	}
	coreYAML, _ := yaml.Marshal(map[string]any{"state_dir": paths["state"], "runtime_dir": paths["run"],
		"plugins": map[string]any{"manifests": paths["plugins.d"], "executables": paths["plugins"]}})
	env := map[string]string{"CONFORMANCE_SOCKET": control.path}
	for k, v := range cfg.HarnessEnv {
		env[k] = v
	}
	composition, _ := yaml.Marshal(map[string]any{"units": map[string]any{"harness": map[string]any{"kind": "plugin", "plugin": head.ID, "env": env}}})
	os.WriteFile(filepath.Join(tree, "core.yaml"), coreYAML, 0o644)
	os.WriteFile(filepath.Join(tree, "composition.yaml"), composition, 0o644)

	// Start, and be told when the Core is ready.
	core := exec.Command(cfg.Core, "--composition", filepath.Join(tree, "composition.yaml"))
	core.Env = append(os.Environ(), "YOKE_CONFIG="+filepath.Join(tree, "core.yaml"))
	stderr, _ := core.StderrPipe()
	if err := core.Start(); err != nil {
		return report, err
	}
	defer stop(core)
	said, ready := watch(stderr)
	select {
	case <-ready:
	case <-time.After(cfg.ReadyWithin):
		return report, fmt.Errorf("the Core was not ready within %v; it said:\n%s", cfg.ReadyWithin, strings.Join(said(), "\n"))
	}

	// Drive.
	run := &Run{tree: tree, control: control, described: text}
	for _, c := range cfg.Cases {
		outcome := c.Run(run)
		language := ""
		if run.unit != nil {
			language = run.unit.Hello().Language
		} else if len(report.Ran.Harnesses) > 0 {
			language = report.Ran.Harnesses[0].Language
		}
		report.Rows = append(report.Rows, Row{Case: c.ID, Language: language, Result: outcome.Result, Detail: outcome.Detail})
	}
	if run.unit != nil {
		report.Ran.Harnesses = append(report.Ran.Harnesses, run.unit.Hello())
		run.unit.finish()
	}

	// Report.
	report.OK = true
	fmt.Fprintf(cfg.Out, "ran  core %s %s\n", report.Ran.Core, report.Ran.CoreVersion)
	for _, h := range report.Ran.Harnesses {
		fmt.Fprintf(cfg.Out, "ran  harness contract=%s version=%d language=%s sdk=%q unit=%q\n", h.Contract, h.Version, h.Language, h.SDK, h.Unit)
	}
	for _, row := range report.Rows {
		// The case leads the row, so no row begins as a result line does.
		fmt.Fprintf(cfg.Out, "%-24s %-10s %s\n", row.Case, row.Language, row.Result)
	}
	for _, row := range report.Rows {
		if row.Result == cellPass {
			fmt.Fprintf(cfg.Out, "pass  %s\n", row.Case)
		} else {
			report.OK = false
			fmt.Fprintf(cfg.Out, "FAIL  %s — %s: %s\n", row.Case, row.Result, row.Detail)
		}
	}
	return report, nil
}

// coreVersion is the module version and commit the Core binary was built from.
func coreVersion(path string) string {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	version := info.Main.Version
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			version += "@" + s.Value
		}
	}
	return version
}

// watch reads the Core's error stream, and closes ready when it says it is ready.
func watch(r io.Reader) (func() []string, <-chan struct{}) {
	var mu sync.Mutex
	var lines []string
	ready := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(r)
		once := sync.Once{}
		for scanner.Scan() {
			mu.Lock()
			lines = append(lines, scanner.Text())
			mu.Unlock()
			if strings.Contains(scanner.Text(), "msg=ready") {
				once.Do(func() { close(ready) })
			}
		}
	}()
	return func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), lines...) }, ready
}

func stop(core *exec.Cmd) {
	core.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() { core.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		core.Process.Kill()
		<-done
	}
}

// Run is what a case drives.
type Run struct {
	tree      string
	control   *control
	unit      *Harness
	described string
}

// Described is the Manifest the harness described at the start of the run.
func (r *Run) Described() string { return r.described }

// NextUnit waits for the harness of the unit's next life, once the Core has launched it again.
func (r *Run) NextUnit(within time.Duration) (*Harness, error) {
	h, err := r.control.next("unit", within)
	if err != nil {
		return nil, err
	}
	r.unit = h
	return h, nil
}

// Tree is the run's temporary tree.
func (r *Run) Tree() string { return r.tree }

// Unit is the harness the Core launched as a unit, once it has said hello.
func (r *Run) Unit() (*Harness, error) {
	if r.unit != nil {
		return r.unit, nil
	}
	h, err := r.control.next("unit", 30*time.Second)
	if err != nil {
		return nil, err
	}
	r.unit = h
	return h, nil
}

// control is the suite's control socket, and the harnesses connected to it.
type control struct {
	path     string
	listener net.Listener
	hellos   chan *Harness
}

func listen(path string) (*control, error) {
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	c := &control{path: path, listener: l, hellos: make(chan *Harness, 8)}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go c.greet(conn)
		}
	}()
	return c, nil
}

func (c *control) close() { c.listener.Close() }

// launch starts the harness out of band, as the suite does to have it describe itself.
func (c *control) launch(cfg Config) (*exec.Cmd, error) {
	cmd := exec.Command(cfg.Harness)
	cmd.Env = append(os.Environ(), "CONFORMANCE_SOCKET="+c.path)
	for k, v := range cfg.HarnessEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	return cmd, cmd.Start()
}

// greet reads a connection's hello and hands the harness over.
func (c *control) greet(conn net.Conn) {
	h := &Harness{conn: conn, lines: make(chan message, 64)}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	if !scanner.Scan() {
		conn.Close()
		return
	}
	var first message
	if err := json.Unmarshal(scanner.Bytes(), &first); err != nil || first.Type != "hello" {
		conn.Close()
		return
	}
	h.hello = first.Hello
	go func() {
		defer close(h.lines)
		for scanner.Scan() {
			var m message
			if json.Unmarshal(scanner.Bytes(), &m) == nil {
				h.lines <- m
			}
		}
	}()
	c.hellos <- h
}

// next waits for a harness: "unit" for one the Core launched, "" for one the suite launched.
func (c *control) next(which string, within time.Duration) (*Harness, error) {
	deadline := time.After(within)
	for {
		select {
		case h := <-c.hellos:
			if (which == "unit") == (h.hello.Unit != "") {
				return h, nil
			}
			h.finish()
		case <-deadline:
			return nil, errors.New("no harness said hello within " + within.String())
		}
	}
}

// message is one line of the control protocol, in either direction.
type message struct {
	Type string `json:"type"`
	Hello
	ID           string         `json:"id,omitempty"`
	Verb         string         `json:"verb,omitempty"`
	Args         map[string]any `json:"args,omitempty"`
	Value        map[string]any `json:"value,omitempty"`
	Refusal      string         `json:"refusal,omitempty"`
	Unrecognised bool           `json:"unrecognised,omitempty"`
	Kind         string         `json:"kind,omitempty"`
	Fields       map[string]any `json:"fields,omitempty"`
}

// Observation is what a library surfaced without being asked.
type Observation struct {
	Kind   string
	Fields map[string]any
}

// Result is a harness's answer to a directive, and what it observed before answering.
type Result struct {
	Value        map[string]any
	Refusal      string // the code, never a message
	Unrecognised bool
	Before       []Observation
	Lost         bool // the harness went away before answering
}

// Harness is one connected harness.
type Harness struct {
	conn    net.Conn
	hello   Hello
	lines   chan message
	next    int
	pending []Observation
	mu      sync.Mutex
}

// Hello is what the harness said first.
func (h *Harness) Hello() Hello { return h.hello }

// Do issues a directive and waits for its result.
func (h *Harness) Do(verb string, args map[string]any) Result {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.next++
	id := fmt.Sprintf("d-%d", h.next)
	b, _ := json.Marshal(message{Type: "directive", ID: id, Verb: verb, Args: args})
	h.conn.Write(append(b, '\n'))
	res := Result{Before: h.pending}
	h.pending = nil
	timeout := time.After(30 * time.Second)
	for {
		select {
		case m, open := <-h.lines:
			if !open {
				res.Lost = true
				return res
			}
			switch {
			case m.Type == "observation":
				res.Before = append(res.Before, Observation{Kind: m.Kind, Fields: m.Fields})
			case m.Type == "result" && m.ID == id:
				res.Value, res.Refusal, res.Unrecognised = m.Value, m.Refusal, m.Unrecognised
				return res
			}
		case <-timeout:
			res.Lost = true
			return res
		}
	}
}

// Gone says whether the harness has closed its connection — its process finishing — within the bound.
func (h *Harness) Gone(within time.Duration) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	deadline := time.After(within)
	for {
		select {
		case m, open := <-h.lines:
			if !open {
				return true
			}
			if m.Type == "observation" {
				h.pending = append(h.pending, Observation{Kind: m.Kind, Fields: m.Fields})
			}
		case <-deadline:
			return false
		}
	}
}

// Observe waits for the next thing the library surfaces unasked.
func (h *Harness) Observe(within time.Duration) (Observation, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.pending) > 0 {
		o := h.pending[0]
		h.pending = h.pending[1:]
		return o, true
	}
	deadline := time.After(within)
	for {
		select {
		case m, open := <-h.lines:
			if !open {
				return Observation{}, false
			}
			if m.Type == "observation" {
				return Observation{Kind: m.Kind, Fields: m.Fields}, true
			}
		case <-deadline:
			return Observation{}, false
		}
	}
}

// finish tells the harness to stop and lets it leave by itself — the connection closing is its going —
// within a bound, so that nothing around it is stopped while it is still finishing.
func (h *Harness) finish() {
	b, _ := json.Marshal(message{Type: "finish"})
	h.conn.Write(append(b, '\n'))
	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, open := <-h.lines:
			if !open {
				h.conn.Close()
				return
			}
		case <-deadline:
			h.conn.Close()
			return
		}
	}
}
