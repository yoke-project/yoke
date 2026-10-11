package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// std: yoke:the-manager-command-line.05
func TestYokeInstanceCreatesListsShowsAndRePoints(t *testing.T) {
	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	for _, ver := range []string{"2.1.0", "2.2.0"} {
		dir := filepath.Join(apps, "com.yoke.station", ver)
		os.MkdirAll(filepath.Join(dir, "bundle"), 0o775)
		os.WriteFile(filepath.Join(dir, "bundle", "yoke.yaml"), []byte("model: 1\nid: com.yoke.station\nversion: "+ver+
			"\narch: aarch64\ncore: \">=0.1\"\nparameters:\n  setup:\n    retries: { type: integer, required: false, default: 3, label: R }\n"+
			"units:\n  setup: { kind: oneshot, exec: components/setup, digest: \"sha256:"+strings.Repeat("ab", 32)+"\" }\n"), 0o644)
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
