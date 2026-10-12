package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

// std: yoke:the-manager-command-line.05
func TestYokeInstanceCreatesListsShowsAndRePoints(t *testing.T) {
	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	for _, ver := range []string{"2.1.0", "2.2.0"} {
		dir := filepath.Join(apps, "com.yoke.station", ver)
		os.MkdirAll(filepath.Join(dir, "bundle", "components"), 0o775)
		os.WriteFile(filepath.Join(dir, "bundle", "components", "setup"), []byte(setupProgram), 0o755)
		os.WriteFile(filepath.Join(dir, "bundle", "yoke.yaml"), []byte("model: 1\nid: com.yoke.station\nversion: "+ver+
			"\narch: "+compose.HostArch()+"\ncore: \">=0.1\"\nparameters:\n  setup:\n    retries: { type: integer, required: false, default: 3, label: R }\n"+
			"units:\n  setup: { kind: oneshot, exec: components/setup, digest: \""+setupDigest+"\" }\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "install.yaml"), []byte("state: complete\nsealed: false\nshared: true\n"), 0o664)
	}
	vars := conf(t, "install_root: "+apps+"\n")
	vars["XDG_STATE_HOME"] = t.TempDir()
	params := filepath.Join(t.TempDir(), "values.toml")
	os.WriteFile(params, []byte("[setup]\nretries = 4\n"), 0o644)
	expect := func(want []string, args ...string) {
		t.Helper()
		code, out := yoke(vars, args...)
		if code != Done {
			t.Fatalf("yoke %s exited %d:\n%s", strings.Join(args, " "), code, out)
		}
		for _, w := range want {
			if !strings.Contains(squash(out), w) {
				t.Errorf("yoke %s does not say %q:\n%s", strings.Join(args, " "), w, out)
			}
		}
	}
	expect([]string{"bench-a", "2.2.0"}, "instance", "create", "com.yoke.station", "--name", "bench-a", "--params", params)
	expect([]string{"bench-a com.yoke.station 2.2.0 stopped"}, "instance", "list")
	expect([]string{"retries: \"4\""}, "instance", "show", "bench-a")
	expect(nil, "instance", "set-param", "bench-a", "setup.retries", "5")
	expect(nil, "instance", "set-version", "bench-a", "2.1.0")
	expect([]string{"version: 2.1.0", "retries: \"5\""}, "instance", "show", "bench-a")
	if code, out := yoke(vars, "instance", "create", "com.yoke.other"); code != Refused {
		t.Errorf("an application not installed exited %d:\n%s", code, out)
	}
}

// std: yoke:the-manager-command-line.06
func TestYokeInstanceCopiesRestoresAndShowsWhatItOccupies(t *testing.T) {
	apps := filepath.Join(t.TempDir(), "apps")
	dir := filepath.Join(apps, "com.yoke.station", "2.1.0")
	os.MkdirAll(filepath.Join(dir, "bundle", "components"), 0o775)
	os.WriteFile(filepath.Join(dir, "bundle", "components", "setup"), []byte(setupProgram), 0o755)
	os.WriteFile(filepath.Join(dir, "bundle", "yoke.yaml"), []byte("model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: "+compose.HostArch()+"\ncore: \">=0.1\"\n"+
		"units:\n  setup: { kind: oneshot, exec: components/setup, digest: \""+setupDigest+"\" }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "install.yaml"), []byte("state: complete\nsealed: false\nshared: true\n"), 0o664)
	vars := conf(t, "install_root: "+apps+"\n")
	vars["XDG_STATE_HOME"] = t.TempDir()
	vars["XDG_RUNTIME_DIR"] = t.TempDir()
	for _, n := range []string{"bench-a", "bench-b"} {
		if code, out := yoke(vars, "instance", "create", "com.yoke.station", "--name", n); code != Done {
			t.Fatalf("create %s: %s", n, out)
		}
	}
	logs := filepath.Join(vars["XDG_STATE_HOME"], "yoke", "instances", "bench-a", "state", "logs.db")
	os.MkdirAll(filepath.Dir(logs), 0o700)
	os.WriteFile(logs, []byte("12345"), 0o600)
	if code, out := yoke(vars, "instance", "show", "bench-a"); code != Done || !strings.Contains(squash(out), "logs 5") {
		t.Errorf("show exited %d:\n%s", code, out)
	}
	copied := filepath.Join(t.TempDir(), "copy")
	if code, out := yoke(vars, "instance", "copy", "bench-a", copied); code != Done || !strings.Contains(out, copied) {
		t.Errorf("copy exited %d:\n%s", code, out)
	}
	if code, out := yoke(vars, "instance", "restore", copied, "bench-b"); code != Done {
		t.Errorf("restore exited %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(vars["XDG_STATE_HOME"], "yoke", "instances", "bench-b", "state", "logs.db")); err != nil {
		t.Errorf("bench-b holds no log store after the restore: %v", err)
	}
}

// std: yoke:the-moments.04
func TestAnInstanceOfAChangedComponentIsRefusedAtCreating(t *testing.T) {
	vars := account(t, "")
	bundle := filepath.Join(installRoot(t, vars), "com.yoke.station", "2.1.0", "bundle")
	os.WriteFile(filepath.Join(bundle, "components", "probe"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	code, out := yoke(vars, "instance", "create", "com.yoke.station", "--name", "bench-a")
	if code != Refused || !strings.Contains(out, "component.digest") {
		t.Errorf("creating over a changed component exited %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(instancerecord.Root(environment(vars)), "bench-a")); err == nil {
		t.Error("a refused creation wrote a record")
	}
}
