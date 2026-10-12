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

	"github.com/yoke-project/yoke/internal/instancerecord"
)

const linger = "#!/bin/sh\nexec /bin/sleep 300\n"

// secretsAccount is an account with com.yoke.station 2.1.0 installed, whose unit keeper needs
// secret:token, keyed needs secret:key, and other needs nothing; each runs until it is stopped.
func secretsAccount(t *testing.T) map[string]string {
	t.Helper()
	vars := account(t, "")
	bundle := filepath.Join(installRoot(t, vars), "com.yoke.station", "2.1.0", "bundle")
	os.WriteFile(filepath.Join(bundle, "components", "linger"), []byte(linger), 0o755)
	unit := func(name, needs string) string {
		return fmt.Sprintf("  %s: { kind: oneshot, exec: components/linger, digest: \"sha256:%x\"%s }\n", name, sha256.Sum256([]byte(linger)), needs)
	}
	os.WriteFile(filepath.Join(bundle, "yoke.yaml"), []byte("model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: aarch64\n"+
		"core: \">=0.0.0-0\"\npolicy: { stop_window: 2s }\nunits:\n"+unit("keeper", `, needs: [ "secret:token" ]`)+
		unit("keyed", `, needs: [ "secret:key" ]`)+unit("other", "")), 0o644)
	return vars
}

// installRoot is the install root the account's configuration names.
func installRoot(t *testing.T, vars map[string]string) string {
	t.Helper()
	cfg, err := LoadConfig(environment(vars))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.InstallRoot
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// std: yoke:the-manager-command-line.11
func TestYokeInstanceSetSecretWritesWithoutDisplayingIt(t *testing.T) {
	vars := secretsAccount(t)
	expectDone(t, vars, "instance", "create", "com.yoke.station", "--name", "bench-a")
	stdin = strings.NewReader("s3cr3t-value\n")
	t.Cleanup(func() { stdin = os.Stdin })
	out := expectDone(t, vars, "instance", "set-secret", "bench-a", "token")
	if strings.Contains(out, "s3cr3t-value") || !strings.Contains(out, "present") || !strings.Contains(out, "nothing is restarted") {
		t.Errorf("set-secret said:\n%s", out)
	}
	root := instancerecord.Root(environment(vars))
	path := filepath.Join(instancerecord.StateDir(root, "bench-a"), "secrets", "token")
	if data, err := os.ReadFile(path); err != nil || string(data) != "s3cr3t-value" {
		t.Errorf("the secret holds %q: %v", data, err)
	}
	for p, mode := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != mode {
			t.Errorf("%s: %v %v, want %v", p, info.Mode(), err, mode)
		}
	}
	show := squash(expectDone(t, vars, "instance", "show", "bench-a"))
	if !strings.Contains(show, "token present") || !strings.Contains(show, "key missing") || strings.Contains(show, "s3cr3t-value") {
		t.Errorf("the show:\n%s", show)
	}
	if strings.Contains(mustRead(t, instancerecord.File(root, "bench-a")), "s3cr3t") {
		t.Error("the record holds the secret")
	}
	stdin = strings.NewReader("other\n")
	if code, out := yoke(vars, "instance", "set-secret", "bench-a", "../token"); code != Refused {
		t.Errorf("a secret named ../token exited %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(instancerecord.StateDir(root, "bench-a"), "token")); err == nil {
		t.Error("a refused name wrote a file")
	}
}

// std: yoke:the-manager-command-line.12
func TestASecretSetOnARunningInstanceRestartsTheUnitsThatNeedIt(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	vars := secretsAccount(t)
	expectDone(t, vars, "instance", "create", "com.yoke.station", "--name", "bench-a")
	t.Cleanup(func() { stdin = os.Stdin })
	for _, s := range []string{"token", "key"} {
		stdin = strings.NewReader("first\n")
		expectDone(t, vars, "instance", "set-secret", "bench-a", s)
	}
	root := instancerecord.Root(environment(vars))
	r, _ := instancerecord.Read(root, "bench-a")
	bundle := filepath.Join(installRoot(t, vars), r.ID, r.Version, "bundle")
	core := exec.Command(binary, "--instance", "bench-a", "--bundle", bundle)
	core.Env = append(os.Environ(), "XDG_RUNTIME_DIR="+vars["XDG_RUNTIME_DIR"], "XDG_STATE_HOME="+vars["XDG_STATE_HOME"])
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { core.Process.Signal(syscall.SIGTERM); core.Wait() })
	running := func(unit, incarnation string) func() bool {
		return func() bool {
			_, out := yoke(vars, "unit", "show", unit, "--instance", "bench-a")
			return strings.Contains(squash(out), `"state": "Running"`) && strings.Contains(squash(out), `"incarnation": "`+incarnation+`"`)
		}
	}
	eventually(t, "keeper and other running", 30*time.Second, func() bool { return running("keeper", "1")() && running("other", "1")() })

	stdin = strings.NewReader("second\n")
	out := expectDone(t, vars, "instance", "set-secret", "bench-a", "token")
	if !strings.Contains(out, "restarted keeper") || strings.Contains(out, "other") || strings.Contains(out, "second") {
		t.Errorf("set-secret said:\n%s", out)
	}
	eventually(t, "keeper in its second incarnation", 30*time.Second, running("keeper", "2"))
	if !running("other", "1")() {
		_, out := yoke(vars, "unit", "show", "other", "--instance", "bench-a")
		t.Errorf("other was touched:\n%s", out)
	}
}
