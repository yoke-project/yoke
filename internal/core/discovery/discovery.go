// Package discovery is the one way a declaration enters a service-form deployment. A scan of the
// Plugin directory makes a Plugin exist — declared in the Registry, authorisable, and not running — and
// the composition document in force says what runs.
//
// There is no API that registers a Plugin and nothing registers itself: the set of known Plugins is a
// property of static, inspectable files. A Manifest that fails its check is logged and skipped and the
// others are still declared. A later scan reconciles in both directions and removes nothing: whether
// anything still declares a plugin is derived from the directory, never written.
package discovery

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
	"github.com/yoke-project/yoke/internal/gate"
)

// Discovery scans one Plugin directory into one Registry.
type Discovery struct {
	dir      string
	registry *registry.Registry
	log      *slog.Logger

	mu        sync.Mutex
	manifests map[string]*gate.Manifest // what the last scan read, by plugin
	publish   func(event.Event)
	resolved  map[string]string // the digest each Manifest last resolved at, by path
	bundled   bool              // the directory is a bundle's `manifests/`, holding `<plugin>.yaml`
}

// Publishing has discovery tell publish what it concludes of each document it reads: that it resolved,
// when its digest is not the one it last resolved at, or each finding it was refused with.
func (d *Discovery) Publishing(publish func(event.Event)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.publish = publish
}

func (d *Discovery) tell(e event.Event) {
	d.mu.Lock()
	publish := d.publish
	d.mu.Unlock()
	if publish != nil {
		publish(e)
	}
}

// New is discovery over the Plugin directory dir.
func New(dir string, r *registry.Registry, log *slog.Logger) *Discovery {
	return &Discovery{dir: dir, registry: r, log: log, manifests: map[string]*gate.Manifest{}, resolved: map[string]string{}}
}

// Bundle is discovery over a bundle's `manifests/` directory, where each Manifest is `<plugin>.yaml`. It
// is scanned once: the bundle is the deployment, and is not watched.
func Bundle(dir string, r *registry.Registry, log *slog.Logger) *Discovery {
	d := New(dir, r, log)
	d.bundled = true
	return d
}

// Scan reads every `<plugin>/manifest.yaml` under the directory, or every `<plugin>.yaml` in a bundle's. Each that passes its check is declared
// in the Registry, its digest computed over the bytes as read; each that does not is logged and
// skipped. What the scan read replaces what the last one did.
func (d *Discovery) Scan() {
	found := map[string]*gate.Manifest{}
	entries, err := os.ReadDir(d.dir)
	if errors.Is(err, fs.ErrNotExist) {
		d.log.Warn("the Plugin directory is absent, so no Plugin is available", "directory", d.dir)
	} else if err != nil {
		d.log.Error("the Plugin directory cannot be read", "directory", d.dir, "error", err)
	}
	for _, e := range entries {
		path := gate.ManifestPath(d.dir, e.Name())
		if d.bundled {
			plugin, isManifest := strings.CutSuffix(e.Name(), ".yaml")
			if e.IsDir() || !isManifest {
				continue
			}
			path = gate.BundleManifestPath(d.dir, plugin)
		} else if !e.IsDir() {
			continue
		}
		doc := gate.Read(path, gate.Composition)
		if errors.Is(doc.Err, fs.ErrNotExist) {
			continue
		}
		report, m := gate.CheckManifest(doc)
		for _, f := range report.Findings {
			d.log.Warn("a Manifest was refused and is skipped", "manifest", path, "code", f.Code, "location", f.Location, "finding", f.Message)
			if m == nil {
				d.tell(event.DocumentRejected(path, f.Code, f.Location))
			}
		}
		if m == nil {
			continue
		}
		sum := sha256.Sum256(doc.Bytes)
		digest := "sha256:" + hex.EncodeToString(sum[:])
		if err := d.registry.Declare(registry.Declared{ID: m.ID, Protocol: m.Protocol, ManifestDigest: digest}); err != nil {
			d.log.Error("a Manifest could not be declared", "manifest", path, "error", err)
			continue
		}
		found[m.ID] = m
		// A level announces a change: a Manifest read again at the digest it resolved at says nothing new.
		d.mu.Lock()
		changed := d.resolved[path] != digest
		d.resolved[path] = digest
		d.mu.Unlock()
		if changed {
			d.tell(event.DocumentResolved(path, m.ID, digest))
		}
	}
	d.mu.Lock()
	d.manifests = found
	d.mu.Unlock()
}

// Every scans again at each interval until the returned function is called.
func (d *Discovery) Every(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				d.Scan()
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { ticker.Stop(); close(done) }) }
}

// Manifest is a plugin's Manifest as the last scan read it: what the next admission is decided against.
func (d *Discovery) Manifest(id string) (*gate.Manifest, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	m, ok := d.manifests[id]
	return m, ok
}

// Available says whether anything on this host still declares the plugin: derived, and never stored.
func (d *Discovery) Available(id string) bool {
	_, ok := d.Manifest(id)
	return ok
}

// Units are what a deployment launches: each unit that starts with it, its program resolved — a
// plugin's from its identity in the executables directory — and every reference already resolved, a
// binding to its value and a secret or a storage need to its path in the state directory.
func Units(dep *gate.Deployment, manifest func(string) (*gate.Manifest, bool), executables, stateDir string) []supervisor.Unit {
	return units(dep, manifest, stateDir, nil, func(u gate.Unit) (string, string) {
		if u.Kind == "plugin" {
			return filepath.Join(executables, u.Plugin), ""
		}
		return u.Exec, ""
	})
}

// Descriptor are what a descriptor launches: each program inside the bundle, with the identity the
// descriptor states for it, and each parameter reference resolved against the values the Core was executed
// with — in the group named after the unit, then in `shared`.
func Descriptor(dep *gate.Deployment, manifest func(string) (*gate.Manifest, bool), bundle, stateDir string,
	values map[string]map[string]string) []supervisor.Unit {
	return units(dep, manifest, stateDir, values, func(u gate.Unit) (string, string) {
		if u.Exec == "" {
			return "", ""
		}
		return filepath.Join(bundle, u.Exec), u.Digest
	})
}

func units(dep *gate.Deployment, manifest func(string) (*gate.Manifest, bool), stateDir string,
	values map[string]map[string]string, program func(gate.Unit) (exec, digest string)) []supervisor.Unit {
	var out []supervisor.Unit
	kinds := map[string]unit.Kind{"plugin": unit.Plugin, "oneshot": unit.Oneshot, "interface": unit.Interface}
	for _, name := range dep.Order {
		u := dep.Units[name]
		if !u.Autostart {
			continue
		}
		materialised := map[string]string{}
		needs := u.Needs
		if m, ok := manifest(u.Plugin); ok && u.Kind == "plugin" {
			needs = m.Needs
		}
		for _, n := range needs {
			switch n.Class {
			case "secret":
				materialised[n.Key()] = filepath.Join(stateDir, "secrets", n.Name)
			case "storage":
				materialised[n.Key()] = filepath.Join(stateDir, "storage", n.Name)
			}
		}
		parameter := func(value string) string { return Substitute(values, u.Name, value) }
		resolve := func(value string) string {
			value = parameter(value)
			for key, bound := range u.Bind {
				bound = parameter(bound)
				value = strings.ReplaceAll(value, "${bind."+key+"}", bound)
			}
			for key, path := range materialised {
				value = strings.ReplaceAll(value, "${bind."+key+"}", path)
			}
			return value
		}
		s := supervisor.Unit{ID: name, Kind: kinds[u.Kind], Plugin: u.Plugin, Image: u.Image, RestartOnFailure: u.OnFailure, DependsOn: u.DependsOn, MigratesFrom: u.MigratesFrom, Env: map[string]string{}}
		for _, n := range needs {
			switch n.Class {
			case "device":
				s.Needs = append(s.Needs, supervisor.Need{Class: n.Class, Path: resolve(u.Bind[n.Key()])})
			case "storage", "secret":
				s.Needs = append(s.Needs, supervisor.Need{Class: n.Class, Path: materialised[n.Key()]})
			case "display", "audio", "network":
				s.Needs = append(s.Needs, supervisor.Need{Class: n.Class})
			}
		}
		s.Exec, s.Digest = program(u)
		for _, a := range u.Args {
			s.Args = append(s.Args, resolve(a))
		}
		for k, v := range u.Env {
			s.Env[k] = resolve(v)
		}
		p := u.Policy
		s.Policy = &supervisor.Policy{StartupWindow: p.StartupWindow, StopWindow: p.StopWindow, Backoff: p.Backoff, Ceiling: p.Ceiling, StabilityWindow: p.StabilityWindow}
		out = append(out, s)
	}
	return out
}

// Substitute resolves each parameter reference in value against the values given: in the group named
// first, then in `shared`. A `${bind.…}` is left for the binding it names.
func Substitute(values map[string]map[string]string, group, value string) string {
	return reference.ReplaceAllStringFunc(value, func(ref string) string {
		name := ref[2 : len(ref)-1]
		if strings.HasPrefix(name, "bind.") {
			return ref
		}
		if v, ok := values[group][name]; ok {
			return v
		}
		return values["shared"][name]
	})
}

// reference is one substitution, `${name}` or `${bind.<key>}`, which is always one whole value.
var reference = regexp.MustCompile(`\$\{[^}]+\}`)
