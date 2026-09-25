package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/config"
)

// environment returns a lookup over the given pairs, and nothing else.
func environment(pairs ...string) func(string) string {
	values := map[string]string{}
	for i := 0; i+1 < len(pairs); i += 2 {
		values[pairs[i]] = pairs[i+1]
	}
	return func(name string) string { return values[name] }
}

// std: yoke:the-core-process.01
func TestAKeyLeftOutTakesItsDefault(t *testing.T) {
	empty, err := config.Parse([]byte(""), environment())
	if err != nil {
		t.Fatalf("an empty core.yaml is refused: %v", err)
	}
	want := config.Config{
		StateDir:   "/var/lib/yoke",
		RuntimeDir: "/run/yoke",
		Plugins: config.Plugins{
			Manifests:    "/etc/yoke/plugins.d",
			Executables:  "/usr/lib/yoke/plugins",
			ScanInterval: 30 * time.Second,
		},
		Engine: "unix:///run/podman/podman.sock",
		Log:    config.Log{Level: "info"},
	}
	if empty != want {
		t.Errorf("the defaults are %+v, want %+v", empty, want)
	}

	one, err := config.Parse([]byte("state_dir: /srv/yoke\n"), environment())
	if err != nil {
		t.Fatalf("a core.yaml setting state_dir is refused: %v", err)
	}
	want.StateDir = "/srv/yoke"
	if one != want {
		t.Errorf("with state_dir set, got %+v, want %+v", one, want)
	}
}

// std: yoke:the-core-process.02
func TestAnUnknownKeyIsRefusedByName(t *testing.T) {
	for _, document := range []struct{ yaml, key string }{
		{"state_dir: /srv/yoke\nendpoint: /run/x.sock\n", "endpoint"},
		{"plugins:\n  manifests: /etc/yoke/plugins.d\n  autostart: true\n", "autostart"},
	} {
		_, err := config.Parse([]byte(document.yaml), environment())
		if err == nil {
			t.Errorf("a core.yaml with %q is read", document.key)
			continue
		}
		if !strings.Contains(err.Error(), document.key) {
			t.Errorf("the refusal does not name %q: %v", document.key, err)
		}
	}
}

// std: yoke:the-core-process.03
func TestAValueIsTypedByItsKey(t *testing.T) {
	read, err := config.Parse([]byte("state_dir: no\n"), environment())
	if err != nil {
		t.Fatalf("state_dir: no is refused: %v", err)
	}
	if read.StateDir != "no" {
		t.Errorf("state_dir: no is read as %q, not the string no", read.StateDir)
	}
	for _, value := range []string{"30", "soon"} {
		_, err := config.Parse([]byte("plugins:\n  scan_interval: "+value+"\n"), environment())
		if err == nil {
			t.Errorf("plugins.scan_interval: %s is read", value)
			continue
		}
		if !strings.Contains(err.Error(), "plugins.scan_interval") {
			t.Errorf("the refusal of %s does not name plugins.scan_interval: %v", value, err)
		}
	}
}

// std: yoke:the-core-process.04
func TestTheEnvironmentOverridesOneValue(t *testing.T) {
	file := []byte("state_dir: /srv/yoke\nplugins:\n  scan_interval: 30s\n")
	read, err := config.Parse(file, environment("YOKE_STATE_DIR", "/data/yoke", "YOKE_PLUGINS_SCAN_INTERVAL", "5s"))
	if err != nil {
		t.Fatalf("the overrides are refused: %v", err)
	}
	if read.StateDir != "/data/yoke" || read.Plugins.ScanInterval != 5*time.Second {
		t.Errorf("the environment did not override: state_dir %q, scan_interval %v", read.StateDir, read.Plugins.ScanInterval)
	}
	_, err = config.Parse(file, environment("YOKE_PLUGINS_SCAN_INTERVAL", "soon"))
	if err == nil {
		t.Fatal("YOKE_PLUGINS_SCAN_INTERVAL=soon is read")
	}
	if !strings.Contains(err.Error(), "YOKE_PLUGINS_SCAN_INTERVAL") {
		t.Errorf("the refusal does not name the variable: %v", err)
	}
}

// std: yoke:the-core-process.05
func TestYokeConfigNamesTheFileRead(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "core.yaml")
	if err := os.WriteFile(path, []byte("runtime_dir: "+directory+"/run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := config.Load(environment("YOKE_CONFIG", path))
	if err != nil {
		t.Fatalf("the file YOKE_CONFIG names is refused: %v", err)
	}
	if read.RuntimeDir != directory+"/run" {
		t.Errorf("runtime_dir is %q, not the named file's", read.RuntimeDir)
	}
	missing := filepath.Join(directory, "absent.yaml")
	_, err = config.Load(environment("YOKE_CONFIG", missing))
	if err == nil {
		t.Fatal("a named file that does not exist is replaced by defaults")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the refusal does not name the path: %v", err)
	}
}

// std: yoke:the-core-process.06
func TestTheCompositionIsChosenByTheChain(t *testing.T) {
	for _, step := range []struct{ flag, variable, want string }{
		{"", "", "/etc/yoke/deployment.yaml"},
		{"", "/etc/yoke/deployments/bench.yaml", "/etc/yoke/deployments/bench.yaml"},
		{"/etc/yoke/deployments/line.yaml", "/etc/yoke/deployments/bench.yaml", "/etc/yoke/deployments/line.yaml"},
	} {
		got := config.Composition(step.flag, environment("YOKE_COMPOSITION", step.variable))
		if got != step.want {
			t.Errorf("flag %q and variable %q choose %q, want %q", step.flag, step.variable, got, step.want)
		}
	}
}

// std: yoke:the-core-process.14
func TestTheLogLevelIsOneOfFour(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		read, err := config.Parse([]byte("log:\n  level: "+level+"\n"), environment())
		if err != nil {
			t.Errorf("log.level %s is refused: %v", level, err)
			continue
		}
		if read.Log.Level != level {
			t.Errorf("log.level %s is read as %q", level, read.Log.Level)
		}
	}
	_, err := config.Parse([]byte("log:\n  level: verbose\n"), environment())
	if err == nil {
		t.Fatal("log.level verbose is read")
	}
	if !strings.Contains(err.Error(), "log.level") {
		t.Errorf("the refusal does not name log.level: %v", err)
	}
}
