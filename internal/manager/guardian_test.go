package manager

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

// holdClaim is this test binary holding an instance's claim, at the root MANAGER_TEST_ROOT names, until
// it is killed.
func holdClaim() int {
	if _, err := instance.Claim(os.Getenv("MANAGER_TEST_ROOT"), 0o700); err != nil {
		fmt.Println("no claim:", err)
		return 1
	}
	fmt.Println("held")
	select {}
}

// claimed runs this test binary as another process holding the claim of the instance named.
func claimed(t *testing.T, vars map[string]string, name string) {
	t.Helper()
	self, _ := os.Executable()
	cmd := exec.Command(self, "-test.run=^$")
	cmd.Env = append(os.Environ(), role+"=claim", "MANAGER_TEST_ROOT="+filepath.Join(vars["XDG_RUNTIME_DIR"], "yoke", name))
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	buf := make([]byte, 5)
	if _, err := io.ReadFull(out, buf); err != nil || string(buf) != "held\n" {
		t.Fatalf("the claim was not taken: %q %v", buf, err)
	}
}

const probe = "#!/bin/sh\nexit 0\n"

// account is an install root holding com.yoke.station 2.1.0, and the variables of an account whose
// directories are the test's own, with extra in its configuration.
func account(t *testing.T, extra string) map[string]string {
	t.Helper()
	apps := filepath.Join(t.TempDir(), "apps")
	bundle := filepath.Join(apps, "com.yoke.station", "2.1.0", "bundle")
	os.MkdirAll(filepath.Join(bundle, "components"), 0o755)
	os.WriteFile(filepath.Join(bundle, "components", "probe"), []byte(probe), 0o755)
	os.WriteFile(filepath.Join(bundle, "yoke.yaml"), []byte(fmt.Sprintf("model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: "+compose.HostArch()+"\n"+
		"core: \">=0.0.0-0\"\npolicy: { stop_window: 5s }\nunits:\n  probe: { kind: oneshot, exec: components/probe, digest: \"sha256:%x\" }\n",
		sha256.Sum256([]byte(probe)))), 0o644)
	os.WriteFile(filepath.Join(apps, "com.yoke.station", "2.1.0", "install.yaml"), []byte("state: complete\nsealed: false\nshared: true\n"), 0o664)
	vars := conf(t, "install_root: "+apps+"\n"+extra)
	run, err := os.MkdirTemp("", "ym")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	vars["XDG_RUNTIME_DIR"], vars["XDG_STATE_HOME"], vars["HOME"] = run, t.TempDir(), t.TempDir()
	return vars
}

func expectDone(t *testing.T, vars map[string]string, args ...string) string {
	t.Helper()
	code, out := yoke(vars, args...)
	if code != Done {
		t.Fatalf("yoke %s exited %d:\n%s", strings.Join(args, " "), code, out)
	}
	return out
}

func holder(vars map[string]string, file ...string) int {
	pid, _ := instance.Holder(filepath.Join(append([]string{vars["XDG_RUNTIME_DIR"], "yoke"}, file...)...))
	return pid
}

func eventually(t *testing.T, what string, within time.Duration, holds func() bool) {
	t.Helper()
	for deadline := time.Now().Add(within); !holds(); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("%s never held within %v", what, within)
		}
	}
}

// std: yoke:the-manager-command-line.08
func TestYokeInstanceStartAndStopTellAGuardianStartingOne(t *testing.T) {
	bin := t.TempDir()
	for _, program := range []string{"yoke-core", "yoke-guardian"} {
		if said, err := exec.Command("go", "build", "-o", filepath.Join(bin, program), "github.com/yoke-project/yoke/cmd/"+program).CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
	}
	vars := account(t, "core: "+filepath.Join(bin, "yoke-core")+"\n")
	vars["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Cleanup(func() {
		if pid := holder(vars, "guardian.lock"); pid != 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	})
	expectDone(t, vars, "instance", "create", "com.yoke.station", "--name", "bench-a")

	expectDone(t, vars, "instance", "start", "bench-a")
	if r, _ := instancerecord.Read(instancerecord.Root(environment(vars)), "bench-a"); r.DesiredState != instancerecord.Running {
		t.Errorf("the record says %q", r.DesiredState)
	}
	eventually(t, "a guardian running", 10*time.Second, func() bool { return holder(vars, "guardian.lock") != 0 })
	if _, err := os.Stat(filepath.Join(vars["XDG_STATE_HOME"], "yoke", "guardian.log")); err != nil {
		t.Errorf("the guardian's output is not in guardian.log: %v", err)
	}
	eventually(t, "bench-a listed observable", 30*time.Second, func() bool {
		_, out := yoke(vars, "instance", "list")
		return strings.Contains(squash(out), "bench-a com.yoke.station 2.1.0 observable")
	})
	first := holder(vars, "bench-a", instance.LockName)

	expectDone(t, vars, "instance", "restart", "bench-a")
	eventually(t, "bench-a observable again under another process", 30*time.Second, func() bool {
		_, out := yoke(vars, "instance", "list")
		now := holder(vars, "bench-a", instance.LockName)
		return now != 0 && now != first && strings.Contains(squash(out), "bench-a com.yoke.station 2.1.0 observable")
	})

	expectDone(t, vars, "instance", "stop", "bench-a")
	if r, _ := instancerecord.Read(instancerecord.Root(environment(vars)), "bench-a"); r.DesiredState != instancerecord.Stopped {
		t.Errorf("the record says %q", r.DesiredState)
	}
	eventually(t, "bench-a's claim free and the guardian gone", 30*time.Second, func() bool {
		return holder(vars, "bench-a", instance.LockName) == 0 && holder(vars, "guardian.lock") == 0
	})
}

// std: yoke:the-manager-command-line.14
func TestAStartTheGateRefusesIsSaidAndStartsNoGuardian(t *testing.T) {
	_, vars := runAccount(t)
	cfg, err := LoadConfig(environment(vars))
	if err != nil {
		t.Fatal(err)
	}
	descriptor := filepath.Join(cfg.InstallRoot, "com.yoke.station", "2.1.0", "bundle", "yoke.yaml")
	data, err := os.ReadFile(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(descriptor, []byte(strings.Replace(string(data), `" }`+"\n", `", needs: [ "secret:token" ] }`+"\n", 1)), 0o644)
	expectDone(t, vars, "instance", "create", "com.yoke.station", "--name", "bench-a")

	for _, verb := range []string{"start", "restart"} {
		code, out := yoke(vars, "instance", verb, "bench-a")
		if code != Refused || !strings.Contains(out, "secret.missing") || !strings.Contains(out, "nothing ran") {
			t.Errorf("yoke instance %s exited %d:\n%s", verb, code, out)
		}
	}
	if r, _ := instancerecord.Read(instancerecord.Root(environment(vars)), "bench-a"); r.DesiredState != instancerecord.Running {
		t.Errorf("the record says %q", r.DesiredState)
	}
	if pid := holder(vars, "guardian.lock"); pid != 0 {
		t.Errorf("a guardian was started for a start the gate refuses: %d", pid)
	}
}

// std: yoke:the-manager-command-line.09
func TestEnableAndDisableAndClearAndRemove(t *testing.T) {
	vars := account(t, "")
	root := instancerecord.Root(environment(vars))
	for _, name := range []string{"bench-a", "bench-b"} {
		expectDone(t, vars, "instance", "create", "com.yoke.station", "--name", name)
	}
	state := instancerecord.StateDir(root, "bench-a")
	os.MkdirAll(state, 0o700)
	os.WriteFile(filepath.Join(state, "logs.db"), []byte("x"), 0o600)
	claimed(t, vars, "bench-b")

	expectDone(t, vars, "instance", "enable", "bench-a")
	if r, _ := instancerecord.Read(root, "bench-a"); !r.Autostart || r.DesiredState != instancerecord.Stopped {
		t.Errorf("after enable: %+v", r)
	}
	expectDone(t, vars, "instance", "disable", "bench-a")
	if r, _ := instancerecord.Read(root, "bench-a"); r.Autostart {
		t.Errorf("after disable: %+v", r)
	}
	for _, verb := range []string{"clear", "remove"} {
		if code, out := yoke(vars, "instance", verb, "bench-b"); code != Refused || !strings.Contains(out, "running") {
			t.Errorf("%s bench-b exited %d:\n%s", verb, code, out)
		}
	}
	if _, err := os.Stat(instancerecord.File(root, "bench-b")); err != nil {
		t.Errorf("a refused verb removed bench-b: %v", err)
	}
	expectDone(t, vars, "instance", "clear", "bench-a")
	if _, err := os.Stat(filepath.Join(state, "logs.db")); err == nil {
		t.Error("clear left the state")
	}
	if _, err := os.Stat(instancerecord.File(root, "bench-a")); err != nil {
		t.Errorf("clear took the record: %v", err)
	}
	expectDone(t, vars, "instance", "remove", "bench-a")
	if _, err := os.Stat(filepath.Join(root, "bench-a")); err == nil {
		t.Error("remove left the instance's directory")
	}
}

// std: yoke:the-manager-command-line.10
func TestYokeInstanceListAnswersWithoutAGuardian(t *testing.T) {
	vars := account(t, "")
	for _, name := range []string{"bench-a", "bench-b"} {
		expectDone(t, vars, "instance", "create", "com.yoke.station", "--name", name)
	}
	claimed(t, vars, "bench-b")
	out := squash(expectDone(t, vars, "instance", "list"))
	for _, want := range []string{"bench-a com.yoke.station 2.1.0 stopped", "bench-b com.yoke.station 2.1.0 running"} {
		if !strings.Contains(out, want) {
			t.Errorf("the list does not say %q:\n%s", want, out)
		}
	}
	if pid := holder(vars, "guardian.lock"); pid != 0 {
		t.Errorf("listing started a guardian, process %d", pid)
	}
}

// setupProgram is the component a fixture's unit setup names, and setupDigest its digest.
const setupProgram = "#!/bin/sh\nexit 0\n"

var setupDigest = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(setupProgram)))
