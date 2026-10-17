package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/instance"
)

// std: yoke:the-service-regime.06
func TestTheShippedUnitRunsTheDeploymentUnderSystemd(t *testing.T) {
	// The account's own systemd runs the unit the service form ships; the scope is the harness's, so the
	// manager asks `systemctl` as it always does.
	units := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "systemd", "user")
	if _, err := os.Stat(filepath.Join(units, Unit)); err == nil {
		t.Fatalf("%s already holds a %s, which this case will not replace", units, Unit)
	}
	bin := t.TempDir()
	for _, program := range []string{"yoke", "yoke-core"} {
		if said, err := exec.Command("go", "build", "-o", filepath.Join(bin, program), "github.com/yoke-project/yoke/cmd/"+program).CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
	}
	vars, _, clean := serviceForm(t)
	// The manager's start gates the composition in force, as the unit's exec does: both read the clean one.
	vars["YOKE_COMPOSITION"] = clean
	shipped, err := os.ReadFile(filepath.Join("..", "..", "packaging", Unit))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(units, Unit+".d"), 0o755)
	os.WriteFile(filepath.Join(units, Unit), shipped, 0o644)
	os.WriteFile(filepath.Join(units, Unit+".d", "test.conf"), []byte("[Service]\nExecStart=\nExecStart="+filepath.Join(bin, "yoke")+
		" service exec\nEnvironment=YOKE_CONFIG="+vars["YOKE_CONFIG"]+" YOKE_COMPOSITION="+clean+" PATH="+bin+":/usr/bin:/bin\n"), 0o644)
	user := func(args ...string) string {
		out, _ := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
		return strings.TrimSpace(string(out))
	}
	user("daemon-reload")
	t.Cleanup(func() {
		user("stop", Unit)
		os.RemoveAll(filepath.Join(units, Unit+".d"))
		os.Remove(filepath.Join(units, Unit))
		user("daemon-reload")
		user("reset-failed", Unit)
	})
	fakeBin(t, vars, "systemctl", "exec /usr/bin/systemctl --user \"$@\"\n")

	expectDone(t, vars, "service", "start")
	eventually(t, "the instance answering under the unit", 30*time.Second, func() bool {
		_, out := yoke(vars, "service", "status")
		return strings.Contains(out, "instance: answers")
	})
	out := expectDone(t, vars, "service", "status")
	if !strings.Contains(out, Unit+" is active") || strings.Contains(out, "disagreement") {
		t.Errorf("the status:\n%s", out)
	}
	expectDone(t, vars, "service", "stop")
	if got := user("is-active", Unit); got != "inactive" {
		t.Errorf("after the stop the unit is %q", got)
	}
	if pid, _ := instance.Holder(filepath.Join(vars["XDG_RUNTIME_DIR"], instance.LockName)); pid != 0 {
		t.Errorf("the claim is still held, by %d", pid)
	}
}
