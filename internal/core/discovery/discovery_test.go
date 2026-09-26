package discovery_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yoke-project/yoke/internal/core/discovery"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/trunk"
)

// manifestOf is a valid Manifest of the plugin id, declaring one stream.
func manifestOf(id, stream string) string {
	return fmt.Sprintf("manifest: 1\nid: %s\nprotocol: 1\nstreams: [ { id: %s } ]\ncapabilities: [ { name: publish, governs: { stream: %s } } ]\n", id, stream, stream)
}

func write(t *testing.T, dir, id, document string) {
	t.Helper()
	os.MkdirAll(filepath.Join(dir, id), 0o755)
	if err := os.WriteFile(filepath.Join(dir, id, "manifest.yaml"), []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
}

func digestOf(document string) string {
	sum := sha256.Sum256([]byte(document))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func newRegistry(t *testing.T) *registry.Registry {
	t.Helper()
	r, err := registry.Open(filepath.Join(t.TempDir(), registry.File))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func logger() (*slog.Logger, *bytes.Buffer) {
	logged := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(logged, nil)), logged
}

func declaredAs(t *testing.T, r *registry.Registry, id string) (registry.Plugin, bool) {
	t.Helper()
	p, found, err := r.Plugin(id)
	if err != nil {
		t.Fatal(err)
	}
	return p, found
}

// std: yoke:discovery.01
func TestAScanDeclaresEveryManifestItFinds(t *testing.T) {
	dir := t.TempDir()
	documents := map[string]string{
		"com.example.a": manifestOf("com.example.a", "a.data"),
		"com.example.b": manifestOf("com.example.b", "b.data"),
	}
	for id, document := range documents {
		write(t, dir, id, document)
	}
	reg := newRegistry(t)
	log, _ := logger()
	d := discovery.New(dir, reg, log)
	d.Scan()
	for id, document := range documents {
		p, found := declaredAs(t, reg, id)
		if !found || p.Protocol != 1 || p.ManifestDigest != digestOf(document) {
			t.Errorf("%s is declared as %+v (found %v)", id, p, found)
		}
		if !d.Available(id) {
			t.Errorf("%s is not available", id)
		}
	}
}

// std: yoke:discovery.02
func TestAManifestThatFailsItsCheckIsLoggedAndSkipped(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "com.example.good", manifestOf("com.example.good", "g"))
	write(t, dir, "com.example.broken", "manifest: [unclosed")
	write(t, dir, "com.example.old", manifestOf("com.example.old", "o")+"endpoint: /run/old.sock\n")
	reg := newRegistry(t)
	log, logged := logger()
	d := discovery.New(dir, reg, log)
	d.Scan()
	if _, found := declaredAs(t, reg, "com.example.good"); !found || !d.Available("com.example.good") {
		t.Error("the valid plugin was not declared")
	}
	for id, what := range map[string]string{"com.example.broken": "document.malformed", "com.example.old": "manifest.field.removed"} {
		if _, found := declaredAs(t, reg, id); found || d.Available(id) {
			t.Errorf("%s was declared", id)
		}
		path := filepath.Join(dir, id, "manifest.yaml")
		if !strings.Contains(logged.String(), path) || !strings.Contains(logged.String(), what) {
			t.Errorf("nothing logged names %s and %s:\n%s", path, what, logged)
		}
	}
}

// std: yoke:discovery.03
func TestALaterScanReconcilesInBothDirections(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "com.example.a", manifestOf("com.example.a", "first"))
	write(t, dir, "com.example.b", manifestOf("com.example.b", "b"))
	reg := newRegistry(t)
	log, _ := logger()
	d := discovery.New(dir, reg, log)
	d.Scan()
	reg.Disable("com.example.b", "davide")

	changed := manifestOf("com.example.a", "second")
	write(t, dir, "com.example.a", changed)
	os.RemoveAll(filepath.Join(dir, "com.example.b"))
	write(t, dir, "com.example.c", manifestOf("com.example.c", "c"))
	d.Scan()

	if _, found := declaredAs(t, reg, "com.example.c"); !found || !d.Available("com.example.c") {
		t.Error("a new Manifest was not declared")
	}
	if p, _ := declaredAs(t, reg, "com.example.a"); p.ManifestDigest != digestOf(changed) {
		t.Error("a changed Manifest's digest was not written")
	}
	if m, ok := d.Manifest("com.example.a"); !ok || m.Streams[0].ID != "second" {
		t.Errorf("discovery holds %+v for the changed Manifest", m)
	}
	p, found := declaredAs(t, reg, "com.example.b")
	if !found || p.Enabled {
		t.Errorf("the gone plugin's record is %+v (found %v)", p, found)
	}
	if h, _ := reg.History("com.example.b"); len(h) != 1 {
		t.Errorf("the gone plugin's history holds %d decisions", len(h))
	}
	if d.Available("com.example.b") {
		t.Error("a Manifest that is gone is still available")
	}
}

// deployment is a service-form trunk over dir, with the Plugin directory, executables and composition
// the test writes.
type deployment struct {
	dir, manifests, executables, composition string
}

func newDeployment(t *testing.T, plugins ...string) deployment {
	t.Helper()
	dir := t.TempDir()
	d := deployment{dir: dir, manifests: filepath.Join(dir, "plugins.d"), executables: filepath.Join(dir, "plugins"), composition: filepath.Join(dir, "bench.yaml")}
	os.MkdirAll(d.manifests, 0o755)
	os.MkdirAll(d.executables, 0o755)
	for _, id := range plugins {
		write(t, d.manifests, id, manifestOf(id, "data"))
		os.WriteFile(filepath.Join(d.executables, id), []byte("#!/bin/sh\n"), 0o755)
	}
	return d
}

// ready runs the trunk up to readiness, launching nothing.
func (d deployment) ready(t *testing.T, composition string) (*trunk.State, *bytes.Buffer) {
	t.Helper()
	os.WriteFile(d.composition, []byte(composition), 0o644)
	core := filepath.Join(d.dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s/run\nplugins:\n  manifests: %s\n  executables: %s\n", d.dir, d.dir, d.manifests, d.executables)), 0o644)
	env := map[string]string{"YOKE_CONFIG": core}
	logged := &bytes.Buffer{}
	st := &trunk.State{Form: trunk.Service, Env: func(k string) string { return env[k] }, Stderr: logged, Composition: d.composition}
	steps := trunk.Steps()
	if err := trunk.Run(st, steps[:len(steps)-1]); err != nil {
		t.Fatalf("the trunk did not reach readiness: %v\n%s", err, logged)
	}
	t.Cleanup(func() { st.Stop() })
	return st, logged
}

// std: yoke:discovery.04
func TestAPluginInstalledAndNotComposedIsPresentAndIdle(t *testing.T) {
	d := newDeployment(t, "com.example.composed", "com.example.idle")
	st, logged := d.ready(t, "units:\n  worker: { kind: plugin, plugin: com.example.composed }\n")
	for _, id := range []string{"com.example.composed", "com.example.idle"} {
		if _, found := declaredAs(t, st.Registry, id); !found {
			t.Errorf("%s is not declared:\n%s", id, logged)
		}
	}
	var handed []string
	for _, u := range st.Units {
		handed = append(handed, u.ID)
	}
	if !slices.Equal(handed, []string{"worker"}) {
		t.Errorf("the units handed over are %v, want [worker]", handed)
	}
}

// std: yoke:discovery.05
func TestTheCompositionSaysWhatRuns(t *testing.T) {
	d := newDeployment(t, "com.example.acquire")
	heads := t.TempDir()
	for _, head := range []string{"a", "b"} {
		os.WriteFile(filepath.Join(heads, head), nil, 0o644)
	}
	d.manifestNeeds(t, "com.example.acquire", "device:instrument")
	st, logged := d.ready(t, fmt.Sprintf(`
units:
  acquire-1: { kind: plugin, plugin: com.example.acquire, bind: { instrument: %[1]s/a }, args: [ "--device", "${bind.instrument}" ] }
  acquire-2: { kind: plugin, plugin: com.example.acquire, bind: { instrument: %[1]s/b }, args: [ "--device", "${bind.instrument}" ] }
  later: { kind: oneshot, exec: /bin/true, autostart: false }
`, heads))
	if len(st.Units) != 2 {
		t.Fatalf("the units handed over are %+v\n%s", st.Units, logged)
	}
	for i, u := range st.Units {
		want := fmt.Sprintf("acquire-%d", i+1)
		device := filepath.Join(heads, map[int]string{0: "a", 1: "b"}[i])
		if u.ID != want || u.Plugin != "com.example.acquire" || u.Exec != filepath.Join(d.executables, "com.example.acquire") || !slices.Equal(u.Args, []string{"--device", device}) {
			t.Errorf("unit %d is %+v", i, u)
		}
	}
}

// manifestNeeds rewrites a plugin's Manifest to declare the needs given.
func (d deployment) manifestNeeds(t *testing.T, id string, needs ...string) {
	write(t, d.manifests, id, manifestOf(id, "data")+"needs: [ \""+strings.Join(needs, "\", \"")+"\" ]\n")
}

// std: yoke:discovery.06
func TestACompositionTheGateRefusesIsReported(t *testing.T) {
	d := newDeployment(t, "com.example.present")
	st, logged := d.ready(t, "units:\n  ghost: { kind: plugin, plugin: com.example.absent }\n")
	if !strings.Contains(logged.String(), "plugin.manifest.missing") || !strings.Contains(logged.String(), "units.ghost.plugin") {
		t.Errorf("the refusal was not reported:\n%s", logged)
	}
	if _, found := declaredAs(t, st.Registry, "com.example.present"); !found {
		t.Error("the valid plugin was not declared")
	}
	if len(st.Units) != 0 {
		t.Errorf("units were handed over from a refused composition: %+v", st.Units)
	}
}

// std: yoke:discovery.07
func TestTheScanRunsAgainAtTheConfiguredInterval(t *testing.T) {
	dir := t.TempDir()
	reg := newRegistry(t)
	log, _ := logger()
	d := discovery.New(dir, reg, log)
	stop := d.Every(100 * time.Millisecond)
	defer stop()
	write(t, dir, "com.example.late", manifestOf("com.example.late", "l"))
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, found := declaredAs(t, reg, "com.example.late"); found && d.Available("com.example.late") {
			return
		}
	}
	t.Fatal("a Manifest written into the directory was not declared within a second")
}

// std: yoke:discovery.08
func TestTheCoreDeclaresWhatItFindsAndRunsWhatTheCompositionSays(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	d := newDeployment(t, "com.example.good")
	write(t, d.manifests, "com.example.broken", "manifest: [unclosed")
	os.WriteFile(d.composition, []byte("units:\n  hello: { kind: oneshot, exec: /bin/echo, args: [ from-the-oneshot ] }\n"), 0o644)
	core := filepath.Join(d.dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s/run\nplugins:\n  manifests: %s\n  executables: %s\n", d.dir, d.dir, d.manifests, d.executables)), 0o644)
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+d.composition)
	out, _ := command.StderrPipe()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	var said []string
	isReady, echoed := false, false
	deadline := time.After(15 * time.Second)
	for !isReady || !echoed {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			isReady = isReady || strings.Contains(line, "msg=ready")
			echoed = echoed || (strings.Contains(line, "from-the-oneshot") && strings.Contains(line, "unit=hello"))
		case <-deadline:
			t.Fatalf("within fifteen seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
	if all := strings.Join(said, "\n"); !strings.Contains(all, filepath.Join(d.manifests, "com.example.broken", "manifest.yaml")) {
		t.Errorf("nothing named the Manifest that did not parse:\n%s", all)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(d.dir, "state", registry.File)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT id FROM plugin ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	if !slices.Equal(ids, []string{"com.example.good"}) {
		t.Errorf("the Registry holds %v, want [com.example.good]", ids)
	}
}
