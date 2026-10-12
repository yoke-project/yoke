// Package instances is what the manager does to instances: it creates them from installed versions,
// re-points them at another version, and replaces a value a site supplied. The manager is the party that
// writes records; the guardian reads them.
package instances

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/version"
)

// Options are where the manager finds what is installed and what is declared, and who it acts as.
type Options struct {
	InstallRoot string
	Instances   string
	Now         func() time.Time
	// User is the account acting, by name; empty is the account running this.
	User string
	// Runtime is where the account's instances keep their sockets and claims: `$XDG_RUNTIME_DIR/yoke`.
	Runtime string
}

func (o Options) me() string {
	if o.User != "" {
		return o.User
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// pinned is a version an instance may be pinned to: its bundle's directory and its descriptor, gated.
type pinned struct {
	version string
	bundle  string
	digest  string
	d       *gate.Deployment
}

// pinnable resolves the version an instance would be pinned to — the one named, or the newest complete
// release where none is — and refuses one that is absent, not complete, or not offered to this account.
func (o Options) pinnable(id, ver string) (pinned, error) {
	apps, err := installroot.Root{Path: o.InstallRoot}.List()
	if err != nil {
		return pinned{}, err
	}
	var app *installroot.App
	for i := range apps {
		if apps[i].ID == id {
			app = &apps[i]
		}
	}
	if app == nil {
		return pinned{}, fmt.Errorf("%s is not installed under %s", id, o.InstallRoot)
	}
	if ver == "" {
		for _, v := range app.Versions {
			if v.State == installroot.Complete && !version.MustParse(v.Version).IsPrerelease() {
				ver = v.Version
			}
		}
		if ver == "" {
			return pinned{}, fmt.Errorf("%s has no complete release installed", id)
		}
	}
	dir := filepath.Join(o.InstallRoot, id, ver)
	rec, err := installroot.ReadRecord(dir)
	if errors.Is(err, os.ErrNotExist) {
		return pinned{}, fmt.Errorf("%s %s is not installed", id, ver)
	}
	if err != nil {
		return pinned{}, err
	}
	if err := rec.Pinnable(); err != nil {
		return pinned{}, fmt.Errorf("%s %s: %w", id, ver, err)
	}
	if !rec.Shared && app.Owner != o.me() {
		return pinned{}, fmt.Errorf("%s %s is not shared with this account: %s installed it for itself", id, ver, app.Owner)
	}
	bundle := filepath.Join(dir, "bundle")
	descriptor := filepath.Join(bundle, "yoke.yaml")
	data, err := os.ReadFile(descriptor)
	if err != nil {
		return pinned{}, err
	}
	report, d := gate.Check(gate.Input{Document: gate.Read(descriptor, gate.Descriptor), Moment: gate.Creating,
		Manifests: filepath.Join(bundle, "manifests")})
	if report.Refused() {
		return pinned{}, refused("the descriptor of "+id+" "+ver, report.Findings)
	}
	sum := sha256.Sum256(data)
	return pinned{version: ver, bundle: bundle, digest: "sha256:" + hex.EncodeToString(sum[:]), d: d}, nil
}

func refused(what string, findings []gate.Finding) error {
	var lines []string
	for _, f := range findings {
		if f.Class == gate.Refusal {
			lines = append(lines, fmt.Sprintf("%s at %s: %s", f.Code, f.Location, f.Message))
		}
	}
	return fmt.Errorf("%s is refused: %s", what, strings.Join(lines, "; "))
}

// readValues reads a values file: TOML, one table per group, each value a scalar.
func readValues(path string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	if path == "" {
		return out, nil
	}
	var raw map[string]map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("the values file %s: %w", path, err)
	}
	for group, entries := range raw {
		out[group] = map[string]string{}
		for name, v := range entries {
			switch v.(type) {
			case string, int64, bool:
				out[group][name] = fmt.Sprint(v)
			default:
				return nil, fmt.Errorf("the values file %s: %s.%s is not a scalar", path, group, name)
			}
		}
	}
	return out, nil
}

// Create creates an instance of an installed application and returns its name: the one given, or one
// generated and unique within the account. The values are checked against the declarations before
// anything is written, and the descriptor's digest is recorded with the pin.
func Create(o Options, id, name, ver, valuesFile string) (string, error) {
	p, err := o.pinnable(id, ver)
	if err != nil {
		return "", err
	}
	supplied, err := readValues(valuesFile)
	if err != nil {
		return "", err
	}
	resolved, findings := gate.Values(p.d, supplied)
	if len(findings) > 0 {
		return "", refused("the values supplied", findings)
	}
	r := instancerecord.Record{ID: id, Version: p.version, DesiredState: instancerecord.Stopped, Owner: o.me(),
		CreatedAt: o.now().UTC(), DescriptorDigest: p.digest, Parameters: resolved}
	if name != "" {
		if !nameable(name) {
			return "", fmt.Errorf("the name %q cannot name an instance: it is a path component, with no separator", name)
		}
		r.Name = name
		return name, instancerecord.Create(o.Instances, r)
	}
	base := id[strings.LastIndex(id, ".")+1:]
	for n := 1; ; n++ {
		r.Name = fmt.Sprintf("%s-%d", base, n)
		if _, err := os.Stat(filepath.Join(o.Instances, r.Name)); err == nil {
			continue
		}
		if err := instancerecord.Create(o.Instances, r); err != nil && strings.Contains(err.Error(), "already exists") {
			continue
		} else if err != nil {
			return "", err
		}
		return r.Name, nil
	}
}

func nameable(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\x00")
}

// SetVersion re-points an instance at another installed version, writing the pin and the descriptor's
// digest together. The version that last wrote the instance's state must fall in the data range the
// new version declares; an omitted upper end means that version itself.
func SetVersion(o Options, name, ver string) error {
	current, err := instancerecord.Read(o.Instances, name)
	if err != nil {
		return err
	}
	p, err := o.pinnable(current.ID, ver)
	if err != nil {
		return err
	}
	if stamp, err := os.ReadFile(filepath.Join(instancerecord.StateDir(o.Instances, name), "version")); err == nil {
		if err := gate.Readable(p.d.Head.Data, strings.TrimSpace(string(stamp)), p.version); err != nil {
			return err
		}
	}
	return instancerecord.Update(o.Instances, name, func(r *instancerecord.Record) error {
		r.Version, r.DescriptorDigest = p.version, p.digest
		return nil
	})
}

// SetParam replaces one value an instance records, checked against its version's declarations as at
// creation.
func SetParam(o Options, name, param, value string) error {
	group, key, ok := strings.Cut(param, ".")
	if !ok || group == "" || key == "" {
		return fmt.Errorf("%q names no parameter: it is written <group>.<name>", param)
	}
	current, err := instancerecord.Read(o.Instances, name)
	if err != nil {
		return err
	}
	p, err := o.pinnable(current.ID, current.Version)
	if err != nil {
		return err
	}
	return instancerecord.Update(o.Instances, name, func(r *instancerecord.Record) error {
		supplied := map[string]map[string]string{}
		for g, entries := range r.Parameters {
			supplied[g] = map[string]string{}
			for k, v := range entries {
				supplied[g][k] = v
			}
		}
		if supplied[group] == nil {
			supplied[group] = map[string]string{}
		}
		supplied[group][key] = value
		resolved, findings := gate.Values(p.d, supplied)
		if len(findings) > 0 {
			return refused("the value of "+param, findings)
		}
		r.Parameters = resolved
		return nil
	})
}

// running says whether an instance's claim is held, asking without taking it.
func (o Options) running(name string) (bool, error) {
	pid, err := instance.Holder(filepath.Join(o.Runtime, name, instance.LockName))
	return pid != 0, err
}

func (o Options) stopped(name string, verbs ...string) error {
	if _, err := instancerecord.Read(o.Instances, name); err != nil {
		return err
	}
	up, err := o.running(name)
	if err != nil {
		return err
	}
	if up {
		what := "copied or restored"
		if len(verbs) > 0 {
			what = verbs[0]
		}
		return fmt.Errorf("the instance %s is running, and it is %s only while it is stopped", name, what)
	}
	return nil
}

// Copy writes a stopped instance's state, whole, to a path that does not exist: `state/`, and `origin`
// naming the instance.
func Copy(o Options, name, path string) error {
	if err := o.stopped(name); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%s exists, and a copy is written only where nothing is", path)
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	if err := tree(instancerecord.StateDir(o.Instances, name), filepath.Join(path, instancerecord.State), nil); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(path, "origin"), []byte(name+"\n"), 0o600)
}

// Restore replaces a stopped instance's state with a copy's: whole where the copy is the same
// instance's, and without its Registry and its secrets where it is another's or says nothing of where it
// came from. The version stamp travels either way.
func Restore(o Options, path, name string) error {
	if err := o.stopped(name); err != nil {
		return err
	}
	from := filepath.Join(path, instancerecord.State)
	if info, err := os.Stat(from); err != nil || !info.IsDir() {
		return fmt.Errorf("%s holds no copied state", path)
	}
	var skip map[string]bool
	if origin, err := os.ReadFile(filepath.Join(path, "origin")); err != nil || strings.TrimSpace(string(origin)) != name {
		skip = map[string]bool{"registry.db": true, "registry.db-wal": true, "registry.db-shm": true, "secrets": true}
	}
	state := instancerecord.StateDir(o.Instances, name)
	staged := state + ".restoring"
	os.RemoveAll(staged)
	if err := tree(from, staged, skip); err != nil {
		os.RemoveAll(staged)
		return err
	}
	former := state + ".former"
	os.RemoveAll(former)
	if err := os.Rename(state, former); err != nil && !errors.Is(err, os.ErrNotExist) {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Rename(staged, state); err != nil {
		os.Rename(former, state)
		return err
	}
	return os.RemoveAll(former)
}

// tree copies a directory, its modes kept, leaving out the top-level entries named in skip.
func tree(from, to string, skip map[string]bool) error {
	return filepath.WalkDir(from, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			if p == from && errors.Is(err, os.ErrNotExist) {
				return os.MkdirAll(to, 0o700)
			}
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if top := strings.Split(rel, string(filepath.Separator))[0]; skip[top] {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(to, rel)
		info, err := e.Info()
		if err != nil {
			return err
		}
		switch {
		case e.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		return nil // a socket or a pipe belongs to a running instance and is not state
	})
}

// Occupies is what an instance's state occupies, read from the filesystem: each store, with the files
// beside it, and each storage need's directory.
func Occupies(o Options, name string) (map[string]int64, error) {
	if _, err := instancerecord.Read(o.Instances, name); err != nil {
		return nil, err
	}
	state := instancerecord.StateDir(o.Instances, name)
	out := map[string]int64{}
	for store, file := range map[string]string{"registry": "registry.db", "logs": "logs.db"} {
		matches, _ := filepath.Glob(filepath.Join(state, file+"*"))
		for _, m := range matches {
			if info, err := os.Stat(m); err == nil && info.Mode().IsRegular() {
				out[store] += info.Size()
			}
		}
	}
	needs, _ := os.ReadDir(filepath.Join(state, "storage"))
	for _, n := range needs {
		if !n.IsDir() {
			continue
		}
		var size int64
		filepath.WalkDir(filepath.Join(state, "storage", n.Name()), func(_ string, e os.DirEntry, err error) error {
			if err == nil && !e.IsDir() {
				if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
					size += info.Size()
				}
			}
			return nil
		})
		out["storage/"+n.Name()] = size
	}
	return out, nil
}

// SetDesired writes the desired state, the one field starting and stopping change.
func SetDesired(o Options, name, desired string) error {
	return instancerecord.Update(o.Instances, name, func(r *instancerecord.Record) error {
		r.DesiredState = desired
		if desired == instancerecord.Running {
			r.LastStartedAt = o.now()
		}
		return nil
	})
}

// SetAutostart writes autostart, a second field and not the desired state.
func SetAutostart(o Options, name string, autostart bool) error {
	return instancerecord.Update(o.Instances, name, func(r *instancerecord.Record) error {
		r.Autostart = autostart
		return nil
	})
}

// Running says whether an instance's claim is held.
func Running(o Options, name string) (bool, error) { return o.running(name) }

// Clear removes a stopped instance's state and leaves it declared.
func Clear(o Options, name string) error {
	if err := o.stopped(name, "cleared"); err != nil {
		return err
	}
	return os.RemoveAll(instancerecord.StateDir(o.Instances, name))
}

// Remove removes a stopped instance's record and its state together: one directory.
func Remove(o Options, name string) error {
	if err := o.stopped(name, "removed"); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(o.Instances, name))
}

// Secrets are the secrets an instance's version declares, each with the units that need it: from the
// units' own needs and, for a Plugin unit, its Manifest's. Which exist is the directory's to say.
func Secrets(o Options, name string) (map[string][]string, error) {
	r, err := instancerecord.Read(o.Instances, name)
	if err != nil {
		return nil, err
	}
	bundle := filepath.Join(o.InstallRoot, r.ID, r.Version, "bundle")
	if r.Source != "" {
		bundle = r.Source
	}
	report, d := gate.Check(gate.Input{Document: gate.Read(filepath.Join(bundle, "yoke.yaml"), gate.Descriptor), Moment: gate.Creating,
		Manifests: filepath.Join(bundle, "manifests")})
	if d == nil {
		return nil, refused("the descriptor of "+r.ID+" "+r.Version, report.Findings)
	}
	out := map[string][]string{}
	for _, unit := range d.Order {
		u := d.Units[unit]
		needs := u.Needs
		if u.Kind == "plugin" {
			if _, m := gate.CheckManifest(gate.Read(gate.BundleManifestPath(filepath.Join(bundle, "manifests"), u.Plugin), gate.Composition)); m != nil {
				needs = m.Needs
			}
		}
		for _, n := range needs {
			if n.Class == "secret" {
				out[n.Name] = append(out[n.Name], unit)
			}
		}
	}
	return out, nil
}

// SecretPath is where an instance's secret is: `<instance state>/secrets/<secret>`.
func SecretPath(o Options, name, secret string) string {
	return filepath.Join(instancerecord.StateDir(o.Instances, name), "secrets", secret)
}

// SetSecret writes a secret into the instance's state, mode 0600 in a directory of mode 0700, replacing
// any value it had. It is never in the record.
func SetSecret(o Options, name, secret, value string) error {
	if _, err := instancerecord.Read(o.Instances, name); err != nil {
		return err
	}
	if err := instance.ValidateName(secret); err != nil {
		return fmt.Errorf("the secret's name: %w", err)
	}
	path := SecretPath(o, name, secret)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+secret+".")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
