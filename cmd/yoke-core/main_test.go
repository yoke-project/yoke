package main_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// build compiles yoke-core into a directory of the test's own.
func build(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	return binary
}

// configured writes a core.yaml whose directories are under root, with extra appended, and returns the
// environment that names it.
func configured(t *testing.T, root, extra string) []string {
	t.Helper()
	path := filepath.Join(root, "core.yaml")
	document := "state_dir: " + root + "/state\nruntime_dir: " + root + "/run\n" + extra
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	return append(os.Environ(), "YOKE_CONFIG="+path, "YOKE_COMPOSITION="+root+"/deployment.yaml")
}

// holds waits until the lock file exists, or gives up.
func holds(lock string) bool {
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(lock); err == nil {
			return true
		}
	}
	return false
}

// std: yoke:the-core-process.12
func TestTheCoreServesOneInstanceAtATime(t *testing.T) {
	binary, root := build(t), t.TempDir()
	env := configured(t, root, "")
	lock := filepath.Join(root, "run", "instance.lock")

	first := exec.Command(binary)
	first.Env = env
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Process.Kill(); first.Wait() })
	if !holds(lock) {
		t.Fatal("the first Core never took the claim")
	}

	second := exec.Command(binary)
	second.Env = env
	said, err := second.CombinedOutput()
	if err == nil {
		t.Fatal("a second Core started on a claimed instance")
	}
	if !strings.Contains(string(said), "already running") || !strings.Contains(string(said), strconv.Itoa(first.Process.Pid)) {
		t.Errorf("the second Core does not say who is running: %s", said)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Errorf("the second Core removed the lock: %v", err)
	}

	if err := first.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := first.Wait(); err != nil {
		t.Errorf("the first Core did not exit zero on SIGTERM: %v", err)
	}

	third := exec.Command(binary)
	third.Env = env
	out, err := third.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := third.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { third.Process.Kill(); third.Wait() })
	done := make(chan error, 1)
	go func() { done <- third.Wait() }()
	select {
	case err := <-done:
		rest, _ := bufio.NewReader(out).ReadString(0)
		t.Fatalf("the third Core exited (%v) instead of holding the claim: %s", err, rest)
	case <-time.After(2 * time.Second):
	}
}

// std: yoke:the-core-process.13
func TestAConfigurationThatDoesNotPassPreventsStartup(t *testing.T) {
	binary, root := build(t), t.TempDir()
	command := exec.Command(binary)
	command.Env = configured(t, root, "hot_reload: true\n")
	said, err := command.CombinedOutput()
	if err == nil {
		t.Fatal("a Core with an unknown key in core.yaml started")
	}
	if !strings.Contains(string(said), "hot_reload") {
		t.Errorf("the refusal does not name the key: %s", said)
	}
	if _, err := os.Stat(filepath.Join(root, "run")); !os.IsNotExist(err) {
		t.Errorf("the runtime directory exists after a refused start: %v", err)
	}
}
