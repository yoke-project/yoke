package gate

import (
	"slices"
	"strings"
)

// A field the description model can say, by the path a finding carries: `*` stands for a name the author
// chooses — a unit's, a channel's, a group's, a parameter's, a variable's — or for a rule's place in its
// list. A field that holds a list is one field.
type field struct {
	path           string
	descriptorOnly bool
}

// policyFields are what a policy block can say, in the document's scope and in a unit's. restart.on_failure
// is a unit's alone, and a oneshot's at that, which the checker says where it reads it.
var policyFields = []string{
	"heartbeat.interval", "heartbeat.tolerance", "startup_window", "stop_window",
	"restart.backoff", "restart.ceiling", "restart.stability_window", "restart.on_failure",
	"retention.age", "retention.bytes", "retention.entries",
}

// schema is every field either document can say. The checker reads from here the keys each object may
// hold, so what an authoring tool compares itself against is what is validated.
var schema = func() []field {
	f := []field{
		{"model", true}, {"id", true}, {"version", true}, {"arch", true}, {"core", true}, {"data", true},
		{"parameters.*.*.type", true}, {"parameters.*.*.range", true}, {"parameters.*.*.pattern", true},
		{"parameters.*.*.values", true}, {"parameters.*.*.required", true}, {"parameters.*.*.default", true},
		{"parameters.*.*.label", true},
	}
	for _, k := range []string{"kind", "plugin", "exec", "image", "args", "env.*", "needs", "bind.*", "depends_on", "autostart"} {
		f = append(f, field{path: "units.*." + k})
	}
	// A descriptor states the digests of what its bundle carries and the versions a unit migrates from;
	// a composition document names the system's files and has no version.
	for _, k := range []string{"digest", "manifest_digest", "migrates_from"} {
		f = append(f, field{path: "units.*." + k, descriptorOnly: true})
	}
	for _, p := range policyFields {
		if p != "restart.on_failure" {
			f = append(f, field{path: "policy." + p})
		}
		f = append(f, field{path: "units.*.policy." + p})
	}
	for _, k := range []string{"unit", "transport", "clients", "on_suspend", "address.class", "address.host", "address.port",
		"address.security.transport", "address.security.caller"} {
		f = append(f, field{path: "channels.*." + k})
	}
	return append(f, field{path: "arbitration.*.prevails"}, field{path: "arbitration.*.over"})
}()

// Fields are every field a document of the kind given can say, by path, in a fixed order.
func Fields(k Kind) []string {
	var out []string
	for _, f := range schema {
		if !f.descriptorOnly || k == Descriptor {
			out = append(out, f.path)
		}
	}
	return out
}

// keysAt are the keys an object at the pattern given may hold, in the order the schema states them.
func keysAt(pattern string) []string {
	var keys []string
	for _, f := range schema {
		rest, found := strings.CutPrefix(f.path, pattern+".")
		if !found {
			continue
		}
		key, _, _ := strings.Cut(rest, ".")
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}
