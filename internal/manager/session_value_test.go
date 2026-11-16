package manager

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/compose"
)

// sessionValue is an account whose install root holds com.yoke.desktop 1.0.0: a parameter
// probe.compositor, and a oneshot `probe` that records, line by line into the directory witness, its
// arguments and then its environment. The programs are built from this tree.
func sessionValue(t *testing.T) (map[string]string, string) {
	t.Helper()
	bin := t.TempDir()
	for _, program := range []string{"yoke-core", "yoke-guardian"} {
		if said, err := exec.Command("go", "build", "-o", filepath.Join(bin, program), "github.com/yoke-project/yoke/cmd/"+program).CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
	}
	witness := t.TempDir()
	probe := "#!/bin/sh\n{ echo \"args $*\"; env; } > " + witness + "/run.$$\nmv " + witness + "/run.$$ " + witness + "/last\n"
	apps := filepath.Join(t.TempDir(), "apps")
	bundle := filepath.Join(apps, "com.yoke.desktop", "1.0.0", "bundle")
	os.MkdirAll(filepath.Join(bundle, "components"), 0o755)
	os.WriteFile(filepath.Join(bundle, "components", "probe"), []byte(probe), 0o755)
	os.WriteFile(filepath.Join(bundle, "yoke.yaml"), []byte(fmt.Sprintf(`model: 1
id: com.yoke.desktop
version: 1.0.0
arch: %s
core: ">=0.0.0-0"
parameters:
  probe:
    compositor: { type: string, required: false, default: "", label: The compositor's instance }
policy: { stop_window: 5s }
units:
  probe: { kind: oneshot, exec: components/probe, digest: "sha256:%x", args: [ "${compositor}" ] }
`, compose.HostArch(), sha256.Sum256([]byte(probe)))), 0o644)
	os.WriteFile(filepath.Join(apps, "com.yoke.desktop", "1.0.0", "install.yaml"), []byte("state: complete\nsealed: false\nshared: true\n"), 0o664)
	vars := conf(t, "install_root: "+apps+"\ncore: "+filepath.Join(bin, "yoke-core")+"\n")
	run, err := os.MkdirTemp("", "ym")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	vars["XDG_RUNTIME_DIR"], vars["XDG_STATE_HOME"], vars["HOME"] = run, t.TempDir(), t.TempDir()
	vars["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Cleanup(func() {
		if pid := holder(vars, "guardian.lock"); pid != 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	})
	return vars, witness
}

// recordedRun waits for the probe to have recorded a run whose arguments are want, and returns it.
func recordedRun(t *testing.T, witness, want string) string {
	t.Helper()
	var got string
	eventually(t, "the probe to be handed "+want, 15*time.Second, func() bool {
		data, _ := os.ReadFile(filepath.Join(witness, "last"))
		got = string(data)
		return strings.HasPrefix(got, "args "+want+"\n")
	})
	return got
}

// startedWith creates the instance `desktop`, sets probe.compositor from a session holding
// HYPRLAND_INSTANCE_SIGNATURE, starts it, and returns what the probe recorded.
func startedWith(t *testing.T, vars map[string]string, witness string) string {
	t.Helper()
	expectDone(t, vars, "instance", "create", "com.yoke.desktop", "--name", "desktop")
	// The graphical session: it knows the compositor's instance, and sets it on the record.
	vars["HYPRLAND_INSTANCE_SIGNATURE"] = "abc123_1794812472"
	expectDone(t, vars, "instance", "set-param", "desktop", "probe.compositor", vars["HYPRLAND_INSTANCE_SIGNATURE"])
	expectDone(t, vars, "instance", "start", "desktop")
	return recordedRun(t, witness, "abc123_1794812472")
}

// std: yoke:a-value-the-session-sets.01
func TestAParameterSetFromTheSessionReachesTheUnitAsAnArgument(t *testing.T) {
	vars, witness := sessionValue(t)
	if first := startedWith(t, vars, witness); strings.Contains(first, "HYPRLAND_INSTANCE_SIGNATURE=") {
		t.Errorf("the unit was handed the session's variable:\n%s", first)
	}
	expectDone(t, vars, "instance", "stop", "desktop")
}

// std: yoke:a-value-the-session-sets.02
func TestAValueSetWhileTheInstanceRunsReachesItsUnitsAtTheNextStart(t *testing.T) {
	vars, witness := sessionValue(t)
	startedWith(t, vars, witness)
	// While it runs, a second value: the record takes it, and nothing is launched for it.
	os.Remove(filepath.Join(witness, "last"))
	expectDone(t, vars, "instance", "set-param", "desktop", "probe.compositor", "def456_1794899999")
	if shown := expectDone(t, vars, "instance", "show", "desktop"); !strings.Contains(shown, "def456_1794899999") {
		t.Errorf("the record does not show the second value:\n%s", shown)
	}
	time.Sleep(time.Second)
	if _, err := os.Stat(filepath.Join(witness, "last")); err == nil {
		t.Error("setting the value launched a unit")
	}
	expectDone(t, vars, "instance", "restart", "desktop")
	recordedRun(t, witness, "def456_1794899999")
	expectDone(t, vars, "instance", "stop", "desktop")
}
