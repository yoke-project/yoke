package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// std: yoke:the-remove-verb.01
func TestRemoveTakesAwayTheNodeAndEverythingBeneathIt(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	must(t, "add", "unit", "setup", "--kind", "oneshot", "--exec", "components/setup", "--dir", dir)
	must(t, "add", "unit", "probe", "--kind", "oneshot", "--exec", "components/setup", "--dir", dir)
	must(t, "set", "units.setup.policy.startup_window", "30s", "--dir", dir)
	must(t, "add", "parameter", "shared.site", "--type", "string", "--required", "--label", "Site", "--dir", dir)
	must(t, "add", "channel", "panel", "--transport", "local", "--clients", "single", "--dir", dir)

	for _, path := range []string{"units.setup.policy.startup_window", "units.probe", "parameters.shared.site", "channels.panel"} {
		code, out := run("remove", path, "--dir", dir)
		if code != Done || !strings.Contains(out, "removed "+path) {
			t.Errorf("remove %s exited %d: %s", path, code, out)
		}
		if written(t, file, path) != nil {
			t.Errorf("%s is still written", path)
		}
	}
	if written(t, file, "units.setup.policy") != nil {
		t.Error("the emptied policy of setup is still written")
	}
	if written(t, file, "units.setup.kind") != "oneshot" || written(t, file, "units.setup.exec") != "components/setup" ||
		written(t, file, "id") != "com.example.station" {
		t.Errorf("what was not removed changed:\n%s", raw(t, file))
	}

	composition := filepath.Join(t.TempDir(), "bench.yaml")
	os.WriteFile(composition, []byte("units:\n  a: { kind: oneshot, exec: /usr/lib/yoke/setup }\n  b: { kind: oneshot, exec: /usr/lib/yoke/setup }\n"), 0o644)
	manifests := t.TempDir()
	if code, out := run("remove", "units.a", "--file", composition, "--manifests", manifests); code != Done {
		t.Errorf("remove from the composition document exited %d: %s", code, out)
	}
	if written(t, composition, "units.a") != nil || written(t, composition, "units.b.kind") != "oneshot" {
		t.Errorf("the composition document reads:\n%s", raw(t, composition))
	}
}

// std: yoke:the-remove-verb.02
func TestARemovalTheGateWouldRefuseIsNotMade(t *testing.T) {
	dir := authored(t)
	file := filepath.Join(dir, Descriptor)
	must(t, "add", "parameter", "shared.site", "--type", "string", "--required", "--label", "Site", "--dir", dir)
	must(t, "add", "unit", "setup", "--kind", "oneshot", "--exec", "components/setup", "--arg", "${site}", "--dir", dir)
	must(t, "add", "unit", "probe", "--kind", "oneshot", "--exec", "components/setup", "--depends-on", "setup", "--dir", dir)

	for _, c := range []struct{ path, says string }{
		{"units.setup", "units.probe.depends_on"},
		{"parameters.shared.site", "units.setup.args"},
		{"version", "version"},
		{"units.nothing", "nothing is at units.nothing"},
	} {
		before, _ := os.ReadFile(file)
		code, out := run("remove", c.path, "--dir", dir)
		after, _ := os.ReadFile(file)
		if code != Refused || string(before) != string(after) {
			t.Errorf("remove %s exited %d and the file changed: %v; it said %s", c.path, code, string(before) != string(after), out)
		}
		if !strings.Contains(out, c.says) {
			t.Errorf("remove %s said %q, which does not name %s", c.path, out, c.says)
		}
	}
}

// std: yoke:the-remove-verb.03
func TestAPlacedBlocksLineGoesWithItsUnitAndNothingElseIsDeleted(t *testing.T) {
	dir, block := blocked(t)
	if err := AddBlock(dir, block, "acquire-1", placedAt); err != nil {
		t.Fatal(err)
	}
	if err := AddBlock(dir, block, "acquire-2", placedAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	must(t, "add", "unit", "store", "--kind", "oneshot", "--exec", "components/setup", "--dir", dir)
	must(t, "set", "units.store.needs", "[storage:datasets]", "--dir", dir)
	os.WriteFile(filepath.Join(dir, Manifests, "com.example.bench.yaml"), []byte("manifest: 1\nid: com.example.bench\nprotocol: 1\n"), 0o644)
	if err := Waive(dir, DataRangeUnstated, "units.store", "the store is rebuilt from the instrument", placedAt); err != nil {
		t.Fatal(err)
	}

	must(t, "remove", "channels.acquire-2", "--dir", dir)
	must(t, "remove", "units.acquire-2", "--dir", dir)
	var placedAs []string
	for _, p := range placements(t, dir) {
		placedAs = append(placedAs, p["placed_as"].(string))
	}
	if strings.Join(placedAs, " ") != "acquire-1" {
		t.Errorf("blocks.yaml holds the placements %v", placedAs)
	}
	file := filepath.Join(dir, Descriptor)
	if written(t, file, "parameters.acquire-2") != nil || written(t, file, "parameters.acquire-1.instrument.type") != "string" {
		t.Errorf("the group acquire-2 did not go with its unit, or acquire-1's went too:\n%s", raw(t, file))
	}

	must(t, "remove", "units.store", "--dir", dir)
	ws, err := Waivers(dir)
	if err != nil || len(ws) != 1 || ws[0].At != "units.store" || !ws[0].Stale {
		t.Errorf("the waivers are %+v (%v)", ws, err)
	}
	for _, f := range []string{filepath.Join(Components, "setup"), filepath.Join(Components, "acquire"), filepath.Join(Manifests, "com.example.bench.yaml")} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s is gone: %v", f, err)
		}
	}
}
