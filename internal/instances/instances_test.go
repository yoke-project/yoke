package instances

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

const declarations = `parameters:
  shared:
    site_label: { type: string, required: true, label: "Label" }
  setup:
    retries: { type: integer, range: [0, 9], required: false, default: 3, label: "Retries" }
`

// installed places a version of com.yoke.station under the root, with a bundle holding a descriptor and
// a record in the state given; extra is added to its head.
func installed(t *testing.T, apps, ver string, state installroot.State, shared bool, extra string) {
	t.Helper()
	r := installroot.Root{Path: apps}
	inst, err := r.Begin("com.yoke.station", ver)
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(inst.Dir, "bundle")
	os.MkdirAll(filepath.Join(bundle, "components"), 0o775)
	os.WriteFile(filepath.Join(bundle, "components", "setup"), []byte("x"), 0o755)
	descriptor := fmt.Sprintf("model: 1\nid: com.yoke.station\nversion: %s\narch: aarch64\ncore: \">=0.1\"\n%s%s"+
		"units:\n  setup: { kind: oneshot, exec: components/setup, digest: \"sha256:%x\" }\n", ver, extra, declarations, sha256.Sum256([]byte("x")))
	os.WriteFile(filepath.Join(bundle, "yoke.yaml"), []byte(descriptor), 0o644)
	inst.Write(installroot.Record{State: state, Shared: shared})
}

func roots(t *testing.T) (string, string) {
	t.Helper()
	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	return apps, filepath.Join(t.TempDir(), "instances")
}

func values(t *testing.T, toml string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "values.toml")
	os.WriteFile(p, []byte(toml), 0o644)
	return p
}

var at = time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)

func opts(apps, instances string) Options {
	return Options{InstallRoot: apps, Instances: instances, Now: func() time.Time { return at }}
}

// std: yoke:the-instances.01
func TestAnInstanceIsCreatedPinnedToTheNewestCompleteRelease(t *testing.T) {
	apps, instances := roots(t)
	installed(t, apps, "2.0.0", installroot.Complete, true, "")
	installed(t, apps, "2.1.0", installroot.Complete, true, "")
	installed(t, apps, "2.2.0-rc.1", installroot.Complete, true, "")
	installed(t, apps, "2.3.0", installroot.Incomplete, true, "")
	name, err := Create(opts(apps, instances), "com.yoke.station", "bench-a", "", values(t, "[shared]\nsite_label = \"Bench A\"\n"))
	if err != nil || name != "bench-a" {
		t.Fatalf("created %q: %v", name, err)
	}
	r, err := instancerecord.Read(instances, "bench-a")
	if err != nil {
		t.Fatal(err)
	}
	descriptor, _ := os.ReadFile(filepath.Join(apps, "com.yoke.station", "2.1.0", "bundle", "yoke.yaml"))
	if r.Version != "2.1.0" || r.DesiredState != instancerecord.Stopped || r.Autostart || r.Owner == "" || !r.CreatedAt.Equal(at) ||
		r.Parameters["shared"]["site_label"] != "Bench A" || r.Parameters["setup"]["retries"] != "3" ||
		r.DescriptorDigest != fmt.Sprintf("sha256:%x", sha256.Sum256(descriptor)) {
		t.Errorf("the record is %+v", r)
	}
}

// std: yoke:the-instances.02
func TestANameIsGeneratedWhereNoneIsGiven(t *testing.T) {
	apps, instances := roots(t)
	installed(t, apps, "2.1.0", installroot.Complete, true, "")
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		name, err := Create(opts(apps, instances), "com.yoke.station", "", "", values(t, "[shared]\nsite_label = \"A\"\n"))
		if err != nil || name == "" || seen[name] {
			t.Fatalf("instance %d named %q: %v", i, name, err)
		}
		seen[name] = true
	}
}

// std: yoke:the-instances.03
func TestAnInstanceIsRefusedWhereItCannotBeMade(t *testing.T) {
	apps, instances := roots(t)
	installed(t, apps, "2.0.0", installroot.Incomplete, true, "")
	installed(t, apps, "2.1.0", installroot.Withdrawn, true, "")
	installed(t, apps, "2.2.0", installroot.Complete, false, "")
	installed(t, apps, "2.3.0", installroot.Complete, true, "")
	good := values(t, "[shared]\nsite_label = \"A\"\n")
	for _, c := range []struct {
		id, ver, values, names string
		stranger               bool
	}{
		{"com.yoke.other", "", good, "com.yoke.other", false},
		{"com.yoke.station", "9.9.9", good, "9.9.9", false},
		{"com.yoke.station", "2.0.0", good, "incomplete", false},
		{"com.yoke.station", "2.1.0", good, "withdrawn", false},
		{"com.yoke.station", "2.2.0", good, "not shared", true},
		{"com.yoke.station", "2.3.0", values(t, ""), "the values supplied are refused: parameter.missing", false},
		{"com.yoke.station", "2.3.0", values(t, "[shared]\nsite_label = \"A\"\n[setup]\nretries = 12\n"), "parameter.value", false},
	} {
		o := opts(apps, instances)
		if c.stranger {
			o.User = "someone-else"
		}
		_, err := Create(o, c.id, "bench", c.ver, c.values)
		if err == nil || !strings.Contains(err.Error(), c.names) {
			t.Errorf("%s %s: %v, want a refusal naming %q", c.id, c.ver, err, c.names)
		}
		if _, err := os.Stat(filepath.Join(instances, "bench")); err == nil {
			t.Fatalf("%s %s left an instance directory behind", c.id, c.ver)
		}
	}
}

// std: yoke:the-instances.04
func TestRePointingWritesThePinAndTheDigestTogether(t *testing.T) {
	apps, instances := roots(t)
	installed(t, apps, "1.9.0", installroot.Withdrawn, true, "")
	installed(t, apps, "2.0.0", installroot.Complete, true, "data: \">=1.0 <2.1\"\n")
	installed(t, apps, "2.1.0", installroot.Complete, true, "")
	installed(t, apps, "2.2.0", installroot.Complete, true, "")
	o := opts(apps, instances)
	if _, err := Create(o, "com.yoke.station", "bench-a", "2.1.0", values(t, "[shared]\nsite_label = \"A\"\n")); err != nil {
		t.Fatal(err)
	}
	state := instancerecord.StateDir(instances, "bench-a")
	os.MkdirAll(state, 0o700)
	os.WriteFile(filepath.Join(state, "version"), []byte("2.1.0\n"), 0o600)
	if err := SetVersion(o, "bench-a", "2.2.0"); err != nil {
		t.Fatal(err)
	}
	r, _ := instancerecord.Read(instances, "bench-a")
	descriptor, _ := os.ReadFile(filepath.Join(apps, "com.yoke.station", "2.2.0", "bundle", "yoke.yaml"))
	if r.Version != "2.2.0" || r.DescriptorDigest != fmt.Sprintf("sha256:%x", sha256.Sum256(descriptor)) {
		t.Errorf("re-pointed to %s with %s", r.Version, r.DescriptorDigest)
	}
	if err := SetVersion(o, "bench-a", "2.0.0"); err == nil || !strings.Contains(err.Error(), ">=1.0 <2.1") || !strings.Contains(err.Error(), "2.1.0") {
		t.Errorf("a rollback the data range refuses: %v", err)
	}
	if err := SetVersion(o, "bench-a", "1.9.0"); err == nil || !strings.Contains(err.Error(), "withdrawn") {
		t.Errorf("a withdrawn version: %v", err)
	}
	if r, _ := instancerecord.Read(instances, "bench-a"); r.Version != "2.2.0" {
		t.Errorf("a refused re-pointing changed the record to %s", r.Version)
	}
}

// std: yoke:the-instances.05
func TestReplacingOneValueIsCheckedAsAtCreation(t *testing.T) {
	apps, instances := roots(t)
	installed(t, apps, "2.1.0", installroot.Complete, true, "")
	o := opts(apps, instances)
	Create(o, "com.yoke.station", "bench-a", "", values(t, "[shared]\nsite_label = \"A\"\n"))
	if err := SetParam(o, "bench-a", "setup.retries", "5"); err != nil {
		t.Fatal(err)
	}
	r, _ := instancerecord.Read(instances, "bench-a")
	if r.Parameters["setup"]["retries"] != "5" || r.Parameters["shared"]["site_label"] != "A" {
		t.Errorf("the values are %v", r.Parameters)
	}
	for param, value := range map[string]string{"setup.retries": "12", "setup.colour": "red"} {
		if err := SetParam(o, "bench-a", param, value); err == nil || !strings.Contains(err.Error(), strings.Split(param, ".")[1]) {
			t.Errorf("%s = %s: %v", param, value, err)
		}
	}
	if r, _ := instancerecord.Read(instances, "bench-a"); r.Parameters["setup"]["retries"] != "5" {
		t.Errorf("a refused value changed the record: %v", r.Parameters)
	}
}

// holdClaim runs this test binary as another process holding an instance's claim, until the test ends.
func holdClaim(t *testing.T, root string) (release func()) {
	t.Helper()
	self, _ := os.Executable()
	cmd := exec.Command(self, "-test.run=^$")
	cmd.Env = append(os.Environ(), claimRole+"="+root)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	release = func() { cmd.Process.Kill(); cmd.Wait() }
	t.Cleanup(release)
	buf := make([]byte, 5)
	if _, err := io.ReadFull(out, buf); err != nil || string(buf) != "held\n" {
		t.Fatalf("the claim was not taken: %q %v", buf, err)
	}
	return release
}

// stateOf fills an instance's state as the Core and its units would.
func stateOf(t *testing.T, instances, name string, files map[string]string) {
	t.Helper()
	for path, body := range files {
		p := filepath.Join(instancerecord.StateDir(instances, name), path)
		os.MkdirAll(filepath.Dir(p), 0o700)
		os.WriteFile(p, []byte(body), 0o600)
	}
}

func created(t *testing.T, o Options, names ...string) {
	t.Helper()
	for _, n := range names {
		if _, err := Create(o, "com.yoke.station", n, "", values(t, "[shared]\nsite_label = \"A\"\n")); err != nil {
			t.Fatal(err)
		}
	}
}

func runtimeOf(t *testing.T) (Options, string) {
	t.Helper()
	apps, instances := roots(t)
	installed(t, apps, "2.1.0", installroot.Complete, true, "")
	run, _ := os.MkdirTemp("", "yr")
	t.Cleanup(func() { os.RemoveAll(run) })
	o := opts(apps, instances)
	o.Runtime = run
	return o, run
}

var full = map[string]string{"registry.db": "reg", "logs.db": "logs", "version": "2.1.0\n", "secrets/token": "s3cret", "storage/datasets/a": "data"}

// std: yoke:the-instances.06
func TestACopyIsTheStateWholeAndWhereItCameFrom(t *testing.T) {
	o, run := runtimeOf(t)
	created(t, o, "bench-a")
	stateOf(t, o.Instances, "bench-a", full)
	dest := filepath.Join(t.TempDir(), "copy")
	release := holdClaim(t, filepath.Join(run, "bench-a"))
	if err := Copy(o, "bench-a", dest); err == nil || !strings.Contains(err.Error(), "running") {
		t.Errorf("a running instance was copied: %v", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("a refused copy wrote something")
	}
	release()
	if err := Copy(o, "bench-a", dest); err != nil {
		t.Fatal(err)
	}
	for path, body := range full {
		got, err := os.ReadFile(filepath.Join(dest, "state", path))
		info, _ := os.Stat(filepath.Join(dest, "state", path))
		if err != nil || string(got) != body || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %q %v", path, got, err)
		}
	}
	if origin, _ := os.ReadFile(filepath.Join(dest, "origin")); strings.TrimSpace(string(origin)) != "bench-a" {
		t.Errorf("origin is %q", origin)
	}
	if err := Copy(o, "bench-a", dest); err == nil || !strings.Contains(err.Error(), dest) {
		t.Errorf("a copy onto a path that exists: %v", err)
	}
}

// std: yoke:the-instances.07
func TestARestoreKeepsOrStripsByWhereTheCopyCameFrom(t *testing.T) {
	o, run := runtimeOf(t)
	created(t, o, "bench-a", "bench-b", "bench-c")
	stateOf(t, o.Instances, "bench-a", full)
	stateOf(t, o.Instances, "bench-c", map[string]string{"logs.db": "mine"})
	dest := filepath.Join(t.TempDir(), "copy")
	if err := Copy(o, "bench-a", dest); err != nil {
		t.Fatal(err)
	}
	if err := Restore(o, dest, "bench-a"); err != nil {
		t.Fatal(err)
	}
	for path := range full {
		if _, err := os.Stat(filepath.Join(instancerecord.StateDir(o.Instances, "bench-a"), path)); err != nil {
			t.Errorf("bench-a lost %s", path)
		}
	}
	if err := Restore(o, dest, "bench-b"); err != nil {
		t.Fatal(err)
	}
	b := instancerecord.StateDir(o.Instances, "bench-b")
	for _, gone := range []string{"registry.db", "secrets"} {
		if _, err := os.Stat(filepath.Join(b, gone)); err == nil {
			t.Errorf("bench-b was given %s", gone)
		}
	}
	for _, kept := range []string{"version", "logs.db", "storage/datasets/a"} {
		if _, err := os.Stat(filepath.Join(b, kept)); err != nil {
			t.Errorf("bench-b was not given %s", kept)
		}
	}
	holdClaim(t, filepath.Join(run, "bench-c"))
	if err := Restore(o, dest, "bench-c"); err == nil || !strings.Contains(err.Error(), "running") {
		t.Errorf("a restore into a running instance: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(instancerecord.StateDir(o.Instances, "bench-c"), "logs.db")); string(got) != "mine" {
		t.Errorf("bench-c's state was touched: %q", got)
	}
}

// std: yoke:the-instances.08
func TestReadingAnInstanceReportsWhatItsStateOccupies(t *testing.T) {
	o, _ := runtimeOf(t)
	created(t, o, "bench-a")
	stateOf(t, o.Instances, "bench-a", map[string]string{"registry.db": "abc", "logs.db": "abcde",
		"storage/datasets/x": "abcdefg", "storage/cache/y": "abcdefghijk"})
	got, err := Occupies(o, "bench-a")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"registry": 3, "logs": 5, "storage/datasets": 7, "storage/cache": 11}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("the state occupies %v, want %v", got, want)
	}
}
