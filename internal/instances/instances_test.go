package instances

import (
	"crypto/sha256"
	"fmt"
	"os"
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
	os.MkdirAll(bundle, 0o775)
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
		{"com.yoke.station", "2.3.0", values(t, ""), "parameter.missing", false},
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
