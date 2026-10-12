package gate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yoke-project/yoke/internal/version"
)

// SocketCeiling is the longest Unix domain socket path the kernel binds: 108 bytes with the terminator.
const SocketCeiling = 107

// ObjectNameCeiling is the longest name a host-wide object may carry: a filename's limit.
const ObjectNameCeiling = 255

// SubscriberWidth bounds the one path component nobody declares, a subscriber's identity: the digits of
// a 64-bit counter. A counter can be budgeted for, and a name a client chose cannot.
const SubscriberWidth = 20

// ManifestPath is where the service form finds a plugin's Manifest in the scanned directory.
func ManifestPath(dir, plugin string) string { return filepath.Join(dir, plugin, "manifest.yaml") }

// BundleManifestPath is where a bundle holds a plugin's Manifest: `manifests/<plugin>.yaml`.
func BundleManifestPath(dir, plugin string) string { return filepath.Join(dir, plugin+".yaml") }

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
		if c.doc.Kind == Descriptor {
			path = BundleManifestPath(c.in.Manifests, u.Plugin)
		}
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
		if _, bound := u.Bind[n.Key()]; !bound && n.Class == "device" {
			c.refuse("unit.needs.unbound", at, "the plugin %s needs %s, and the unit %s binds nothing to %s", m.ID, written(n), u.Name, n.Key())
		}
	}
	for _, key := range sortedKeys(u.Bind) {
		n, needed := needs[key]
		if !needed {
			c.refuse("bind.undeclared", joined(at, key), "the unit %s binds %s, which the Manifest of %s never declares: the composing document cannot widen what it asked for", u.Name, key, m.ID)
			continue
		}
		c.unbindable(n, joined(at, key))
	}
	check := func(value, location string) {
		for _, match := range reference.FindAllStringSubmatch(value, -1) {
			key, isBind := strings.CutPrefix(match[1], "bind.")
			if isBind {
				if _, bound := u.Bind[key]; bound {
					continue
				}
				if n, needed := needs[key]; needed && materialised(n) {
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
	longestLabel := ""
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
					// A stream that tolerates loss may be carried on a ring, whose label derives from the
					// instance, the unit and the stream.
					if label := "yoke-" + h.Instance + "-" + name + "-" + s.ID; s.ToleratesLoss && len(label) > len(longestLabel) {
						longestLabel = label
					}
					fits(filepath.Join(h.RuntimeRoot, "plugins", name, "streams", s.ID+".sock"))
					fits(filepath.Join(h.RuntimeRoot, "plugins", name, "subscribers", s.ID, strings.Repeat("0", SubscriberWidth)))
				}
			}
		}
		fits(filepath.Join(h.RuntimeRoot, "plugins", name+".sock"))
		if c.doc.Kind == Descriptor {
			// A descriptor names every program, a Plugin's included, inside its bundle.
			executable, where = "", joined(location, "exec")
			if u.Exec != "" {
				executable = filepath.Join(h.Bundle, u.Exec)
			}
		}
		if executable != "" && at(Installing, Creating, Starting) {
			info, err := os.Stat(executable)
			switch {
			case err != nil:
				c.refuse("component.missing", where, "the executable %s of the unit %s is not there", executable, name)
			case !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0:
				c.refuse("component.not_executable", where, "the executable %s of the unit %s is there and cannot be executed", executable, name)
			case c.doc.Kind == Descriptor && u.Digest != "":
				if got, err := fileDigest(executable); err != nil {
					c.refuse("component.missing", where, "the executable %s of the unit %s cannot be read: %v", executable, name, err)
				} else if got != u.Digest {
					c.refuse("component.digest", joined(location, "digest"), "the executable %s of the unit %s is %s, and the descriptor states %s", executable, name, got, u.Digest)
				}
			}
		}
		if m := c.manifests[u.Plugin]; u.Kind == "plugin" && u.ManifestDigest != "" && m != nil && at(Installing, Creating, Starting) {
			if got, err := fileDigest(m.Path); err == nil && got != u.ManifestDigest {
				c.refuse("plugin.manifest.digest", joined(location, "manifest_digest"), "the Manifest %s of the plugin %s is %s, and the unit %s states %s", m.Path, u.Plugin, got, name, u.ManifestDigest)
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
	// The engine is looked for where a unit names an image, and only there.
	if h.Engine != nil && at(Installing, Creating, Starting) && slices.ContainsFunc(c.deployment.Order, func(name string) bool {
		return c.deployment.Units[name].Image != ""
	}) {
		if facts, err := h.Engine(); err != nil {
			c.refuse("engine.unreachable", "", "the deployment declares containerised units, and the container engine cannot be reached: %v", err)
		} else {
			c.engine = &facts
		}
	}
	if head := c.deployment.Head; c.doc.Kind == Descriptor && at(Installing, Creating, Starting) {
		if h.Arch != "" && head.Arch != h.Arch {
			c.refuse("arch.mismatch", "arch", "the descriptor is for %s, and this host is %s", head.Arch, h.Arch)
		}
		if h.Core != nil {
			if r, err := version.ParseRange(head.Core); err == nil && !r.Contains(*h.Core) {
				c.refuse("core.range", "core", "the descriptor needs a Core in %s, and this host executes %s", head.Core, h.Core)
			}
		}
	}
	if h.RuntimeDir != nil && at(Creating, Starting) {
		if info, err := os.Stat(*h.RuntimeDir); *h.RuntimeDir == "" || err != nil || !info.IsDir() {
			c.refuse("runtime.dir.missing", "", "the launching account has no runtime directory (XDG_RUNTIME_DIR %q), and an instance's sockets "+
				"and claim go nowhere else: give the account a login session through pam_systemd, or let it linger with "+
				"loginctl enable-linger", *h.RuntimeDir)
		}
	}
	if i := c.in.Instance; i != nil && c.doc.Kind == Descriptor {
		sum := sha256.Sum256(c.doc.Bytes)
		if got := "sha256:" + hex.EncodeToString(sum[:]); i.DescriptorDigest != "" && got != i.DescriptorDigest && at(Starting) {
			c.refuse("descriptor.digest", "", "the descriptor %s is %s, and the instance record pins %s", c.doc.Path, got, i.DescriptorDigest)
		}
		if err := Readable(c.deployment.Head.Data, i.Stamp, c.deployment.Head.Version); err != nil && at(Creating, Starting) {
			c.refuse("data.range", "data", "%v", err)
		}
	}
	if longest > SocketCeiling && at(Installing, Creating, Starting) {
		c.refuse("socket.path.ceiling", "", "the longest socket path this deployment can produce is %d characters, %s, and the ceiling is %d", longest, longestPath, SocketCeiling)
	}
	if len(longestLabel) > ObjectNameCeiling && at(Installing, Creating, Starting) {
		c.refuse("object.name.ceiling", "", "the longest name of a ring this deployment can create is %d characters, %s, and the ceiling is %d", len(longestLabel), longestLabel, ObjectNameCeiling)
	}
	return true
}

// Readable says whether a version declaring the data range given may read state the version stamped last
// wrote. An omitted upper end means the declaring version; no range, or no stamp, admits anything.
func Readable(data, stamp, ver string) error {
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

// fileDigest is a file's sha256 digest, as `sha256:<hex>`.
func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
