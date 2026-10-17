package compose

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// authored is a project made by init, with a program at components/setup.
func authored(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if code, out := run("init", "com.example.station", "--dir", dir); code != Done {
		t.Fatalf("init: %d %s", code, out)
	}
	os.WriteFile(filepath.Join(dir, Components, "setup"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	return dir
}

// written reads the descriptor's value at a path, as YAML, or nil.
func written(t *testing.T, file, path string) any {
	t.Helper()
	data, _ := os.ReadFile(file)
	var doc map[string]any
	yaml.Unmarshal(data, &doc)
	var v any = doc
	for _, p := range strings.Split(path, ".") {
		switch m := v.(type) {
		case map[string]any:
			v = m[p]
		case []any:
			i := 0
			for _, c := range p {
				i = i*10 + int(c-'0')
			}
			if i >= len(m) {
				return nil
			}
			v = m[i]
		default:
			return nil
		}
	}
	return v
}

// unchanged runs a verb that must refuse, and says whether the file is byte for byte what it was.
func unchanged(t *testing.T, file string, args ...string) {
	t.Helper()
	before, _ := os.ReadFile(file)
	code, out := run(args...)
	after, _ := os.ReadFile(file)
	if code != Refused || !bytes.Equal(before, after) {
		t.Errorf("%v exited %d (%s) and the file changed: %v", args, code, out, !bytes.Equal(before, after))
	}
}

func must(t *testing.T, args ...string) {
	t.Helper()
	if code, out := run(args...); code != Done {
		t.Fatalf("%v: %d %s", args, code, out)
	}
}

// std: yoke:the-authoring-verbs.01
func TestAddUnitWritesItsProgramArgumentsAndDependencies(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	must(t, "add", "unit", "store", "--kind", "oneshot", "--exec", "components/setup", "--arg", "one", "--arg", "two", "--dir", dir)
	must(t, "add", "unit", "acquire", "--dir", dir, "--kind", "oneshot", "--exec", "components/setup", "--depends-on", "store")
	if written(t, file, "units.store.kind") != "oneshot" || written(t, file, "units.store.exec") != "components/setup" ||
		!strings.HasPrefix(written(t, file, "units.store.digest").(string), "sha256:") ||
		len(written(t, file, "units.store.args").([]any)) != 2 || written(t, file, "units.acquire.depends_on").([]any)[0] != "store" {
		data, _ := os.ReadFile(file)
		t.Errorf("the descriptor is\n%s", data)
	}
	if r := Check(dir); r.Refused() {
		t.Errorf("the project is refused: %v", r.Findings)
	}
	unchanged(t, file, "add", "unit", "store", "--kind", "oneshot", "--exec", "components/setup", "--dir", dir)
	unchanged(t, file, "add", "unit", "late", "--kind", "oneshot", "--exec", "components/missing", "--dir", dir)
}

// std: yoke:the-authoring-verbs.02
func TestAddChannelWritesTransportCardinalityAndAddress(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	must(t, "add", "unit", "store", "--kind", "oneshot", "--exec", "components/setup", "--dir", dir)
	must(t, "add", "channel", "panel", "--transport", "http+ws", "--clients", "single", "--dir", dir)
	must(t, "add", "channel", "remote", "--transport", "local", "--clients", "multiple", "--class", "routable", "--host", "0.0.0.0", "--port", "8443", "--dir", dir)
	if written(t, file, "channels.panel.clients") != "single" || written(t, file, "channels.remote.address.security.transport") != "encrypted" ||
		written(t, file, "channels.remote.address.security.caller") != "authenticated" {
		data, _ := os.ReadFile(file)
		t.Errorf("the descriptor is\n%s", data)
	}
	unchanged(t, file, "add", "channel", "bad", "--transport", "carrier-pigeon", "--clients", "single", "--dir", dir)
}

// std: yoke:the-authoring-verbs.03
func TestAddParameterWritesADeclaration(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	must(t, "add", "parameter", "shared.label", "--type", "string", "--required", "--label", "Site label", "--dir", dir)
	must(t, "add", "parameter", "shared.rate", "--type", "integer", "--range", "[1, 10]", "--default", "5", "--label", "Rate", "--dir", dir)
	if written(t, file, "parameters.shared.label.required") != true || len(written(t, file, "parameters.shared.rate.range").([]any)) != 2 {
		data, _ := os.ReadFile(file)
		t.Errorf("the descriptor is\n%s", data)
	}
	unchanged(t, file, "add", "parameter", "shared.label", "--type", "string", "--label", "Again", "--dir", dir)
}

// std: yoke:the-authoring-verbs.04
func TestSetWritesAFieldByItsPath(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	must(t, "add", "unit", "store", "--kind", "oneshot", "--exec", "components/setup", "--dir", dir)
	data, _ := os.ReadFile(file)
	os.WriteFile(file, append([]byte("# the station's description\n"), data...), 0o644)
	must(t, "set", "units.store.policy.startup_window", "45s", "--dir", dir)
	must(t, "set", "units.store.args", "[a, b]", "--dir", dir)
	after, _ := os.ReadFile(file)
	if written(t, file, "units.store.policy.startup_window") != "45s" || len(written(t, file, "units.store.args").([]any)) != 2 ||
		!strings.Contains(string(after), "# the station's description") {
		t.Errorf("the descriptor is\n%s", after)
	}
	before, _ := os.ReadFile(file)
	code, out := run("set", "units.store.policy.startup_window", "soon", "--dir", dir)
	if code != Refused || !strings.Contains(out, "format.duration") {
		t.Errorf("a value the gate refuses: %d %s", code, out)
	}
	code, out = run("set", "units.store.colour", "red", "--dir", dir)
	if code != Refused || !strings.Contains(out, "units.store.colour") {
		t.Errorf("a path the model does not have: %d %s", code, out)
	}
	if now, _ := os.ReadFile(file); !bytes.Equal(before, now) {
		t.Error("a refused set changed the descriptor")
	}
}

// std: yoke:the-authoring-verbs.05
func TestArbitrateWritesOneRelation(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	must(t, "add", "unit", "store", "--kind", "oneshot", "--exec", "components/setup", "--dir", dir)
	for _, ch := range []string{"panel", "remote", "bench"} {
		must(t, "add", "channel", ch, "--transport", "local", "--clients", "single", "--dir", dir)
	}
	must(t, "arbitrate", "panel", "over", "remote", "bench", "--dir", dir)
	if written(t, file, "arbitration.0.prevails") != "panel" || len(written(t, file, "arbitration.0.over").([]any)) != 2 {
		data, _ := os.ReadFile(file)
		t.Errorf("the descriptor is\n%s", data)
	}
	unchanged(t, file, "arbitrate", "panel", "over", "nowhere", "--dir", dir)
}

// sample is a value the gate accepts for each field of the schema, in the context the coverage case
// builds: unit a a oneshot, unit p a plugin, unit i an interface; channel c local; rule 0 between c and d;
// group shared with an integer n, a string s and an enum e.
var sample = map[string][2]string{
	"model": {"", "1"}, "id": {"", "com.example.station"}, "version": {"", "1.0.0"}, "arch": {"", gate.HostArch()},
	"core": {"", ">=0.5"}, "data": {"", ">=1.0"},
	"parameters.*.*.type": {"parameters.shared.s.type", "string"}, "parameters.*.*.range": {"parameters.shared.n.range", "[1, 9]"},
	"parameters.*.*.pattern": {"parameters.shared.s.pattern", "^[a-z]+$"}, "parameters.*.*.values": {"parameters.shared.e.values", "[a, b, c]"},
	"parameters.*.*.required": {"parameters.shared.s.required", "true"}, "parameters.*.*.default": {"parameters.shared.n.default", "3"},
	"parameters.*.*.label": {"parameters.shared.s.label", "S"},
	"units.*.kind":         {"units.a.kind", "oneshot"}, "units.*.plugin": {"units.p.plugin", "com.example.bench"},
	"units.*.exec": {"units.a.exec", "components/setup"}, "units.*.image": {"units.c.image", "docker.io/library/busybox@sha256:" + strings.Repeat("a", 64)},
	"units.*.digest": {"units.a.digest", "sha256:" + strings.Repeat("0", 64)}, "units.*.manifest_digest": {"units.p.manifest_digest", "sha256:" + strings.Repeat("0", 64)},
	"units.*.args": {"units.a.args", "[x]"}, "units.*.env.*": {"units.a.env.MODE", "fast"}, "units.*.needs": {"units.a.needs", "[network]"},
	"units.*.bind.*": {"units.d.bind.zero", "/dev/zero"}, "units.*.depends_on": {"units.a.depends_on", "[p]"}, "units.*.autostart": {"units.a.autostart", "true"},
	"units.*.migrates_from": {"units.a.migrates_from", ">=0.9"},
	"channels.*.unit":       {"channels.c.unit", "i"}, "channels.*.transport": {"channels.c.transport", "local"}, "channels.*.clients": {"channels.c.clients", "multiple"},
	"channels.*.on_suspend": {"channels.c.on_suspend", "dark"}, "channels.*.address.class": {"channels.c.address.class", "loopback"},
	"channels.*.address.host": {"channels.r.address.host", "0.0.0.0"}, "channels.*.address.port": {"channels.r.address.port", "8443"},
	"channels.*.address.security.transport": {"channels.r.address.security.transport", "encrypted"},
	"channels.*.address.security.caller":    {"channels.r.address.security.caller", "authenticated"},
	"arbitration.*.prevails":                {"arbitration.0.prevails", "d"}, "arbitration.*.over": {"arbitration.0.over", "[c]"},
}

func init() {
	durations := map[string]string{"heartbeat.interval": "5s", "heartbeat.tolerance": "3", "startup_window": "30s", "stop_window": "10s",
		"restart.backoff": "1s", "restart.ceiling": "1m", "restart.stability_window": "5m", "restart.on_failure": "true",
		"retention.age": "7d", "retention.bytes": "10MB", "retention.entries": "1000"}
	for k, v := range durations {
		sample["units.*.policy."+k] = [2]string{"units.a.policy." + k, v}
		sample["policy."+k] = [2]string{"policy." + k, v}
	}
}

// context writes, by the verbs, the declarations every sample is written into.
func context(t *testing.T, program string, target ...string) {
	t.Helper()
	add := func(args ...string) { must(t, append(args, target...)...) }
	add("add", "unit", "a", "--kind", "oneshot", "--exec", program)
	if program == "components/setup" {
		add("add", "unit", "p", "--kind", "plugin", "--plugin", "com.example.bench", "--exec", program)
	} else {
		add("add", "unit", "p", "--kind", "plugin", "--plugin", "com.example.bench")
	}
	add("add", "unit", "i", "--kind", "interface", "--exec", program)
	add("add", "unit", "c", "--kind", "oneshot", "--image", "docker.io/library/busybox@sha256:"+strings.Repeat("b", 64))
	add("add", "unit", "d", "--kind", "oneshot", "--exec", program)
	add("set", "units.d.needs", "[device:zero]")
	add("add", "channel", "c", "--transport", "local", "--clients", "single")
	add("add", "channel", "d", "--transport", "local", "--clients", "single")
	add("add", "channel", "r", "--transport", "local", "--clients", "single", "--class", "routable", "--host", "127.0.0.1", "--port", "1")
	add("arbitrate", "c", "over", "d")
}

// std: yoke:the-authoring-verbs.06
func TestEveryFieldEitherDocumentCanSayIsWrittenByAVerb(t *testing.T) {
	dir := authored(t)
	os.WriteFile(filepath.Join(dir, Manifests, "com.example.bench.yaml"), []byte("manifest: 1\nid: com.example.bench\nprotocol: 1\n"), 0o644)
	context(t, "components/setup", "--dir", dir)
	must(t, "add", "parameter", "shared.n", "--type", "integer", "--default", "1", "--label", "N", "--dir", dir)
	must(t, "add", "parameter", "shared.s", "--type", "string", "--required", "--label", "S", "--dir", dir)
	must(t, "add", "parameter", "shared.e", "--type", "enum", "--values", "a,b", "--required", "--label", "E", "--dir", dir)
	composition := filepath.Join(t.TempDir(), "bench.yaml")
	os.WriteFile(composition, []byte("units: {}\n"), 0o644)
	manifests := t.TempDir()
	os.MkdirAll(filepath.Join(manifests, "com.example.bench"), 0o755)
	os.WriteFile(filepath.Join(manifests, "com.example.bench", "manifest.yaml"), []byte("manifest: 1\nid: com.example.bench\nprotocol: 1\n"), 0o644)
	os.MkdirAll(filepath.Join(filepath.Dir(composition), Components), 0o755)
	context(t, "/usr/lib/yoke/setup", "--file", composition, "--manifests", manifests)
	for _, target := range []struct {
		kind gate.Kind
		file string
		args []string
	}{{gate.Descriptor, filepath.Join(dir, Descriptor), []string{"--dir", dir}},
		{gate.Composition, composition, []string{"--file", composition, "--manifests", manifests}}} {
		for _, field := range gate.Fields(target.kind) {
			s, found := sample[field]
			if !found {
				t.Errorf("no verb is shown writing %s", field)
				continue
			}
			path, v := s[0], s[1]
			if path == "" {
				path = field
			}
			if target.kind == gate.Composition && field == "units.*.exec" {
				v = "/usr/lib/yoke/setup"
			}
			if code, out := run(append([]string{"set", path, v}, target.args...)...); code != Done {
				t.Errorf("%s in %s: %d %s", path, target.file, code, out)
				continue
			}
			if written(t, target.file, path) == nil {
				t.Errorf("%s is not in %s after it was written", path, target.file)
			}
		}
	}
}

// std: yoke:the-authoring-verbs.07
func TestAddUnitWritesItsBindings(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	os.MkdirAll(filepath.Join(dir, "manifests"), 0o755)
	os.WriteFile(filepath.Join(dir, "manifests", "com.example.head.yaml"),
		[]byte("manifest: 1\nid: com.example.head\nprotocol: 1\nneeds:\n  - device:instrument\n  - device:lamp\n"), 0o644)
	unchanged(t, file, "add", "unit", "head", "--kind", "plugin", "--plugin", "com.example.head", "--exec", "components/setup", "--dir", dir)
	must(t, "add", "unit", "head", "--kind", "plugin", "--plugin", "com.example.head", "--exec", "components/setup",
		"--bind", "instrument=/dev/spectro-a", "--bind", "lamp=/dev/lamp-a", "--dir", dir)
	if written(t, file, "units.head.bind.instrument") != "/dev/spectro-a" || written(t, file, "units.head.bind.lamp") != "/dev/lamp-a" {
		data, _ := os.ReadFile(file)
		t.Errorf("the descriptor is\n%s", data)
	}
	if r := Check(dir); r.Refused() {
		t.Errorf("the project is refused: %v", r.Findings)
	}
}
