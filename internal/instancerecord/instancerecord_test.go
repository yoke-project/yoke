package instancerecord

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func declared(name, ver string) Record {
	return Record{ID: "com.yoke.station", Version: ver, Name: name, Autostart: true, DesiredState: Running, Owner: "davide",
		CreatedAt: time.Date(2026, 8, 15, 9, 20, 44, 0, time.UTC), LastStartedAt: time.Date(2026, 8, 15, 9, 21, 2, 0, time.UTC),
		DescriptorDigest: "sha256:7d10", Parameters: map[string]map[string]string{"shared": {"site_label": "Bench A"}, "acquire": {"instrument": "/dev/spectro-head-a"}}}
}

// std: yoke:the-instance-record.01
func TestARecordSitsUnderTheStateDirectory(t *testing.T) {
	env := func(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }
	if got := File(Root(env(map[string]string{"XDG_STATE_HOME": "/s"})), "bench-a"); got != "/s/yoke/instances/bench-a/instance.yaml" {
		t.Errorf("under XDG_STATE_HOME: %s", got)
	}
	root := Root(env(map[string]string{"HOME": "/home/d"}))
	if got := File(root, "bench-a"); got != "/home/d/.local/state/yoke/instances/bench-a/instance.yaml" {
		t.Errorf("under HOME: %s", got)
	}
	if got := StateDir(root, "bench-a"); got != "/home/d/.local/state/yoke/instances/bench-a/state" {
		t.Errorf("the state is at %s", got)
	}
}

// std: yoke:the-instance-record.02
func TestARecordHoldsWhatItDeclaresReadStrictly(t *testing.T) {
	root := t.TempDir()
	want := declared("bench-a", "2.1.0")
	if err := Create(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, "bench-a")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	path := File(root, "bench-a")
	body, _ := os.ReadFile(path)
	for edit, names := range map[string]string{
		string(body) + "colour: red\n": "colour",
		strings.Replace(string(body), "desired_state: running", "desired_state: paused", 1): "paused",
		strings.Replace(string(body), "name: bench-a", "name: bench-z", 1):                  "bench-z",
	} {
		os.WriteFile(path, []byte(edit), 0o644)
		if _, err := Read(root, "bench-a"); err == nil || !strings.Contains(err.Error(), names) {
			t.Errorf("an edited record: %v, want a refusal naming %q", err, names)
		}
	}
}

// std: yoke:the-instance-record.03
func TestANameIsUniqueBecauseCreatingItMakesADirectory(t *testing.T) {
	root := t.TempDir()
	if err := Create(root, declared("bench-a", "2.1.0")); err != nil {
		t.Fatal(err)
	}
	if err := Create(root, declared("bench-a", "2.0.0")); err == nil || !strings.Contains(err.Error(), "bench-a") {
		t.Errorf("a second creation: %v", err)
	}
	if r, _ := Read(root, "bench-a"); r.Version != "2.1.0" {
		t.Errorf("the record was changed to %s", r.Version)
	}
	os.MkdirAll(filepath.Join(root, "bench-b"), 0o700)
	listed, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range listed {
		if l.Name == "bench-b" && (l.Record != nil || l.Err == nil) {
			t.Errorf("bench-b is listed as %+v", l)
		}
	}
}

// std: yoke:the-instance-record.04
func TestTwoEditsAreSerialisedAndAWriteReplacesTheFile(t *testing.T) {
	root := t.TempDir()
	r := declared("bench-a", "2.1.0")
	r.Parameters = map[string]map[string]string{}
	Create(root, r)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := Update(root, "bench-a", func(r *Record) error {
				if r.Parameters["g"] == nil {
					r.Parameters["g"] = map[string]string{}
				}
				r.Parameters["g"][fmt.Sprint("k", i)] = fmt.Sprint(i)
				return nil
			})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, err := Read(root, "bench-a")
	if err != nil || len(got.Parameters["g"]) != 20 {
		t.Errorf("%d values of 20 survived: %v", len(got.Parameters["g"]), err)
	}
	entries, _ := os.ReadDir(filepath.Join(root, "bench-a"))
	for _, e := range entries {
		if e.Name() != "instance.yaml" && e.Name() != "record.lock" {
			t.Errorf("%s is left in the instance's directory", e.Name())
		}
	}
}

// std: yoke:the-instance-record.05
func TestAnEphemeralRecordCarriesItsFourFields(t *testing.T) {
	root := t.TempDir()
	e := Record{ID: "com.yoke.station", Version: "2.1.0", Name: "run-1", DesiredState: Running, Owner: "davide",
		CreatedAt: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC), Ephemeral: true, Source: "/home/davide/work/station",
		Verification: Waived, Admission: UnlaunchedPermitted, Outcome: Failed}
	if err := Create(root, e); err != nil {
		t.Fatal(err)
	}
	if got, err := Read(root, "run-1"); err != nil || fmt.Sprint(got) != fmt.Sprint(e) {
		t.Errorf("read back %+v %v", got, err)
	}
	notEphemeral := declared("bench-a", "2.1.0")
	notEphemeral.Verification = Waived
	noSource := e
	noSource.Name, noSource.Source = "run-2", ""
	for _, r := range []Record{notEphemeral, noSource} {
		if err := Create(root, r); err == nil || !strings.Contains(err.Error(), "verification") {
			t.Errorf("%s: %v", r.Name, err)
		}
	}
}

// std: yoke:the-instance-record.06
func TestTheSetIsADirectoryListing(t *testing.T) {
	root := t.TempDir()
	for name, ver := range map[string]string{"bench-a": "2.1.0", "bench-b": "2.1.0", "bench-c": "2.0.0"} {
		Create(root, declared(name, ver))
	}
	listed, _ := List(root)
	var names []string
	for _, l := range listed {
		names = append(names, l.Name)
	}
	if !slices.Equal(names, []string{"bench-a", "bench-b", "bench-c"}) {
		t.Errorf("listed %v", names)
	}
	pointing, err := PointingAt(root, "com.yoke.station", "2.1.0")
	if err != nil || !slices.Equal(pointing, []string{"bench-a", "bench-b"}) {
		t.Errorf("pointing at 2.1.0: %v %v", pointing, err)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("%s lists something in the root", e.Name())
		}
	}
}
