package gate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// SocketCeiling is the longest Unix domain socket path the kernel binds: 108 bytes with the terminator.
const SocketCeiling = 107

// SubscriberWidth bounds the one path component nobody declares, a subscriber's identity: the digits of
// a 64-bit counter. A counter can be budgeted for, and a name a client chose cannot.
const SubscriberWidth = 20

// ManifestPath is where the service form finds a plugin's Manifest in the scanned directory.
func ManifestPath(dir, plugin string) string { return filepath.Join(dir, plugin, "manifest.yaml") }

// crossDocument is phase 3: each plugin unit against its Manifest, needs against bindings in both
// directions. A Manifest the document names is checked, and what it refuses refuses the deployment.
func (c *checker) crossDocument() bool {
	if c.in.Manifests == "" {
		return false
	}
	c.manifests = map[string]*Manifest{}
	checked := map[string]bool{}
	for _, name := range c.deployment.Order {
		u := c.deployment.Units[name]
		if u.Kind != "plugin" {
			continue
		}
		path := ManifestPath(c.in.Manifests, u.Plugin)
		if !checked[u.Plugin] {
			checked[u.Plugin] = true
			if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
				c.refuse("plugin.manifest.missing", fmt.Sprintf("units.%s.plugin", name), "the unit %s names the plugin %s, and this host has no Manifest for it at %s", name, u.Plugin, path)
				continue
			}
			report, m := CheckManifest(Read(path, Composition))
			for _, f := range report.Findings {
				c.report.Findings = append(c.report.Findings, f)
			}
			if m != nil {
				c.manifests[u.Plugin] = m
			}
		}
		if m := c.manifests[u.Plugin]; m != nil {
			c.bindings(u, m)
		}
	}
	return true
}

// bindings joins a plugin unit's binding to what its Manifest needs: every need but a secret is bound,
// nothing is bound that is not needed, a secret is never bound, and every reference resolves.
func (c *checker) bindings(u Unit, m *Manifest) {
	at := fmt.Sprintf("units.%s.bind", u.Name)
	needs := map[string]Need{}
	for _, n := range m.Needs {
		needs[n.Key()] = n
		if _, bound := u.Bind[n.Key()]; !bound && n.Class != "secret" {
			c.refuse("unit.needs.unbound", at, "the plugin %s needs %s, and the unit %s binds nothing to %s", m.ID, written(n), u.Name, n.Key())
		}
	}
	for _, key := range sortedKeys(u.Bind) {
		n, needed := needs[key]
		switch {
		case !needed:
			c.refuse("bind.undeclared", joined(at, key), "the unit %s binds %s, which the Manifest of %s never declares: the composing document cannot widen what it asked for", u.Name, key, m.ID)
		case n.Class == "secret":
			c.refuse("bind.secret", joined(at, key), "%s names the secret %s, which a document never binds", key, n.Name)
		}
	}
	check := func(value, location string) {
		for _, match := range reference.FindAllStringSubmatch(value, -1) {
			key, isBind := strings.CutPrefix(match[1], "bind.")
			if isBind {
				if _, bound := u.Bind[key]; bound {
					continue
				}
				if n, needed := needs[key]; needed && n.Class == "secret" {
					continue
				}
			}
			c.refuse("substitution.unresolved", location, "the reference %s in the unit %s resolves in neither scope", match[0], u.Name)
		}
	}
	for i, a := range u.Args {
		check(a, fmt.Sprintf("units.%s.args.%d", u.Name, i))
	}
	for _, k := range sortedKeys(u.Env) {
		check(u.Env[k], fmt.Sprintf("units.%s.env.%s", u.Name, k))
	}
	for _, k := range sortedKeys(u.Bind) {
		check(u.Bind[k], fmt.Sprintf("units.%s.bind.%s", u.Name, k))
	}
}

func written(n Need) string {
	if n.Name != "" {
		return n.Class + ":" + n.Name
	}
	return n.Class
}

// hostFacts is phase 4: what the host that will run the deployment must hold. It is not run while
// composing, where the host in hand is not the target.
func (c *checker) hostFacts() bool {
	h := c.in.Host
	if h == nil || c.in.Moment == Composing {
		return false
	}
	at := func(marks ...Moment) bool { return slices.Contains(marks, c.in.Moment) }
	longest, longestPath := 0, ""
	fits := func(path string) {
		if len(path) > longest {
			longest, longestPath = len(path), path
		}
	}
	for _, name := range []string{"plugin.sock", "operator.sock", "shell.sock"} {
		fits(filepath.Join(h.RuntimeRoot, name))
	}
	for _, name := range c.deployment.Order {
		u := c.deployment.Units[name]
		location := joined("units", name)
		needs := u.Needs
		executable, where := u.Exec, joined(location, "exec")
		if u.Kind == "plugin" {
			executable, where = filepath.Join(h.Executables, u.Plugin), joined(location, "plugin")
			if m := c.manifests[u.Plugin]; m != nil {
				needs = m.Needs
				for _, s := range m.Streams {
					fits(filepath.Join(h.RuntimeRoot, "plugins", name, "streams", s.ID+".sock"))
					fits(filepath.Join(h.RuntimeRoot, "plugins", name, "subscribers", s.ID, strings.Repeat("0", SubscriberWidth)))
				}
			}
		}
		fits(filepath.Join(h.RuntimeRoot, "plugins", name+".sock"))
		if executable != "" && at(Installing, Creating, Starting) {
			info, err := os.Stat(executable)
			switch {
			case err != nil:
				c.refuse("component.missing", where, "the executable %s of the unit %s is not there", executable, name)
			case !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0:
				c.refuse("component.not_executable", where, "the executable %s of the unit %s is there and cannot be executed", executable, name)
			}
		}
		for _, n := range needs {
			switch {
			case n.Class == "device" && at(Creating, Starting):
				path, bound := u.Bind[n.Key()]
				if !bound || hasReference(path) {
					continue
				}
				if _, err := os.Stat(path); err != nil {
					c.refuse("path.missing", joined(joined(location, "bind"), n.Key()), "the unit %s binds %s to %s, which does not exist on this host", name, n.Key(), path)
				}
			case n.Class == "secret" && at(Starting):
				path := filepath.Join(h.StateDir, "secrets", n.Name)
				if _, err := os.Stat(path); err != nil {
					c.refuse("secret.missing", location, "the unit %s needs the secret %s, and the instance's store holds none at %s", name, n.Name, path)
				}
			}
		}
	}
	if longest > SocketCeiling && at(Installing, Creating, Starting) {
		c.refuse("socket.path.ceiling", "", "the longest socket path this deployment can produce is %d characters, %s, and the ceiling is %d", longest, longestPath, SocketCeiling)
	}
	return true
}
