package manager

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func yoke(vars map[string]string, args ...string) (int, string) {
	var out bytes.Buffer
	code := Run(args, environment(vars), &out, &out)
	return code, out.String()
}

// std: yoke:the-manager-command-line.01
func TestAnExitStatusTellsARefusalFromAFault(t *testing.T) {
	run := t.TempDir()
	if code, out := yoke(map[string]string{"XDG_RUNTIME_DIR": run}, "help"); code != 0 {
		t.Errorf("help: %d %s", code, out)
	}
	code, out := yoke(map[string]string{"XDG_RUNTIME_DIR": run}, "core", "status", "--instance", "bench")
	if code != 1 || !strings.Contains(out, "not running") {
		t.Errorf("an instance not running: %d %s", code, out)
	}
	code, out = yoke(map[string]string{"XDG_RUNTIME_DIR": run}, "frobnicate")
	if code != 2 || !strings.Contains(out, "frobnicate") || !strings.Contains(out, "plugin") {
		t.Errorf("no such verb: %d %s", code, out)
	}
}

// std: yoke:the-manager-command-line.02
func TestTheInstanceIsASelector(t *testing.T) {
	vars := map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}
	got, err := operatorAddress("bench", true, environment(vars))
	if err != nil || got != "/run/user/1000/yoke/bench/operator.sock" {
		t.Errorf("bench: %q %v", got, err)
	}
	for _, name := range []string{"../bench", ""} {
		if _, err := operatorAddress(name, true, environment(vars)); err == nil {
			t.Errorf("%q chose an address", name)
		}
	}
	core := filepath.Join(t.TempDir(), "core.yaml")
	os.WriteFile(core, []byte("runtime_dir: /run/station\n"), 0o644)
	vars["YOKE_CONFIG"] = core
	got, err = operatorAddress("", false, environment(vars))
	if err != nil || got != "/run/station/operator.sock" {
		t.Errorf("no selector: %q %v", got, err)
	}
}
