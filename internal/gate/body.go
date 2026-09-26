package gate

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Deployment is what a document that passed describes: the shared body, with every figure resolved.
type Deployment struct {
	Units       map[string]Unit
	Order       []string // the unit names, as written
	Channels    map[string]Channel
	Arbitration []Rule
	Policy      Policy // the deployment's scope, defaults applied
}

// Unit is one declared unit.
type Unit struct {
	Name           string
	Kind           string // plugin, oneshot or interface
	Plugin         string
	Exec           string
	Image          string
	Digest         string
	ManifestDigest string
	Args           []string
	Env            map[string]string
	Needs          []Need
	Bind           map[string]string
	DependsOn      []string
	Autostart      bool
	MigratesFrom   string
	Policy         Policy // the two scopes resolved
	OnFailure      bool   // a oneshot the author states is safe to run twice
}

// Need is one declared need: a class from the closed set, and a name where the class takes one.
type Need struct {
	Class string
	Name  string
}

// Key is what `bind` and `${bind.…}` call the need: its name where it has one, its class where not.
func (n Need) Key() string {
	if n.Name != "" {
		return n.Name
	}
	return n.Class
}

// Channel is one declared channel. It is managed when it names a unit, attached when it does not.
type Channel struct {
	Name      string
	Unit      string
	Transport string
	Clients   string
	Address   *Address // nil: a local socket
	OnSuspend string
}

// Address says where a channel is bound when it is not a local socket.
type Address struct {
	Class     string // local, loopback or routable
	Host      string
	Port      string
	Transport string // security.transport
	Caller    string // security.caller
}

// Rule is one arbitration relation.
type Rule struct {
	Prevails string
	Over     []string
}

// Policy is the deployment's figures. An omitted field takes the written default and never the zero of
// its encoding; an explicit zero on a retention limit means no constraint.
type Policy struct {
	HeartbeatInterval  time.Duration
	HeartbeatTolerance float64
	StartupWindow      time.Duration
	StopWindow         time.Duration
	Backoff            time.Duration
	Ceiling            time.Duration
	StabilityWindow    time.Duration
	RetentionAge       time.Duration
	RetentionBytes     int64
	RetentionEntries   int64
}

// Defaults are the written defaults.
func Defaults() Policy {
	return Policy{
		HeartbeatInterval: 10 * time.Second, HeartbeatTolerance: 3,
		StartupWindow: 30 * time.Second, StopWindow: 10 * time.Second,
		Backoff: 5 * time.Second, Ceiling: 5 * time.Minute, StabilityWindow: 60 * time.Second,
		RetentionAge: 7 * 24 * time.Hour, RetentionBytes: 50_000_000, RetentionEntries: 100_000,
	}
}

// overrides are the fields one scope wrote.
type overrides struct {
	heartbeatInterval, startupWindow, stopWindow, backoff, ceiling, stability, age *time.Duration
	tolerance                                                                      *float64
	bytes, entries                                                                 *int64
	onFailure                                                                      bool
}

func (o overrides) over(p Policy) Policy {
	set := func(dst *time.Duration, v *time.Duration) {
		if v != nil {
			*dst = *v
		}
	}
	set(&p.HeartbeatInterval, o.heartbeatInterval)
	set(&p.StartupWindow, o.startupWindow)
	set(&p.StopWindow, o.stopWindow)
	set(&p.Backoff, o.backoff)
	set(&p.Ceiling, o.ceiling)
	set(&p.StabilityWindow, o.stability)
	set(&p.RetentionAge, o.age)
	if o.tolerance != nil {
		p.HeartbeatTolerance = *o.tolerance
	}
	if o.bytes != nil {
		p.RetentionBytes = *o.bytes
	}
	if o.entries != nil {
		p.RetentionEntries = *o.entries
	}
	return p
}

var (
	identifier     = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)+$`)
	duration       = regexp.MustCompile(`^([0-9]+)(s|m|h|d)$`)
	size           = regexp.MustCompile(`^([0-9]+)(KB|MB|GB)$`)
	digest         = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	pinnedImage    = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)
	namedClasses   = []string{"device", "storage", "secret"}
	unnamedClasses = []string{"display", "audio", "network"}
	transports     = []string{"local", "http+ws"}
)

// headKeys are a descriptor's, read by the part that builds the application form.
var headKeys = []string{"model", "id", "version", "arch", "core", "data", "parameters"}

// shape is phase 1: keys, types, values, formats, what each kind and document may carry. It builds the
// deployment as it goes.
func (c *checker) shape() bool {
	d := &Deployment{Units: map[string]Unit{}, Channels: map[string]Channel{}}
	c.deployment = d
	root := c.root
	if !root.isMapping() {
		c.refuse("field.type", "", "the document is %s, not a mapping", root.text())
		return true
	}
	var units, channels, arbitration, policy *node
	for _, e := range root.entries() {
		switch {
		case e.key == "units":
			units = e.value
		case e.key == "channels":
			channels = e.value
		case e.key == "arbitration":
			arbitration = e.value
		case e.key == "policy":
			policy = e.value
		case c.doc.Kind == Descriptor && slices.Contains(headKeys, e.key):
		default:
			c.refuse("key.unknown", e.key, "the key %s is not part of the document", e.key)
		}
	}
	outer := overrides{}
	if policy != nil {
		outer = c.policy(policy, "policy", "")
	}
	d.Policy = outer.over(Defaults())
	if units == nil {
		c.refuse("field.required", "units", "the document declares no units")
	} else if c.mapping(units, "units") {
		for _, e := range units.entries() {
			if u, ok := c.unit(e.key, e.value, outer); ok {
				d.Units[u.Name] = u
				d.Order = append(d.Order, u.Name)
			}
		}
	}
	if channels != nil && c.mapping(channels, "channels") {
		for _, e := range channels.entries() {
			ch := c.channel(e.key, e.value)
			d.Channels[ch.Name] = ch
		}
	}
	if arbitration != nil {
		if !arbitration.isSequence() {
			c.refuse("field.type", "arbitration", "arbitration is %s, not a list of rules", arbitration.text())
		} else {
			for i, item := range arbitration.items() {
				d.Arbitration = append(d.Arbitration, c.rule(item, fmt.Sprintf("arbitration.%d", i)))
			}
		}
	}
	return true
}

func (c *checker) mapping(n *node, location string) bool {
	if !n.isMapping() {
		c.refuse("field.type", location, "%s is %s, not a mapping", location, n.text())
		return false
	}
	return true
}

// str reads a string. Where substitution is not permitted, a reference is refused and the value is
// reported as unusable.
func (c *checker) str(n *node, location string, permitted bool) (string, bool) {
	if !n.isScalar() {
		c.refuse("field.type", location, "%s is %s, not a string", location, n.text())
		return "", false
	}
	if !permitted && hasReference(n.n.Value) {
		c.refuse("substitution.place", location, "%s holds the reference %q, and substitution is not permitted there", location, n.n.Value)
		return "", false
	}
	return n.n.Value, true
}

func (c *checker) boolean(n *node, location string) (bool, bool) {
	if n.isScalar() && (n.n.Value == "true" || n.n.Value == "false") {
		return n.n.Value == "true", true
	}
	c.refuse("field.type", location, "%s is %q, not true or false", location, n.text())
	return false, false
}

func (c *checker) strings(n *node, location string, permitted bool) ([]string, bool) {
	if !n.isSequence() {
		c.refuse("field.type", location, "%s is %s, not a list", location, n.text())
		return nil, false
	}
	out := []string{}
	ok := true
	for i, item := range n.items() {
		s, good := c.str(item, fmt.Sprintf("%s.%d", location, i), permitted)
		ok = ok && good
		out = append(out, s)
	}
	return out, ok
}

func (c *checker) stringMap(n *node, location string, permitted bool) (map[string]string, bool) {
	if !c.mapping(n, location) {
		return nil, false
	}
	out := map[string]string{}
	ok := true
	for _, e := range n.entries() {
		s, good := c.str(e.value, joined(location, e.key), permitted)
		ok = ok && good
		out[e.key] = s
	}
	return out, ok
}

func (c *checker) enum(n *node, location string, set ...string) (string, bool) {
	s, ok := c.str(n, location, false)
	if !ok {
		return "", false
	}
	if !slices.Contains(set, s) {
		c.refuse("field.value", location, "%s is %q, which is not one of %s", location, s, strings.Join(set, ", "))
		return "", false
	}
	return s, true
}

func (c *checker) unit(name string, n *node, outer overrides) (Unit, bool) {
	location := joined("units", name)
	u := Unit{Name: name, Autostart: true, Env: map[string]string{}, Bind: map[string]string{}}
	good := true
	if !nameOK(name) {
		c.refuse("name.not_a_path_component", location, "the unit name %q is not a path component: no separator and no null byte", name)
		good = false
	}
	if !c.mapping(n, location) {
		return u, false
	}
	fields := map[string]*node{}
	for _, e := range n.entries() {
		fields[e.key] = e.value
	}
	known := []string{"kind", "plugin", "exec", "image", "digest", "manifest_digest", "args", "env", "needs", "bind", "depends_on", "autostart", "migrates_from", "policy"}
	for _, e := range n.entries() {
		if !slices.Contains(known, e.key) {
			c.refuse("key.unknown", joined(location, e.key), "the key %s is not part of a unit", e.key)
		}
	}
	at := func(key string) string { return joined(location, key) }

	if k, there := fields["kind"]; !there {
		c.refuse("field.required", at("kind"), "the unit %s has no kind", name)
	} else if kind, ok := c.enum(k, at("kind"), "plugin", "oneshot", "interface"); ok {
		u.Kind = kind
	}

	// The program, and what only some kinds carry.
	if p, there := fields["plugin"]; there {
		if u.Kind != "" && u.Kind != "plugin" {
			c.refuse("key.unknown", at("plugin"), "plugin is carried by a unit of kind plugin only, and %s is %s", name, u.Kind)
		} else if s, ok := c.str(p, at("plugin"), false); ok {
			if !identifier.MatchString(s) {
				c.refuse("field.value", at("plugin"), "the plugin %q is not an identifier of dot-separated lowercase labels", s)
			}
			u.Plugin = s
		}
	} else if u.Kind == "plugin" {
		c.refuse("field.required", at("plugin"), "the plugin unit %s names no plugin", name)
	}
	if e, there := fields["exec"]; there {
		if s, ok := c.str(e, at("exec"), false); ok {
			if c.doc.Kind == Composition && u.Kind != "plugin" && !strings.HasPrefix(s, "/") {
				c.refuse("field.value", at("exec"), "exec %q is not an absolute path, which a composition document requires", s)
			}
			u.Exec = s
		}
	}
	if i, there := fields["image"]; there {
		if s, ok := c.str(i, at("image"), false); ok {
			if !pinnedImage.MatchString(s) {
				c.refuse("unit.image.tagged", at("image"), "the image %q is not referenced by its digest", s)
			}
			u.Image = s
		}
	}
	_, hasExec := fields["exec"]
	_, hasImage := fields["image"]
	namesProgram := u.Kind != "plugin" || c.doc.Kind == Descriptor
	if u.Kind != "" && namesProgram && hasExec == hasImage {
		which := map[bool]string{true: "both exec and image", false: "neither exec nor image"}[hasExec]
		c.refuse("unit.program.ambiguous", location, "the unit %s names %s, and exactly one is required", name, which)
	}
	if dg, there := fields["digest"]; there {
		if s, ok := c.str(dg, at("digest"), false); ok {
			if !digest.MatchString(s) {
				c.refuse("format.digest", at("digest"), "the digest %q is not sha256:<64 hex digits>", s)
			}
			u.Digest = s
		}
	} else if c.doc.Kind == Descriptor && hasExec {
		c.refuse("unit.exec.no_digest", at("exec"), "the unit %s names exec with no digest", name)
	}
	if md, there := fields["manifest_digest"]; there {
		if u.Kind != "" && u.Kind != "plugin" {
			c.refuse("key.unknown", at("manifest_digest"), "manifest_digest is carried by a unit of kind plugin only")
		} else if s, ok := c.str(md, at("manifest_digest"), false); ok {
			if !digest.MatchString(s) {
				c.refuse("format.digest", at("manifest_digest"), "the digest %q is not sha256:<64 hex digits>", s)
			}
			u.ManifestDigest = s
		}
	}

	if a, there := fields["args"]; there {
		u.Args, _ = c.strings(a, at("args"), true)
	}
	if e, there := fields["env"]; there {
		if env, ok := c.stringMap(e, at("env"), true); ok {
			for _, entry := range e.entries() {
				if strings.HasPrefix(entry.key, "YOKE_") {
					c.refuse("unit.env.reserved", joined(at("env"), entry.key), "the variable %s is in the reserved prefix YOKE_, which the Core hands a process", entry.key)
				}
			}
			u.Env = env
		}
	}
	if nd, there := fields["needs"]; there {
		if u.Kind == "plugin" {
			c.refuse("unit.needs.on_plugin", at("needs"), "the plugin unit %s declares needs, which its Manifest states", name)
		} else if needs, ok := c.strings(nd, at("needs"), false); ok {
			for i, written := range needs {
				if need, ok := c.need(written, fmt.Sprintf("%s.%d", at("needs"), i)); ok {
					u.Needs = append(u.Needs, need)
				}
			}
		}
	}
	if b, there := fields["bind"]; there {
		if bind, ok := c.stringMap(b, at("bind"), true); ok {
			for _, entry := range b.entries() {
				for _, need := range u.Needs {
					if need.Class == "secret" && need.Key() == entry.key {
						c.refuse("bind.secret", joined(at("bind"), entry.key), "%s names the secret %s, which a document never binds", entry.key, need.Name)
					}
				}
			}
			u.Bind = bind
		}
	}
	if dp, there := fields["depends_on"]; there {
		u.DependsOn, _ = c.strings(dp, at("depends_on"), false)
	}
	if as, there := fields["autostart"]; there {
		if v, ok := c.boolean(as, at("autostart")); ok {
			u.Autostart = v
		}
	}
	if mf, there := fields["migrates_from"]; there {
		if u.Kind != "" && u.Kind != "oneshot" {
			c.refuse("unit.migrates_from.kind", at("migrates_from"), "migrates_from is carried by a unit of kind oneshot only, and %s is %s", name, u.Kind)
		} else if s, ok := c.str(mf, at("migrates_from"), false); ok {
			u.MigratesFrom = s
		}
	}
	inner := overrides{}
	if p, there := fields["policy"]; there {
		inner = c.policy(p, at("policy"), u.Kind)
	}
	u.Policy = inner.over(outer.over(Defaults()))
	u.OnFailure = inner.onFailure
	return u, good
}

// need reads `<class>` or `<class>:<name>` against the closed set.
func (c *checker) need(written, location string) (Need, bool) {
	class, name, named := strings.Cut(written, ":")
	switch {
	case slices.Contains(namedClasses, class) && named && nameOK(name):
		return Need{Class: class, Name: name}, true
	case slices.Contains(unnamedClasses, class) && !named:
		return Need{Class: class}, true
	}
	c.refuse("field.value", location, "the need %q is not one of device:<name>, storage:<name>, secret:<name>, display, audio, network", written)
	return Need{}, false
}

// policy reads one scope's block. The unit's scope is the only one that may say restart.on_failure, and
// only for a oneshot; a key a scope does not have is unknown there.
func (c *checker) policy(n *node, location, kind string) overrides {
	o := overrides{}
	if !c.mapping(n, location) {
		return o
	}
	dur := func(v *node, at string, zeroOK bool) *time.Duration {
		s, ok := c.str(v, at, false)
		if !ok {
			return nil
		}
		if zeroOK && s == "0" {
			zero := time.Duration(0)
			return &zero
		}
		m := duration.FindStringSubmatch(s)
		if m == nil {
			c.refuse("format.duration", at, "%s is %q, not an integer followed by s, m, h or d", at, s)
			return nil
		}
		count, _ := strconv.ParseInt(m[1], 10, 64)
		d := time.Duration(count) * map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour}[m[2]]
		return &d
	}
	block := func(v *node, at string, keys []string, read func(key string, v *node, at string)) {
		if !c.mapping(v, at) {
			return
		}
		for _, e := range v.entries() {
			if !slices.Contains(keys, e.key) {
				c.refuse("key.unknown", joined(at, e.key), "the key %s is not part of %s", e.key, at)
				continue
			}
			read(e.key, e.value, joined(at, e.key))
		}
	}
	block(n, location, []string{"heartbeat", "startup_window", "stop_window", "restart", "retention"}, func(key string, v *node, at string) {
		switch key {
		case "startup_window":
			o.startupWindow = dur(v, at, false)
		case "stop_window":
			o.stopWindow = dur(v, at, false)
		case "heartbeat":
			block(v, at, []string{"interval", "tolerance"}, func(key string, v *node, at string) {
				if key == "interval" {
					o.heartbeatInterval = dur(v, at, false)
					return
				}
				s, ok := c.str(v, at, false)
				if !ok {
					return
				}
				t, err := strconv.ParseFloat(s, 64)
				if err != nil {
					c.refuse("field.type", at, "%s is %q, not a number", at, s)
					return
				}
				if t < 1 {
					c.refuse("field.value", at, "%s is %s, and its minimum is 1.0", at, s)
					return
				}
				o.tolerance = &t
			})
		case "restart":
			keys := []string{"backoff", "ceiling", "stability_window"}
			if kind == "oneshot" {
				keys = append(keys, "on_failure")
			}
			block(v, at, keys, func(key string, v *node, at string) {
				switch key {
				case "backoff":
					o.backoff = dur(v, at, false)
				case "ceiling":
					o.ceiling = dur(v, at, false)
				case "stability_window":
					o.stability = dur(v, at, false)
				case "on_failure":
					o.onFailure, _ = c.boolean(v, at)
				}
			})
		case "retention":
			block(v, at, []string{"age", "bytes", "entries"}, func(key string, v *node, at string) {
				switch key {
				case "age":
					o.age = dur(v, at, true)
				case "bytes":
					s, ok := c.str(v, at, false)
					if !ok {
						return
					}
					var b int64
					if s != "0" {
						m := size.FindStringSubmatch(s)
						if m == nil {
							c.refuse("format.size", at, "%s is %q, not an integer followed by KB, MB or GB", at, s)
							return
						}
						count, _ := strconv.ParseInt(m[1], 10, 64)
						b = count * map[string]int64{"KB": 1_000, "MB": 1_000_000, "GB": 1_000_000_000}[m[2]]
					}
					o.bytes = &b
				case "entries":
					s, ok := c.str(v, at, false)
					if !ok {
						return
					}
					if !isInteger(s) {
						c.refuse("field.type", at, "%s is %q, not a count", at, s)
						return
					}
					e, _ := strconv.ParseInt(s, 10, 64)
					o.entries = &e
				}
			})
		}
	})
	return o
}

func (c *checker) channel(name string, n *node) Channel {
	location := joined("channels", name)
	ch := Channel{Name: name, OnSuspend: "read-only"}
	if !nameOK(name) {
		c.refuse("name.not_a_path_component", location, "the channel name %q is not a path component: no separator and no null byte", name)
	}
	if !c.mapping(n, location) {
		return ch
	}
	at := func(key string) string { return joined(location, key) }
	fields := map[string]*node{}
	for _, e := range n.entries() {
		if !slices.Contains([]string{"unit", "transport", "clients", "address", "on_suspend"}, e.key) {
			c.refuse("key.unknown", at(e.key), "the key %s is not part of a channel", e.key)
			continue
		}
		fields[e.key] = e.value
	}
	if u, there := fields["unit"]; there {
		ch.Unit, _ = c.str(u, at("unit"), false)
	}
	if t, there := fields["transport"]; there {
		ch.Transport, _ = c.str(t, at("transport"), false)
	} else {
		c.refuse("field.required", at("transport"), "the channel %s has no transport", name)
	}
	if cl, there := fields["clients"]; there {
		ch.Clients, _ = c.enum(cl, at("clients"), "single", "multiple")
	} else {
		c.refuse("field.required", at("clients"), "the channel %s does not say whether it takes one client or several", name)
	}
	if s, there := fields["on_suspend"]; there {
		ch.OnSuspend, _ = c.enum(s, at("on_suspend"), "read-only", "dark")
	}
	if a, there := fields["address"]; there {
		ch.Address = c.address(a, at("address"))
	}
	return ch
}

func (c *checker) address(n *node, location string) *Address {
	a := &Address{}
	if !c.mapping(n, location) {
		return a
	}
	at := func(key string) string { return joined(location, key) }
	fields := map[string]*node{}
	for _, e := range n.entries() {
		if !slices.Contains([]string{"class", "host", "port", "security"}, e.key) {
			c.refuse("key.unknown", at(e.key), "the key %s is not part of an address", e.key)
			continue
		}
		fields[e.key] = e.value
	}
	cl, there := fields["class"]
	if !there {
		c.refuse("field.required", at("class"), "the address has no class")
		return a
	}
	class, ok := c.enum(cl, at("class"), "local", "loopback", "routable")
	if !ok {
		return a
	}
	a.Class = class
	allowed := map[string][]string{"local": {}, "loopback": {"port"}, "routable": {"host", "port", "security"}}[class]
	for _, key := range []string{"host", "port", "security"} {
		v, there := fields[key]
		switch {
		case there && !slices.Contains(allowed, key):
			c.refuse("key.unknown", at(key), "an address of class %s carries no %s", class, key)
		case !there && slices.Contains(allowed, key) && key != "security":
			c.refuse("field.required", at(key), "an address of class %s requires %s", class, key)
		case there && key == "host":
			a.Host, _ = c.str(v, at(key), false)
		case there && key == "port":
			if s, ok := c.str(v, at(key), true); ok {
				if !hasReference(s) && !isInteger(s) {
					c.refuse("field.type", at(key), "the port %q is not a number", s)
				}
				a.Port = s
			}
		case there && key == "security":
			if !c.mapping(v, at(key)) {
				continue
			}
			for _, e := range v.entries() {
				switch e.key {
				case "transport":
					a.Transport, _ = c.enum(e.value, joined(at(key), e.key), "encrypted")
				case "caller":
					a.Caller, _ = c.enum(e.value, joined(at(key), e.key), "authenticated")
				default:
					c.refuse("key.unknown", joined(at(key), e.key), "the key %s is not part of security", e.key)
				}
			}
		}
	}
	return a
}

func (c *checker) rule(n *node, location string) Rule {
	r := Rule{}
	if !c.mapping(n, location) {
		return r
	}
	var prevails, over bool
	for _, e := range n.entries() {
		switch e.key {
		case "prevails":
			prevails = true
			r.Prevails, _ = c.str(e.value, joined(location, e.key), false)
		case "over":
			over = true
			r.Over, _ = c.strings(e.value, joined(location, e.key), false)
		default:
			c.refuse("key.unknown", joined(location, e.key), "the key %s is not part of an arbitration rule", e.key)
		}
	}
	if !prevails {
		c.refuse("field.required", joined(location, "prevails"), "the rule names no channel that prevails")
	}
	if !over {
		c.refuse("field.required", joined(location, "over"), "the rule names no channel it prevails over")
	}
	return r
}

// joins is phase 2: every reference inside one document.
func (c *checker) joins() bool {
	d := c.deployment
	for _, name := range d.Order {
		u := d.Units[name]
		for i, dep := range u.DependsOn {
			at := fmt.Sprintf("units.%s.depends_on.%d", name, i)
			target, there := d.Units[dep]
			switch {
			case !there:
				c.refuse("depends_on.unknown", at, "the unit %s depends on %s, which is not declared", name, dep)
			case target.Kind == "interface":
				c.refuse("depends_on.interface", at, "the unit %s depends on %s, a managed interface, and nothing may depend on one", name, dep)
			}
		}
		c.resolve(u)
	}
	for _, name := range sortedKeys(d.Channels) {
		ch := d.Channels[name]
		at := joined("channels", name)
		if ch.Unit != "" {
			target, there := d.Units[ch.Unit]
			switch {
			case !there:
				c.refuse("channel.unit.unknown", joined(at, "unit"), "the channel %s names the unit %s, which is not declared", name, ch.Unit)
			case target.Kind != "interface":
				c.refuse("channel.unit.kind", joined(at, "unit"), "the channel %s names %s, which is of kind %s and not interface", name, ch.Unit, target.Kind)
			}
		}
		if ch.Transport != "" && !slices.Contains(transports, ch.Transport) {
			c.refuse("channel.transport.unknown", joined(at, "transport"), "the transport %q is not one of %s", ch.Transport, strings.Join(transports, ", "))
		}
		if a := ch.Address; a != nil {
			if a.Class == "routable" && (a.Transport == "" || a.Caller == "") {
				c.refuse("channel.address.security", joined(at, "address"), "the routable address of %s must state both security.transport and security.caller", name)
			}
			for _, ref := range reference.FindAllString(a.Port, -1) {
				c.refuse("substitution.unresolved", joined(at, "address.port"), "the reference %s in the channel %s resolves in neither scope", ref, name)
			}
		}
	}
	for i, r := range d.Arbitration {
		for j, named := range append([]string{r.Prevails}, r.Over...) {
			if named == "" {
				continue
			}
			if _, there := d.Channels[named]; !there {
				at := fmt.Sprintf("arbitration.%d.prevails", i)
				if j > 0 {
					at = fmt.Sprintf("arbitration.%d.over.%d", i, j-1)
				}
				c.refuse("arbitration.channel.unknown", at, "the rule names the channel %s, which is not declared", named)
			}
		}
	}
	return true
}

// resolve refuses every reference in a unit that resolves in neither scope. A binding resolves against
// what this unit binds and the secrets it needs; a plugin unit's secrets are its Manifest's, joined in
// the phase that reads it. A parameter resolves against the descriptor's parameters, which a composition
// document does not have.
func (c *checker) resolve(u Unit) {
	secrets := map[string]bool{}
	for _, n := range u.Needs {
		if n.Class == "secret" {
			secrets[n.Key()] = true
		}
	}
	check := func(value, at string) {
		for _, m := range reference.FindAllStringSubmatch(value, -1) {
			ref, name := m[0], m[1]
			if key, isBind := strings.CutPrefix(name, "bind."); isBind {
				if _, bound := u.Bind[key]; bound || secrets[key] || u.Kind == "plugin" {
					continue
				}
			} else if c.doc.Kind == Descriptor {
				continue
			}
			c.refuse("substitution.unresolved", at, "the reference %s in the unit %s resolves in neither scope", ref, u.Name)
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

// weaker is phase 5: arrangements that give something up, reported and never refusing.
func (c *checker) weaker() bool {
	for _, name := range sortedKeys(c.deployment.Channels) {
		if a := c.deployment.Channels[name].Address; a != nil && a.Class == "loopback" {
			c.note("channel.address.loopback", joined(joined("channels", name), "address"),
				"the channel %s is bound on the loopback address, which keeps none of the first trust layer and requires nothing of a caller", name)
		}
	}
	return true
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
