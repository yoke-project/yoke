// Package instancerecord is the record that declares an instance: one per instance, in the account's own
// state directory beside the state it declares, the set of them a directory listing.
//
// A record is created by making its directory exclusively and writing the record last, so a name is
// unique without a survey and an interrupted creation is visibly no instance. A write replaces the file
// through a temporary one and a rename, and two edits of one record are serialised by an advisory lock
// beside it. A record declares and authorises nothing, and holds nothing about the instance's present.
package instancerecord

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"

	"go.yaml.in/yaml/v3"
	"golang.org/x/sys/unix"
)

// The names in an instance's directory.
const (
	RecordFile = "instance.yaml"
	LockFile   = "record.lock"
	State      = "state"
)

// The values a field may take.
const (
	Running = "running"
	Stopped = "stopped"

	Waived              = "waived"
	UnlaunchedPermitted = "unlaunched-permitted"
	Failed              = "failed"
)

// Record is an instance's record.
type Record struct {
	ID               string
	Version          string
	Name             string
	Autostart        bool
	DesiredState     string
	Owner            string
	CreatedAt        time.Time
	LastStartedAt    time.Time
	DescriptorDigest string
	Parameters       map[string]map[string]string
	// An ephemeral run's fields, and only an ephemeral run's.
	Ephemeral    bool
	Source       string
	Verification string
	Admission    string
	Outcome      string
}

type file struct {
	ID               string                       `yaml:"id"`
	Version          string                       `yaml:"version"`
	Name             string                       `yaml:"name"`
	Autostart        bool                         `yaml:"autostart"`
	DesiredState     string                       `yaml:"desired_state"`
	Owner            string                       `yaml:"owner"`
	CreatedAt        string                       `yaml:"created_at,omitempty"`
	LastStartedAt    string                       `yaml:"last_started_at,omitempty"`
	DescriptorDigest string                       `yaml:"descriptor_digest,omitempty"`
	Parameters       map[string]map[string]string `yaml:"parameters,omitempty"`
	Ephemeral        bool                         `yaml:"ephemeral,omitempty"`
	Source           string                       `yaml:"source,omitempty"`
	Verification     string                       `yaml:"verification,omitempty"`
	Admission        string                       `yaml:"admission,omitempty"`
	Outcome          string                       `yaml:"outcome,omitempty"`
}

// Root is where the account's instances are: `$XDG_STATE_HOME/yoke/instances`, the state home being
// `$HOME/.local/state` where the variable is unset.
func Root(getenv func(string) string) string {
	home := getenv("XDG_STATE_HOME")
	if home == "" {
		home = filepath.Join(getenv("HOME"), ".local", "state")
	}
	return filepath.Join(home, "yoke", "instances")
}

// File is where an instance's record is.
func File(root, name string) string { return filepath.Join(root, name, RecordFile) }

// StateDir is where an instance's state is, beside its record.
func StateDir(root, name string) string { return filepath.Join(root, name, State) }

// check refuses a record that is not one: a field outside its set, an ephemeral run's field on a declared
// record, or the verification waived with no source.
func (r Record) check() error {
	if r.Name == "" || r.ID == "" || r.Version == "" {
		return errors.New("a record names an instance, an application and a version")
	}
	if r.DesiredState != Running && r.DesiredState != Stopped {
		return fmt.Errorf("desired_state %q is neither running nor stopped", r.DesiredState)
	}
	if !r.Ephemeral {
		for field, v := range map[string]string{"source": r.Source, "verification": r.Verification, "admission": r.Admission, "outcome": r.Outcome} {
			if v != "" {
				return fmt.Errorf("%s belongs to an ephemeral run's record, and this one is declared", field)
			}
		}
		return nil
	}
	if r.Autostart {
		return errors.New("an ephemeral run's record is not eligible for autostart")
	}
	if r.Verification != "" && (r.Verification != Waived || r.Source == "") {
		return fmt.Errorf("verification %q is the development waiver, and belongs only to a run from a source", r.Verification)
	}
	if r.Admission != "" && r.Admission != UnlaunchedPermitted {
		return fmt.Errorf("admission %q is not the waiver %s", r.Admission, UnlaunchedPermitted)
	}
	if r.Outcome != "" && r.Outcome != Failed {
		return fmt.Errorf("outcome %q is not %s", r.Outcome, Failed)
	}
	return nil
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func unstamp(field, s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s %q is not a time: %w", field, s, err)
	}
	return t.UTC(), nil
}

// Read reads an instance's record. An unknown key is refused, and so is a record whose name is not the
// directory's holding it — which is what a record moved or copied by hand looks like.
func Read(root, name string) (Record, error) {
	path := File(root, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return Record{}, fmt.Errorf("%s: %w", path, err)
	}
	if f.Name != name {
		return Record{}, fmt.Errorf("%s: the record names %s, and is in the directory of %s", path, f.Name, name)
	}
	r := Record{ID: f.ID, Version: f.Version, Name: f.Name, Autostart: f.Autostart, DesiredState: f.DesiredState, Owner: f.Owner,
		DescriptorDigest: f.DescriptorDigest, Parameters: f.Parameters, Ephemeral: f.Ephemeral, Source: f.Source,
		Verification: f.Verification, Admission: f.Admission, Outcome: f.Outcome}
	if r.Parameters == nil {
		r.Parameters = map[string]map[string]string{}
	}
	if r.CreatedAt, err = unstamp("created_at", f.CreatedAt); err != nil {
		return Record{}, fmt.Errorf("%s: %w", path, err)
	}
	if r.LastStartedAt, err = unstamp("last_started_at", f.LastStartedAt); err != nil {
		return Record{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := r.check(); err != nil {
		return Record{}, fmt.Errorf("%s: %w", path, err)
	}
	return r, nil
}

// write replaces a record: a temporary file in the same directory, then a rename.
func write(root string, r Record) error {
	if err := r.check(); err != nil {
		return err
	}
	data, err := yaml.Marshal(file{ID: r.ID, Version: r.Version, Name: r.Name, Autostart: r.Autostart, DesiredState: r.DesiredState,
		Owner: r.Owner, CreatedAt: stamp(r.CreatedAt), LastStartedAt: stamp(r.LastStartedAt), DescriptorDigest: r.DescriptorDigest,
		Parameters: r.Parameters, Ephemeral: r.Ephemeral, Source: r.Source, Verification: r.Verification, Admission: r.Admission,
		Outcome: r.Outcome})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Join(root, r.Name), "."+RecordFile+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), File(root, r.Name))
}

// Create creates an instance: its directory exclusively, which is what makes its name unique, and its
// record last.
func Create(root string, r Record) error {
	if err := r.check(); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(root, r.Name), 0o700); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("an instance named %s already exists", r.Name)
		}
		return err
	}
	return write(root, r)
}

// Update edits an instance's record under its advisory lock: read, edited, then replaced.
func Update(root, name string, edit func(*Record) error) error {
	lock, err := os.OpenFile(filepath.Join(root, name, LockFile), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	r, err := Read(root, name)
	if err != nil {
		return err
	}
	if err := edit(&r); err != nil {
		return err
	}
	if r.Name != name {
		return fmt.Errorf("an edit may not rename the instance %s", name)
	}
	return write(root, r)
}

// Listed is one directory under the root: its record, or why it holds no valid one.
type Listed struct {
	Name   string
	Record *Record
	Err    error
}

// List reads the root: every directory in it, by name, with its record or why it holds none.
func List(root string) ([]Listed, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Listed
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		l := Listed{Name: e.Name()}
		if r, err := Read(root, e.Name()); err != nil {
			l.Err = err
		} else {
			l.Record = &r
		}
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// PointingAt names the instances whose records are pinned to the version of the application given.
func PointingAt(root, id, version string) ([]string, error) {
	listed, err := List(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range listed {
		if l.Record != nil && l.Record.ID == id && l.Record.Version == version {
			names = append(names, l.Name)
		}
	}
	return slices.Clip(names), nil
}
