// Package packaging holds the files a distribution installs beside the programs. Its tests read them as
// the parties that consume them do.
package packaging

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/core/config"
)

// lines are a file's lines that are neither empty nor comments, each split into its fields.
func lines(t *testing.T, name string) [][]string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, strings.Fields(l))
		}
	}
	return out
}

// std: yoke:the-packaging-inputs.01
func TestTheDefaultConfigurationStatesEveryKeyAtItsDefault(t *testing.T) {
	shipped, err := config.Load(func(k string) string {
		if k == "YOKE_CONFIG" {
			return "core.yaml"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if shipped != config.Defaults() {
		t.Errorf("the shipped configuration is %+v, an empty one %+v", shipped, config.Defaults())
	}
	data, err := os.ReadFile("core.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	stated := map[string]bool{}
	for k, v := range doc {
		if inner, ok := v.(map[string]any); ok {
			for k2 := range inner {
				stated[k+"."+k2] = true
			}
			continue
		}
		stated[k] = true
	}
	for _, k := range []string{"state_dir", "runtime_dir", "plugins.manifests", "plugins.executables", "plugins.scan_interval", "engine", "log.level"} {
		if !stated[k] {
			t.Errorf("%s is not stated, among %v", k, stated)
		}
	}
}

// std: yoke:the-packaging-inputs.02
func TestTheLayoutCreatesTheDirectoriesWithTheirOwnersAndModes(t *testing.T) {
	want := map[string]string{
		"/etc/yoke":                "0755 root root",
		"/etc/yoke/plugins.d":      "0755 root root",
		"/etc/yoke/deployments":    "0755 root root",
		"/usr/lib/yoke":            "0755 root root",
		"/usr/lib/yoke/plugins":    "0755 root root",
		"/usr/lib/yoke/interfaces": "0755 root root",
		"/var/lib/yoke":            "0755 root root",
		"/var/lib/yoke/apps":       "2775 root yoke-apps",
	}
	got := map[string]string{}
	for _, l := range lines(t, "yoke.tmpfiles") {
		if len(l) < 5 || l[0] != "d" {
			t.Errorf("a line that does not create a directory: %v", l)
			continue
		}
		got[l[1]] = strings.Join(l[2:5], " ")
	}
	for path, w := range want {
		if got[path] != w {
			t.Errorf("%s is %q, where it should be %q", path, got[path], w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d directories, where %d", len(got), len(want))
	}
}

// std: yoke:the-packaging-inputs.03
func TestTheInstallRootsGroupIsCreatedWithIt(t *testing.T) {
	got := lines(t, "yoke.sysusers")
	if len(got) != 1 || len(got[0]) < 3 || got[0][0] != "g" || got[0][1] != "yoke-apps" || got[0][2] != "-" {
		t.Errorf("the sysusers lines are %v", got)
	}
}

// unit is a systemd unit file's keys by section, each key with the values it is given, in order.
func unit(t *testing.T, name string) map[string]map[string][]string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string][]string{}
	section := ""
	for _, l := range strings.Split(string(data), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "" || strings.HasPrefix(l, "#"):
		case strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]"):
			section = l[1 : len(l)-1]
			out[section] = map[string][]string{}
		case section != "" && strings.Contains(l, "="):
			k, v, _ := strings.Cut(l, "=")
			out[section][k] = append(out[section][k], v)
		default:
			t.Errorf("a line outside any section or not a key: %q", l)
		}
	}
	return out
}

// std: yoke:the-packaging-inputs.04
func TestTheGuardiansInitUnitRunsItForItsAccountAtBoot(t *testing.T) {
	u := unit(t, "yoke-guardian.service")
	want := map[string]map[string]string{
		"Service": {"ExecStart": "/usr/bin/yoke-guardian --boot", "Restart": "on-failure"},
		"Install": {"WantedBy": "default.target"},
	}
	for section, keys := range want {
		for k, v := range keys {
			if got := u[section][k]; len(got) != 1 || got[0] != v {
				t.Errorf("[%s] %s is %q, where it should be %q", section, k, got, v)
			}
		}
	}
	for _, k := range []string{"User", "Group", "DynamicUser"} {
		if v, found := u["Service"][k]; found {
			t.Errorf("the user unit names an account of its own: %s=%v", k, v)
		}
	}
}
