package manager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/compose"
	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"
)

// installedStation is an install root holding com.yoke.station 2.1.0, as an installation leaves it.
func installedStation(t *testing.T) string {
	t.Helper()
	apps := filepath.Join(t.TempDir(), "apps")
	dir := filepath.Join(apps, "com.yoke.station", "2.1.0")
	os.MkdirAll(filepath.Join(dir, "bundle", "components"), 0o775)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	os.WriteFile(filepath.Join(dir, "bundle", "components", "setup"), []byte(setupProgram), 0o755)
	os.WriteFile(filepath.Join(dir, "bundle", "yoke.yaml"), []byte("model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: "+compose.HostArch()+
		"\ncore: \">=0.1\"\nunits:\n  setup: { kind: oneshot, exec: components/setup, digest: \""+setupDigest+"\" }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "install.yaml"), []byte("state: complete\nsealed: false\nshared: true\n"), 0o664)
	return apps
}

// shortRuntime is a runtime directory short enough for the socket paths an instance makes beneath it.
func shortRuntime(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "yr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// std: yoke:the-manager-command-line.18
func TestEachGroupSelectsItsWholeRegime(t *testing.T) {
	vars, _, clean := serviceForm(t)
	vars["YOKE_COMPOSITION"] = clean
	account := conf(t, "heartbeat: 5s\n")
	vars["XDG_CONFIG_HOME"] = account["XDG_CONFIG_HOME"]
	if code, out := yoke(vars, "app", "list"); code == Done || !strings.Contains(out, "heartbeat") {
		t.Errorf("a manager configuration carrying a key the Core reads was not refused, naming it (%d):\n%s", code, out)
	}
	if code, out := yoke(vars, "service", "check"); code != Done || !strings.Contains(out, "passes") {
		t.Errorf("the service regime read the account's configuration (%d):\n%s", code, out)
	}
}

// std: yoke:the-manager-command-line.19
func TestNoVerbPerformsTheFourStepsOfAnUpdate(t *testing.T) {
	vars := map[string]string{"XDG_RUNTIME_DIR": t.TempDir()}
	for _, args := range [][]string{{"app", "update"}, {"instance", "update"}, {"instance", "upgrade"}} {
		code, out := yoke(vars, args...)
		if code != Refused || !strings.Contains(out, args[1]) {
			t.Errorf("yoke %s was not refused as a verb that does not exist (%d):\n%s", strings.Join(args, " "), code, out)
		}
	}
	_, usage := yoke(vars, "help")
	for _, step := range []string{"install", "copy", "set-version", "restart"} {
		if !strings.Contains(usage, step) {
			t.Errorf("the usage does not offer %s as a step of its own:\n%s", step, usage)
		}
	}
	for _, convenience := range []string{"update", "upgrade"} {
		if strings.Contains(usage, convenience) {
			t.Errorf("the usage offers %q:\n%s", convenience, usage)
		}
	}
}

// std: yoke:the-manager-command-line.20
func TestAnAccountListsItsOwnInstancesAndEveryAccountSeesWhatIsInstalled(t *testing.T) {
	apps := installedStation(t)
	shared := conf(t, "install_root: "+apps+"\n")
	accounts := map[string]map[string]string{}
	for _, name := range []string{"bench-a", "bench-b"} {
		vars := map[string]string{"XDG_CONFIG_HOME": shared["XDG_CONFIG_HOME"], "XDG_STATE_HOME": t.TempDir(), "XDG_RUNTIME_DIR": shortRuntime(t)}
		if code, out := yoke(vars, "instance", "create", "com.yoke.station", "--name", name); code != Done {
			t.Fatalf("creating %s exited %d:\n%s", name, code, out)
		}
		accounts[name] = vars
	}
	for own, vars := range accounts {
		_, listed := yoke(vars, "instance", "list")
		other := map[string]string{"bench-a": "bench-b", "bench-b": "bench-a"}[own]
		if !strings.Contains(listed, own) || strings.Contains(listed, other) {
			t.Errorf("the account of %s lists:\n%s", own, listed)
		}
		if _, installed := yoke(vars, "app", "list"); !strings.Contains(installed, "com.yoke.station") || !strings.Contains(installed, "2.1.0") {
			t.Errorf("the account of %s does not see what is installed:\n%s", own, installed)
		}
	}
}

// std: yoke:the-manager-command-line.21
func TestTheLifecycleAndTheContentsHaveSeparateInterlocutors(t *testing.T) {
	apps := installedStation(t)
	vars := conf(t, "install_root: "+apps+"\n")
	vars["XDG_STATE_HOME"], vars["XDG_RUNTIME_DIR"] = t.TempDir(), shortRuntime(t)
	if code, out := yoke(vars, "instance", "create", "com.yoke.station", "--name", "bench-a"); code != Done {
		t.Fatalf("creating exited %d:\n%s", code, out)
	}
	record := filepath.Join(vars["XDG_STATE_HOME"], "yoke", "instances", "bench-a", "instance.yaml")
	before, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if code, out := yoke(vars, "unit", "restart", "setup"); code == Done || !strings.Contains(out, "not running") {
		t.Errorf("a client verb on an instance not running (%d):\n%s", code, out)
	}
	if after, _ := os.ReadFile(record); string(after) != string(before) {
		t.Errorf("a client verb changed the record:\n%s", after)
	}
	fields := (&administrativev1.Request{}).ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		name := string(fields.Get(i).Name())
		if strings.Contains(name, "instance") || strings.HasPrefix(name, "core") || name == "stop" || name == "shutdown" {
			t.Errorf("the administrative contract carries %s, which acts on the instance's lifecycle", name)
		}
	}
}
