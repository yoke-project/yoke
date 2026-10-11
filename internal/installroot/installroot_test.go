package installroot

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// root makes a directory with the properties a root must have: this account's group, set-group-id and
// mode 2775.
func root(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "apps")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o2775|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	return dir
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// std: yoke:the-install-root.01
func TestARootIsCheckedForItsProperties(t *testing.T) {
	good := root(t)
	if _, err := Check(good); err != nil {
		t.Fatalf("a root with every property: %v", err)
	}
	file := filepath.Join(t.TempDir(), "apps")
	os.WriteFile(file, nil, 0o644)
	noSetgid := root(t)
	chmod(t, noSetgid, 0o775)
	noGroupWrite := root(t)
	chmod(t, noGroupWrite, 0o755|os.ModeSetgid)
	othersWrite := root(t)
	chmod(t, othersWrite, 0o777|os.ModeSetgid)
	for path, names := range map[string]string{file: "directory", noSetgid: "set-group-id", noGroupWrite: "group may not write", othersWrite: "others may write"} {
		if _, err := Check(path); err == nil || !strings.Contains(err.Error(), names) {
			t.Errorf("%s: %v, want a refusal naming %q", path, err, names)
		}
	}
	member := groups
	groups = func() ([]int, error) { return []int{-1}, nil }
	defer func() { groups = member }()
	if _, err := Check(good); err == nil || !strings.Contains(err.Error(), "group") {
		t.Errorf("a group this account is not in: %v", err)
	}
}

// std: yoke:the-install-root.02
func TestAVersionDirectoryIsCreatedExclusively(t *testing.T) {
	r, err := Check(root(t))
	if err != nil {
		t.Fatal(err)
	}
	first, err := r.Begin("com.yoke.station", "2.1.0")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := ReadRecord(first.Dir)
	if err != nil || rec.State != Incomplete {
		t.Fatalf("the record written first: %+v %v", rec, err)
	}
	if _, err := r.Begin("com.yoke.station", "2.1.0"); !errors.Is(err, ErrInProgress) {
		t.Errorf("a second install while the first is incomplete: %v", err)
	}
	rec.State = Complete
	if err := first.Write(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Begin("com.yoke.station", "2.1.0"); !errors.Is(err, ErrPresent) {
		t.Errorf("a third install once it is complete: %v", err)
	}
	if _, err := r.Begin("com.yoke.station", "2.2.0"); err != nil {
		t.Errorf("another version beside it: %v", err)
	}
}

// std: yoke:the-install-root.03
func TestTheRecordSaysWhatTheFilesystemCannot(t *testing.T) {
	r, _ := Check(root(t))
	i, err := r.Begin("com.yoke.station", "2.1.0")
	if err != nil {
		t.Fatal(err)
	}
	me, _ := user.Current()
	at := time.Date(2026, 10, 11, 9, 12, 4, 0, time.FixedZone("CEST", 2*3600))
	want := Record{State: Complete, InstalledBy: me.Username, InstalledAt: at, VerifiedBy: "9f3c1e", Sealed: true, Digest: "sha256:1a2b", Shared: true}
	if err := i.Write(want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord(i.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.InstalledAt.Equal(at) || got.InstalledAt.Location() != time.UTC {
		t.Errorf("installed_at read back as %v", got.InstalledAt)
	}
	got.InstalledAt, want.InstalledAt = time.Time{}, time.Time{}
	if got != want {
		t.Errorf("read back %+v, want %+v", got, want)
	}
	begun, _ := r.Begin("com.yoke.other", "1.0.0")
	if rec, _ := ReadRecord(begun.Dir); rec.InstalledBy != me.Username {
		t.Errorf("installed_by is %q, want %q", rec.InstalledBy, me.Username)
	}
	for body, names := range map[string]string{
		"state: complete\ncolour: red\n": "colour",
		"state: pending\n":               "pending",
	} {
		os.WriteFile(filepath.Join(i.Dir, "install.yaml"), []byte(body), 0o664)
		if _, err := ReadRecord(i.Dir); err == nil || !strings.Contains(err.Error(), names) {
			t.Errorf("%q: %v", body, err)
		}
	}
}

// std: yoke:the-install-root.04
func TestTheTreeCarriesItsModes(t *testing.T) {
	path := root(t)
	r, _ := Check(path)
	rootInfo, _ := os.Stat(path)
	i, err := r.Begin("com.yoke.station", "2.1.0")
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Dir(i.Dir), i.Dir} {
		info, _ := os.Stat(dir)
		if info.Mode()&(os.ModePerm|os.ModeSetgid) != 0o775|os.ModeSetgid {
			t.Errorf("%s is %v, want 2775", dir, info.Mode())
		}
		if gid(info) != gid(rootInfo) {
			t.Errorf("%s is not in the root's group", dir)
		}
	}
	info, _ := os.Stat(filepath.Join(i.Dir, "install.yaml"))
	if info.Mode().Perm() != 0o664 {
		t.Errorf("install.yaml is %v, want 0664", info.Mode())
	}
}

// std: yoke:the-install-root.05
func TestWhatIsInstalledIsReadFromTheTree(t *testing.T) {
	path := root(t)
	r, _ := Check(path)
	for _, v := range []struct{ id, version string }{
		{"com.yoke.station", "2.1.0"}, {"com.yoke.station", "2.10.0"}, {"com.yoke.station", "2.2.0-rc.1"}, {"com.yoke.other", "1.0.0"},
	} {
		i, err := r.Begin(v.id, v.version)
		if err != nil {
			t.Fatal(err)
		}
		if v.version != "2.2.0-rc.1" {
			i.Write(Record{State: Complete, Shared: true})
		}
	}
	apps, err := r.List()
	if err != nil {
		t.Fatal(err)
	}
	me, _ := user.Current()
	var ids []string
	for _, a := range apps {
		ids = append(ids, a.ID)
		if a.Owner != me.Username {
			t.Errorf("%s is owned by %q", a.ID, a.Owner)
		}
		if a.ID == "com.yoke.station" {
			var versions []string
			for _, v := range a.Versions {
				versions = append(versions, v.Version+":"+string(v.State))
			}
			if want := []string{"2.1.0:complete", "2.2.0-rc.1:incomplete", "2.10.0:complete"}; !slices.Equal(versions, want) {
				t.Errorf("versions %v, want %v", versions, want)
			}
		}
	}
	if !slices.Equal(ids, []string{"com.yoke.other", "com.yoke.station"}) {
		t.Errorf("applications %v", ids)
	}
	if v, ok := r.Newest("com.yoke.station"); !ok || v != "2.10.0" {
		t.Errorf("the newest is %q", v)
	}
	entries, _ := os.ReadDir(path)
	for _, e := range entries {
		if !e.IsDir() {
			t.Errorf("%s lists something in the root", e.Name())
		}
	}
}

// std: yoke:the-install-root.06
func TestAWithdrawnVersionRefusesNewPins(t *testing.T) {
	if err := (Record{State: Complete}).Pinnable(); err != nil {
		t.Errorf("complete: %v", err)
	}
	for _, s := range []State{Withdrawn, Incomplete} {
		if err := (Record{State: s}).Pinnable(); err == nil || !strings.Contains(err.Error(), string(s)) {
			t.Errorf("%s: %v", s, err)
		}
	}
}
