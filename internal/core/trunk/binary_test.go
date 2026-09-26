package trunk_test

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// core builds yoke-core, and writes a core.yaml under dir at the given level.
func core(t *testing.T, dir, level string) (binary string, env []string) {
	t.Helper()
	binary = filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	path := filepath.Join(dir, "core.yaml")
	document := "state_dir: " + dir + "/state\nruntime_dir: " + dir + "/run\nlog:\n  level: " + level + "\n"
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	// A clean start composes something, if only nothing: a composition that is absent is refused.
	if err := os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte("units: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return binary, append(os.Environ(), "YOKE_CONFIG="+path, "YOKE_COMPOSITION="+dir+"/deployment.yaml")
}

// ready starts the Core and waits for the line saying it is ready, returning what it said by then.
func ready(t *testing.T, binary string, env []string) (*exec.Cmd, []string) {
	t.Helper()
	command := exec.Command(binary)
	command.Env = env
	out, _ := command.StderrPipe()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	var said []string
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited before it was ready: %v", said)
			}
			said = append(said, line)
			if strings.Contains(line, "msg=ready") {
				go func() {
					for range lines {
					}
				}()
				return command, said
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("the Core was never ready: %v", said)
		}
	}
}

// std: yoke:the-trunk.11
func TestTheProcessLoggerWritesTextGovernedByTheLevel(t *testing.T) {
	dir := t.TempDir()
	binary, env := core(t, dir, "info")
	command, said := ready(t, binary, env)
	command.Process.Signal(syscall.SIGTERM)
	command.Wait()
	joined := strings.Join(said, "\n") + "\n"
	for _, step := range theEleven[2:9] {
		if !strings.Contains(joined, `step="`+step+`"`) && !strings.Contains(joined, "step="+step+"\n") {
			t.Errorf("no line names the step %q: %s", step, joined)
		}
	}
	if !strings.Contains(joined, "msg=ready") {
		t.Errorf("no line says the Core is ready: %s", joined)
	}
	for _, line := range said {
		if !strings.Contains(line, "level=") || !strings.Contains(line, "msg=") {
			t.Errorf("a line is not key=value text: %q", line)
		}
	}

	quiet := t.TempDir()
	binary, env = core(t, quiet, "error")
	command = exec.Command(binary)
	command.Env = env
	stderr, _ := command.StderrPipe()
	command.Start()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(quiet, "run", "instance.lock")); err == nil {
			break
		}
	}
	time.Sleep(200 * time.Millisecond)
	command.Process.Signal(syscall.SIGTERM)
	rest, _ := bufio.NewReader(stderr).ReadString(0)
	command.Wait()
	if strings.TrimSpace(rest) != "" {
		t.Errorf("at level error, a clean start and stop said: %s", rest)
	}
}

// std: yoke:the-trunk.12
func TestTheCoreIsReadyWithNoChannelAndAnOrderlyStopLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	binary, env := core(t, dir, "info")
	command, _ := ready(t, binary, env)
	if err := command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Errorf("the Core did not exit zero: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "run")); !os.IsNotExist(err) {
		t.Errorf("the runtime directory is still there after an orderly stop: %v", err)
	}
}

// std: yoke:the-trunk.13
func TestACoreThatDiedUncleanLeavesItsDebrisToTheNextStart(t *testing.T) {
	dir := t.TempDir()
	binary, env := core(t, dir, "info")
	first, _ := ready(t, binary, env)
	stray := filepath.Join(dir, "run", "plugins", "acquire.sock")
	os.MkdirAll(filepath.Dir(stray), 0o700)
	os.WriteFile(stray, nil, 0o600)
	first.Process.Signal(syscall.SIGKILL)
	first.Wait()

	second, _ := ready(t, binary, env)
	defer func() { second.Process.Signal(syscall.SIGTERM); second.Wait() }()
	entries, _ := os.ReadDir(filepath.Join(dir, "run"))
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	// What is there once the next start is ready is its claim and the plugin channel it bound itself.
	if strings.Join(left, ",") != "instance.lock,plugin.sock" {
		t.Errorf("the next start left %v under the root, want only instance.lock and its own plugin.sock", left)
	}
}
