package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// environment is a getenv over the given variables and nothing else.
func environment(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func conf(t *testing.T, body string) map[string]string {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "yoke"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "yoke", "manager.conf"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"XDG_CONFIG_HOME": home, "XDG_RUNTIME_DIR": "/run/user/1000"}
}

// std: yoke:the-manager-configuration.01
func TestTheManagersFileIsRead(t *testing.T) {
	c, err := LoadConfig(environment(conf(t, "install_root: /srv/yoke/apps\nengine: unix:///run/docker.sock\ncore: /opt/yoke/bin/yoke-core\nretention:\n  bytes: 20MB\n  age: 7d\n")))
	if err != nil {
		t.Fatal(err)
	}
	if c.InstallRoot != "/srv/yoke/apps" || c.Engine != "unix:///run/docker.sock" || c.Core != "/opt/yoke/bin/yoke-core" {
		t.Errorf("read as %+v", c)
	}
	if c.Retention["bytes"] != "20MB" || c.Retention["age"] != "7d" || len(c.Retention) != 2 {
		t.Errorf("the ceiling is %v", c.Retention)
	}
}

// std: yoke:the-manager-configuration.02
func TestAMissingFileIsNotAnError(t *testing.T) {
	c, err := LoadConfig(environment(map[string]string{"XDG_CONFIG_HOME": t.TempDir(), "XDG_RUNTIME_DIR": "/run/user/1000"}))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{InstallRoot: "/var/lib/yoke/apps", Engine: "unix:///run/user/1000/podman/podman.sock", Core: "yoke-core"}
	if c.InstallRoot != want.InstallRoot || c.Engine != want.Engine || c.Core != want.Core || len(c.Retention) != 0 {
		t.Errorf("the defaults are %+v, want %+v", c, want)
	}
}

// std: yoke:the-manager-configuration.03
func TestAnUnknownKeyInTheManagersFileIsRefused(t *testing.T) {
	for body, names := range map[string]string{
		"install_root: /srv/apps\nunits_env: {}\n": "units_env",
		"retention:\n  files: 10\n":                "files",
	} {
		_, err := LoadConfig(environment(conf(t, body)))
		if err == nil || !strings.Contains(err.Error(), names) {
			t.Errorf("%q: %v", body, err)
		}
	}
}

// std: yoke:the-manager-configuration.04
func TestEachValueOfTheManagersFileIsWhatItSays(t *testing.T) {
	for body, names := range map[string]string{
		"install_root: apps\n":                "apps",
		"retention:\n  bytes: 20 megabytes\n": "20 megabytes",
		"retention:\n  age: a week\n":         "a week",
		"retention:\n  entries: many\n":       "many",
	} {
		_, err := LoadConfig(environment(conf(t, body)))
		if err == nil || !strings.Contains(err.Error(), names) {
			t.Errorf("%q: %v", body, err)
		}
	}
}

// std: yoke:the-manager-configuration.05
func TestTheFileIsFoundUnderTheConfigurationHome(t *testing.T) {
	vars := conf(t, "core: /from/xdg\n")
	c, err := LoadConfig(environment(vars))
	if err != nil || c.Core != "/from/xdg" {
		t.Errorf("under XDG_CONFIG_HOME: %+v %v", c, err)
	}
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".config", "yoke"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "yoke", "manager.conf"), []byte("core: /from/home\n"), 0o644)
	c, err = LoadConfig(environment(map[string]string{"HOME": home}))
	if err != nil || c.Core != "/from/home" {
		t.Errorf("under HOME: %+v %v", c, err)
	}
}
