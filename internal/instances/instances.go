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
		if err := readable(p.d.Head.Data, strings.TrimSpace(string(stamp)), p.version); err != nil {
			return err
		}
	}
	return instancerecord.Update(o.Instances, name, func(r *instancerecord.Record) error {
		r.Version, r.DescriptorDigest = p.version, p.digest
		return nil
	})
}

// readable says whether a version declaring the data range given may read state the version stamped
// last wrote.
func readable(data, stamp, ver string) error {
	if data == "" || stamp == "" {
		return nil
	}
	r, err := version.ParseRange(data)
	if err != nil {
		return err
	}
	wrote, err := version.Parse(stamp)
	if err != nil {
		return fmt.Errorf("the state's stamp %q is not a version", stamp)
	}
	within := r.Contains(wrote)
	if !strings.Contains(data, "<") {
		within = within && wrote.Compare(version.MustParse(ver)) <= 0
	}
	if !within {
		return fmt.Errorf("%s declares it reads state written by %s, and %s last wrote this instance's state", ver, data, stamp)
	}
	return nil
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
