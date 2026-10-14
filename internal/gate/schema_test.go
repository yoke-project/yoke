package gate_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// named is a path with each `*` given the name a document writing it uses: unit a, channel c, group
// shared and parameter p, variable X, binding zero, and the first rule.
func named(path string) string {
	parts := strings.Split(path, ".")
	for i, p := range parts {
		if p != "*" {
			continue
		}
		switch parent := parts[i-1]; {
		case parent == "units":
			parts[i] = "a"
		case parent == "channels":
			parts[i] = "c"
		case parent == "parameters":
			parts[i] = "shared"
		case i >= 2 && parts[i-2] == "parameters":
			parts[i] = "p"
		case parent == "env":
			parts[i] = "X"
		case parent == "bind":
			parts[i] = "zero"
		case parent == "arbitration":
			parts[i] = "0"
		}
	}
	return strings.Join(parts, ".")
}

// documentWriting is a document of the kind given, valid but for what is written at path, which holds v.
func documentWriting(t *testing.T, kind gate.Kind, path string) string {
	t.Helper()
	unit := map[string]any{"kind": "oneshot"}
	if strings.Contains(path, ".plugin") || strings.Contains(path, "manifest_digest") {
		unit["kind"] = "plugin"
	}
	doc := map[string]any{"units": map[string]any{"a": unit}}
	if kind == gate.Descriptor {
		for k, v := range map[string]any{"model": 1, "id": "com.example.station", "version": "1.0.0", "arch": "x86_64", "core": ">=0.1"} {
			doc[k] = v
		}
	}
	set := func(at string, v any) {
		parts := strings.Split(named(at), ".")
		if parts[0] == "arbitration" {
			rule := map[string]any{}
			doc["arbitration"] = []any{rule}
			rule[parts[2]] = v
			return
		}
		m := doc
		for _, p := range parts[:len(parts)-1] {
			next, ok := m[p].(map[string]any)
			if !ok {
				next = map[string]any{}
				m[p] = next
			}
			m = next
		}
		m[parts[len(parts)-1]] = v
	}
	if strings.HasPrefix(path, "channels.*.address.") && path != "channels.*.address.class" {
		set("channels.*.address.class", "routable")
	}
	set(path, "v")
	out, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func checked(t *testing.T, kind gate.Kind, text string) gate.Report {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.yaml")
	os.WriteFile(path, []byte(text), 0o644)
	r, _ := gate.Check(gate.Input{Document: gate.Read(path, kind), Moment: gate.Composing, Manifests: t.TempDir()})
	return r
}

// std: yoke:the-gate.20
func TestTheSchemaTheGateCarriesIsWhatItAccepts(t *testing.T) {
	objects := map[string]bool{}
	for _, kind := range []gate.Kind{gate.Descriptor, gate.Composition} {
		for _, path := range gate.Fields(kind) {
			r := checked(t, kind, documentWriting(t, kind, path))
			for _, f := range r.Findings {
				if f.Code == "key.unknown" {
					t.Errorf("%s, written in a document of kind %d, is unknown: %s at %s", path, kind, f.Message, f.Location)
				}
			}
			parts := strings.Split(path, ".")
			for i := 1; i < len(parts); i++ {
				if parts[i] != "*" && parts[i-1] != "*" || parts[i-1] == "*" {
					if parts[i] != "*" {
						objects[strings.Join(parts[:i], ".")] = true
					}
				}
			}
		}
	}
	for object := range objects {
		if strings.HasSuffix(object, ".env") || strings.HasSuffix(object, ".bind") || !strings.Contains(object, ".") && object != "policy" {
			continue
		}
		path := object + ".zzz"
		r := checked(t, gate.Descriptor, documentWriting(t, gate.Descriptor, path))
		if !slices.ContainsFunc(r.Findings, func(f gate.Finding) bool { return f.Code == "key.unknown" && f.Location == named(path) }) {
			t.Errorf("a key the schema does not name, at %s, was not refused: %v", named(path), r.Findings)
		}
	}
	descriptorOnly := 0
	for _, path := range gate.Fields(gate.Descriptor) {
		if slices.Contains(gate.Fields(gate.Composition), path) {
			continue
		}
		descriptorOnly++
		r := checked(t, gate.Composition, documentWriting(t, gate.Composition, path))
		top, _, _ := strings.Cut(path, ".")
		if !slices.ContainsFunc(r.Findings, func(f gate.Finding) bool {
			return f.Class == gate.Refusal && (f.Location == top || f.Location == named(path))
		}) {
			t.Errorf("%s was not refused in a composition document: %v", path, r.Findings)
		}
	}
	if descriptorOnly == 0 || strconv.Itoa(len(gate.Fields(gate.Descriptor))) == "0" {
		t.Error("the schema names no descriptor-only field")
	}
}
