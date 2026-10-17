package manager

import (
	"bytes"
	"errors"
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
	if code != 2 || !strings.Contains(out, "not running") {
		t.Errorf("an instance not running: %d %s", code, out)
	}
	code, out = yoke(map[string]string{"XDG_RUNTIME_DIR": run}, "frobnicate")
	if code != 1 || !strings.Contains(out, "frobnicate") || !strings.Contains(out, "plugin") {
		t.Errorf("no such verb: %d %s", code, out)
	}
}

// std: yoke:the-manager-command-line.02
func TestTheInstanceIsASelector(t *testing.T) {
	vars := map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000", "XDG_STATE_HOME": t.TempDir()}
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

// std: yoke:the-manager-command-line.13
func TestWithNoSelectorTheAccountsOneInstanceIsAddressed(t *testing.T) {
	state := t.TempDir()
	core := filepath.Join(t.TempDir(), "core.yaml")
	os.WriteFile(core, []byte("runtime_dir: /run/station\n"), 0o644)
	vars := map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000", "XDG_STATE_HOME": state, "YOKE_CONFIG": core}
	instances := filepath.Join(state, "yoke", "instances")
	os.MkdirAll(filepath.Join(instances, "bench-a"), 0o700)
	if got, err := operatorAddress("", false, environment(vars)); err != nil || got != "/run/user/1000/yoke/bench-a/operator.sock" {
		t.Errorf("one instance: %q %v", got, err)
	}
	os.MkdirAll(filepath.Join(instances, "bench-b"), 0o700)
	_, err := operatorAddress("", false, environment(vars))
	if !errors.As(err, new(refusal)) || !strings.Contains(err.Error(), "bench-a") || !strings.Contains(err.Error(), "bench-b") ||
		!strings.Contains(err.Error(), "--instance") {
		t.Errorf("two instances: %v", err)
	}
	os.RemoveAll(instances)
	if got, err := operatorAddress("", false, environment(vars)); err != nil || got != "/run/station/operator.sock" {
		t.Errorf("no instance: %q %v", got, err)
	}
}

// std: yoke:the-manager-command-line.16
func TestHelpAfterAVerbIsItsUsage(t *testing.T) {
	vars := map[string]string{"XDG_RUNTIME_DIR": t.TempDir(), "XDG_STATE_HOME": t.TempDir()}
	for _, c := range []struct {
		args  []string
		usage string
	}{
		{[]string{"log", "query", "--help"}, "yoke log query"},
		{[]string{"plugin", "grant", "--help"}, "yoke plugin grant <plugin> <capability>"},
		{[]string{"shell", "--help"}, "yoke shell [--instance <name>]"},
	} {
		code, out := yoke(vars, c.args...)
		if code != Done || !strings.Contains(out, c.usage) || strings.Contains(out, "help requested") {
			t.Errorf("%v exited %d:\n%s", c.args, code, out)
		}
	}
}

// std: yoke:the-manager-command-line.17
func TestTheWrongSubjectsAreAnsweredWithTheRightOnes(t *testing.T) {
	vars := map[string]string{"XDG_RUNTIME_DIR": t.TempDir(), "XDG_STATE_HOME": t.TempDir()}
	for _, c := range []struct {
		args  []string
		names string
	}{
		{[]string{"plugin", "grant"}, "yoke plugin grant <plugin> <capability>"},
		{[]string{"unit", "restart", "a", "b"}, "yoke unit restart <unit>"},
	} {
		if code, out := yoke(vars, c.args...); code != Refused || !strings.Contains(out, c.names) {
			t.Errorf("%v exited %d:\n%s", c.args, code, out)
		}
	}
}
