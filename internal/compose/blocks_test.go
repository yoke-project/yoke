package compose

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

// headBlock is the block acquisition-head: an interface running components/acquire, its group declaring
// instrument, and a channel it serves.
const headBlock = `block: acquisition-head
model: 1
unit:
  kind: interface
  exec: components/acquire
  args: [ "--device", "${instrument}" ]
parameters:
  instrument:
    type: string
    required: true
    label: "The acquisition head this copy drives"
channels:
  acquisition-head: { transport: local, clients: single, unit: acquisition-head }
`

// blocked is a project made by init holding components/acquire, and the block's file in a directory of
// its own beside it.
func blocked(t *testing.T) (dir, block string) {
	t.Helper()
	dir = authored(t)
	os.WriteFile(filepath.Join(dir, Components, "acquire"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	shelf := filepath.Join(t.TempDir(), "head")
	os.MkdirAll(shelf, 0o755)
	block = filepath.Join(shelf, "acquisition-head.yaml")
	os.WriteFile(block, []byte(headBlock), 0o644)
	return dir, block
}

func raw(t *testing.T, path string) string {
	t.Helper()
	data, _ := os.ReadFile(path)
	return string(data)
}

var placedAt = time.Date(2026, 10, 15, 10, 4, 11, 0, time.UTC)

func placements(t *testing.T, dir string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, Blocks))
	if err != nil {
		return nil
	}
	var doc struct {
		Placements []map[string]any `yaml:"placements"`
	}
	yaml.Unmarshal(data, &doc)
	return doc.Placements
}

// std: yoke:blocks.01
func TestPlacingABlockExpandsItAndRecordsWhereItCameFrom(t *testing.T) {
	dir, block := blocked(t)
	file := filepath.Join(dir, Descriptor)
	if err := AddBlock(dir, block, "acquire-2", placedAt); err != nil {
		t.Fatal(err)
	}
	program, _ := os.ReadFile(filepath.Join(dir, Components, "acquire"))
	if written(t, file, "units.acquire-2.kind") != "interface" || written(t, file, "units.acquire-2.exec") != "components/acquire" ||
		written(t, file, "units.acquire-2.digest") != fmt.Sprintf("sha256:%x", sha256.Sum256(program)) ||
		written(t, file, "units.acquire-2.args.1") != "${instrument}" ||
		written(t, file, "parameters.acquire-2.instrument.type") != "string" ||
		written(t, file, "channels.acquire-2.unit") != "acquire-2" || written(t, file, "channels.acquire-2.transport") != "local" {
		data, _ := os.ReadFile(file)
		t.Fatalf("the descriptor is:\n%s", data)
	}
	if data, _ := os.ReadFile(file); strings.Contains(string(data), "acquisition-head") || strings.Contains(string(data), "block") {
		t.Errorf("the descriptor still names the block:\n%s", data)
	}
	if r := Check(dir); r.Refused() {
		t.Errorf("check refuses %v", r.Findings)
	}
	if code, out := run("block", "add", filepath.Dir(block), "--as", "acquire-3", "--dir", dir); code != Done {
		t.Fatalf("placing from the directory: %d %s", code, out)
	}
	p := placements(t, dir)
	read, _ := os.ReadFile(block)
	if len(p) != 2 || p[0]["block"] != "acquisition-head" || p[0]["placed_as"] != "acquire-2" || p[0]["source"] != block ||
		p[0]["digest"] != fmt.Sprintf("sha256:%x", sha256.Sum256(read)) || !strings.Contains(raw(t, filepath.Join(dir, Blocks)), "placed_at: 2026-10-15T10:04:11Z") ||
		p[1]["placed_as"] != "acquire-3" || p[1]["source"] != filepath.Dir(block) {
		t.Errorf("blocks.yaml holds %v", p)
	}
	data, err := archive(dir)
	if err != nil {
		t.Fatal(err)
	}
	z, _ := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	for _, f := range z.File {
		if f.Name == Blocks {
			t.Error("the bundle holds blocks.yaml")
		}
	}
}

// std: yoke:blocks.02
func TestWhatPlacementRefusesLeavesTheProjectAsItWas(t *testing.T) {
	dir, block := blocked(t)
	if err := AddBlock(dir, block, "acquire-2", placedAt); err != nil {
		t.Fatal(err)
	}
	must(t, "add", "channel", "front", "--transport", "local", "--clients", "single", "--dir", dir)
	shelf := t.TempDir()
	variant := func(name, from, to string) string {
		p := filepath.Join(shelf, name+".yaml")
		os.WriteFile(p, []byte(strings.Replace(headBlock, from, to, 1)), 0o644)
		return p
	}
	older := variant("older", "model: 1", "model: 0")
	shared := variant("shared", "parameters:\n  instrument:", "parameters:\n  shared:\n    site: { type: string, required: false }\n  instrument:")
	refusedKind := variant("kind", "kind: interface", "kind: daemon")
	two := filepath.Join(shelf, "two")
	os.MkdirAll(two, 0o755)
	os.WriteFile(filepath.Join(two, "a.yaml"), []byte(headBlock), 0o644)
	os.WriteFile(filepath.Join(two, "b.yaml"), []byte(headBlock), 0o644)
	missing := filepath.Join(shelf, "nowhere.yaml")

	file, record := filepath.Join(dir, Descriptor), filepath.Join(dir, Blocks)
	for _, c := range []struct {
		source, as, names string
	}{
		{block, "acquire-2", "acquire-2"},
		{block, "front", "front"},
		{older, "acquire-5", "model"},
		{shared, "acquire-5", "shared"},
		{refusedKind, "acquire-5", "field.value"},
		{two, "acquire-5", "b.yaml"},
		{missing, "acquire-5", missing},
	} {
		before, _ := os.ReadFile(file)
		beforeRecord, _ := os.ReadFile(record)
		code, out := run("block", "add", c.source, "--as", c.as, "--dir", dir)
		after, _ := os.ReadFile(file)
		afterRecord, _ := os.ReadFile(record)
		if code != Refused || !strings.Contains(out, c.names) || !bytes.Equal(before, after) || !bytes.Equal(beforeRecord, afterRecord) {
			t.Errorf("placing %s as %s exited %d (%s); descriptor unchanged %v, record unchanged %v", c.source, c.as, code, out,
				bytes.Equal(before, after), bytes.Equal(beforeRecord, afterRecord))
		}
	}
}

// std: yoke:blocks.03
func TestSavingAUnitWritesABlockThatPlacesTheSameDeclarations(t *testing.T) {
	dir, block := blocked(t)
	if err := AddBlock(dir, block, "acquire-2", placedAt); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, Descriptor)
	before, _ := os.ReadFile(file)
	beforeRecord, _ := os.ReadFile(filepath.Join(dir, Blocks))
	saved := filepath.Join(t.TempDir(), "saved.yaml")
	must(t, "block", "save", "acquire-2", saved, "--dir", dir)
	after, _ := os.ReadFile(file)
	afterRecord, _ := os.ReadFile(filepath.Join(dir, Blocks))
	if !bytes.Equal(before, after) || !bytes.Equal(beforeRecord, afterRecord) {
		t.Error("saving a block changed the project")
	}
	if written(t, saved, "block") != "acquire-2" || written(t, saved, "model") != 1 || written(t, saved, "unit.kind") != "interface" ||
		written(t, saved, "parameters.instrument.type") != "string" || written(t, saved, "channels.acquire-2.unit") != "acquire-2" {
		data, _ := os.ReadFile(saved)
		t.Fatalf("the saved block is:\n%s", data)
	}
	if code, out := run("block", "save", "acquire-2", saved, "--dir", dir); code != Refused || !strings.Contains(out, saved) {
		t.Errorf("saving over a file: %d %s", code, out)
	}
	must(t, "block", "add", saved, "--as", "acquire-4", "--dir", dir)
	same := func(path string) {
		t.Helper()
		a, b := written(t, file, strings.ReplaceAll(path, "@", "acquire-2")), written(t, file, strings.ReplaceAll(path, "@", "acquire-4"))
		if fmt.Sprint(a) != fmt.Sprint(b) {
			t.Errorf("%s: %v and %v", path, a, b)
		}
	}
	for _, p := range []string{"units.@.kind", "units.@.exec", "units.@.digest", "units.@.args", "parameters.@", "channels.@.transport", "channels.@.clients"} {
		same(p)
	}
	if written(t, file, "channels.acquire-4.unit") != "acquire-4" {
		t.Errorf("the channel placed serves %v", written(t, file, "channels.acquire-4.unit"))
	}
}
