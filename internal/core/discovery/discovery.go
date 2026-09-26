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
	"strings"
	"sync"
	"time"

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
}

// New is discovery over the Plugin directory dir.
func New(dir string, r *registry.Registry, log *slog.Logger) *Discovery {
	return &Discovery{dir: dir, registry: r, log: log, manifests: map[string]*gate.Manifest{}}
}

// Scan reads every `<plugin>/manifest.yaml` under the directory. Each that passes its check is declared
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
		if !e.IsDir() {
			continue
		}
		path := gate.ManifestPath(d.dir, e.Name())
		doc := gate.Read(path, gate.Composition)
		if errors.Is(doc.Err, fs.ErrNotExist) {
			continue
		}
		report, m := gate.CheckManifest(doc)
		for _, f := range report.Findings {
			d.log.Warn("a Manifest was refused and is skipped", "manifest", path, "code", f.Code, "location", f.Location, "finding", f.Message)
		}
		if m == nil {
			continue
		}
		sum := sha256.Sum256(doc.Bytes)
		if err := d.registry.Declare(registry.Declared{ID: m.ID, Protocol: m.Protocol, ManifestDigest: "sha256:" + hex.EncodeToString(sum[:])}); err != nil {
			d.log.Error("a Manifest could not be declared", "manifest", path, "error", err)
			continue
		}
		found[m.ID] = m
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
// binding to its value and a secret to its path in the state directory.
func Units(dep *gate.Deployment, manifest func(string) (*gate.Manifest, bool), executables, stateDir string) []supervisor.Unit {
	var out []supervisor.Unit
	kinds := map[string]unit.Kind{"plugin": unit.Plugin, "oneshot": unit.Oneshot, "interface": unit.Interface}
	for _, name := range dep.Order {
		u := dep.Units[name]
		if !u.Autostart {
			continue
		}
		secrets := map[string]string{}
		needs := u.Needs
		if m, ok := manifest(u.Plugin); ok && u.Kind == "plugin" {
			needs = m.Needs
		}
		for _, n := range needs {
			if n.Class == "secret" {
				secrets[n.Key()] = filepath.Join(stateDir, "secrets", n.Name)
			}
		}
		resolve := func(value string) string {
			for key, bound := range u.Bind {
				value = strings.ReplaceAll(value, "${bind."+key+"}", bound)
			}
			for key, path := range secrets {
				value = strings.ReplaceAll(value, "${bind."+key+"}", path)
			}
			return value
		}
		s := supervisor.Unit{ID: name, Kind: kinds[u.Kind], Plugin: u.Plugin, Exec: u.Exec, RestartOnFailure: u.OnFailure, DependsOn: u.DependsOn, Env: map[string]string{}}
		if u.Kind == "plugin" {
			s.Exec = filepath.Join(executables, u.Plugin)
		}
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
