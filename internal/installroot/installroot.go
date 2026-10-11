// Package installroot is the shared root bundles are installed under: one directory per application and
// one per version, the bundle and its install record side by side.
//
// A root is checked for its properties and never for having been configured. A version's directory is
// created exclusively, which is what decides between two installs of it. The record says what the
// filesystem cannot — whether an installation completed, which key verified it, what entered — and
// everything else is read from the tree: nothing writes a list of what is installed.
package installroot

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"syscall"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/version"
)

// RecordFile is the install record's name, beside the bundle.
const RecordFile = "install.yaml"

// The modes the tree carries.
const (
	dirMode    = 0o775 | os.ModeSetgid
	recordMode = 0o664
)

var (
	// ErrInProgress is a version whose installation is in progress, or was interrupted.
	ErrInProgress = errors.New("an installation of this version is in progress or was interrupted")
	// ErrPresent is a version already installed: there is nothing to do.
	ErrPresent = errors.New("this version is already installed")
)

// groups are the groups this account belongs to.
var groups = func() ([]int, error) {
	g, err := os.Getgroups()
	return append(g, os.Getgid()), err
}

// State is where an installation stands.
type State string

const (
	Incomplete State = "incomplete"
	Complete   State = "complete"
	Withdrawn  State = "withdrawn"
)

// Record is an install record.
type Record struct {
	State       State
	InstalledBy string
	InstalledAt time.Time
	VerifiedBy  string
	Sealed      bool
	Digest      string
	Shared      bool
}

type recordFile struct {
	State       string `yaml:"state"`
	InstalledBy string `yaml:"installed_by,omitempty"`
	InstalledAt string `yaml:"installed_at,omitempty"`
	VerifiedBy  string `yaml:"verified_by,omitempty"`
	Sealed      bool   `yaml:"sealed"`
	Digest      string `yaml:"digest,omitempty"`
	Shared      bool   `yaml:"shared"`
}

// Pinnable says whether a new instance may pin this version: only a complete one may. A withdrawn version
// refuses new pins, which closes the race between deciding nothing points at it and deleting it.
func (r Record) Pinnable() error {
	if r.State != Complete {
		return fmt.Errorf("this version is %s, and only a complete version may be pinned", r.State)
	}
	return nil
}

// Root is a checked install root.
type Root struct {
	Path string
}

// Check requires path to have the properties a root must have: a directory, of a group this account
// belongs to, with set-group-id, which the group may write and others may not. A root without any of them
// is refused, naming it.
func Check(path string) (Root, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Root{}, fmt.Errorf("the install root %s: %w", path, err)
	}
	refuse := func(property string) (Root, error) {
		return Root{}, fmt.Errorf("the install root %s is refused: %s", path, property)
	}
	if !info.IsDir() {
		return refuse("it is not a directory")
	}
	mine, err := groups()
	if err != nil {
		return Root{}, err
	}
	if g := gid(info); !slices.Contains(mine, int(g)) {
		return refuse(fmt.Sprintf("its group %d is not a group this account belongs to, and membership is the right to install", g))
	}
	mode := info.Mode()
	if mode&os.ModeSetgid == 0 {
		return refuse("the set-group-id bit is not set, so what is written beneath it would not stay in its group")
	}
	if mode.Perm()&0o020 == 0 {
		return refuse("its group may not write")
	}
	if mode.Perm()&0o002 != 0 {
		return refuse("others may write")
	}
	return Root{Path: path}, nil
}

func gid(info fs.FileInfo) uint32 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Gid
	}
	return ^uint32(0)
}

func owner(info fs.FileInfo) string {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	id := strconv.FormatUint(uint64(st.Uid), 10)
	if u, err := user.LookupId(id); err == nil {
		return u.Username
	}
	return id
}

// Installation is a version directory this installation created.
type Installation struct {
	Dir string
}

// Begin creates the application's directory if it is absent, then the version's exclusively, and writes
// its record `incomplete`. A version directory already there is reported as what its record says.
func (r Root) Begin(id, ver string) (*Installation, error) {
	app := filepath.Join(r.Path, id)
	if err := os.Mkdir(app, 0o775); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	} else if err == nil {
		if err := os.Chmod(app, dirMode); err != nil {
			return nil, err
		}
	}
	dir := filepath.Join(app, ver)
	if err := os.Mkdir(dir, 0o775); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		rec, rerr := ReadRecord(dir)
		if rerr == nil && rec.State == Complete {
			return nil, fmt.Errorf("%s %s: %w", id, ver, ErrPresent)
		}
		return nil, fmt.Errorf("%s %s: %w", id, ver, ErrInProgress)
	}
	if err := os.Chmod(dir, dirMode); err != nil {
		return nil, err
	}
	i := &Installation{Dir: dir}
	by := ""
	if u, err := user.Current(); err == nil {
		by = u.Username
	}
	return i, i.Write(Record{State: Incomplete, InstalledBy: by, Shared: true})
}

// Write replaces the installation's record.
func (i *Installation) Write(rec Record) error {
	f := recordFile{State: string(rec.State), InstalledBy: rec.InstalledBy, VerifiedBy: rec.VerifiedBy,
		Sealed: rec.Sealed, Digest: rec.Digest, Shared: rec.Shared}
	if !rec.InstalledAt.IsZero() {
		f.InstalledAt = rec.InstalledAt.UTC().Format(time.RFC3339)
	}
	data, err := yaml.Marshal(f)
	if err != nil {
		return err
	}
	path := filepath.Join(i.Dir, RecordFile)
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, recordMode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, recordMode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadRecord reads the record beside a bundle. An unknown key, and a state that is none of the three, are
// refused.
func ReadRecord(dir string) (Record, error) {
	path := filepath.Join(dir, RecordFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var f recordFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return Record{}, fmt.Errorf("%s: %w", path, err)
	}
	r := Record{State: State(f.State), InstalledBy: f.InstalledBy, VerifiedBy: f.VerifiedBy, Sealed: f.Sealed,
		Digest: f.Digest, Shared: f.Shared}
	if !slices.Contains([]State{Incomplete, Complete, Withdrawn}, r.State) {
		return Record{}, fmt.Errorf("%s: state %q is none of incomplete, complete and withdrawn", path, f.State)
	}
	if f.InstalledAt != "" {
		at, err := time.Parse(time.RFC3339, f.InstalledAt)
		if err != nil {
			return Record{}, fmt.Errorf("%s: installed_at %q is not a time: %w", path, f.InstalledAt, err)
		}
		r.InstalledAt = at.UTC()
	}
	return r, nil
}

// App is one application under the root: its directory's owner, and its versions in order.
type App struct {
	ID       string
	Owner    string
	Versions []Version
}

// Version is one version of an application and where its installation stands.
type Version struct {
	Version string
	State   State
}

// List reads what is installed from the tree: the applications in order of their identifiers, and each
// one's versions ordered as semantic versions. A version whose record cannot be read is incomplete.
func (r Root) List() ([]App, error) {
	entries, err := os.ReadDir(r.Path)
	if err != nil {
		return nil, err
	}
	var apps []App
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		a := App{ID: e.Name(), Owner: owner(info)}
		versions, err := os.ReadDir(filepath.Join(r.Path, e.Name()))
		if err != nil {
			return nil, err
		}
		for _, v := range versions {
			if !v.IsDir() {
				continue
			}
			if _, err := version.Parse(v.Name()); err != nil {
				continue
			}
			state := Incomplete
			if rec, err := ReadRecord(filepath.Join(r.Path, e.Name(), v.Name())); err == nil {
				state = rec.State
			}
			a.Versions = append(a.Versions, Version{Version: v.Name(), State: state})
		}
		sort.Slice(a.Versions, func(i, j int) bool {
			return version.MustParse(a.Versions[i].Version).Compare(version.MustParse(a.Versions[j].Version)) < 0
		})
		apps = append(apps, a)
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].ID < apps[j].ID })
	return apps, nil
}

// Newest is the newest complete version of an application, ordered as semantic versions.
func (r Root) Newest(id string) (string, bool) {
	apps, err := r.List()
	if err != nil {
		return "", false
	}
	for _, a := range apps {
		if a.ID != id {
			continue
		}
		for i := len(a.Versions) - 1; i >= 0; i-- {
			if a.Versions[i].State == Complete {
				return a.Versions[i].Version, true
			}
		}
	}
	return "", false
}
