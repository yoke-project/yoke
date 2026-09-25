package instance_test

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/yoke-project/yoke/internal/core/instance"
)

// A test binary re-executed with this variable set claims the root it names, says so, and waits.
const holdRoot = "YOKE_TEST_HOLD_ROOT"

func TestMain(m *testing.M) {
	if root := os.Getenv(holdRoot); root != "" {
		if _, err := instance.Claim(root, 0o700); err != nil {
			os.Stdout.WriteString("refused " + err.Error() + "\n")
			os.Exit(3)
		}
		os.Stdout.WriteString("held\n")
		select {}
	}
	os.Exit(m.Run())
}

// holder starts another process that claims root, and returns it once it holds the claim.
func holder(t *testing.T, root string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0])
	command.Env = append(os.Environ(), holdRoot+"="+root)
	out, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	if strings.TrimSpace(line) != "held" {
		command.Process.Kill()
		command.Wait()
		t.Fatalf("the other process did not hold the claim: %q", line)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
	return command
}

// std: yoke:the-core-process.07
func TestANameThatCouldChooseWhereItWritesIsRefused(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	t.Setenv("XDG_STATE_HOME", "/home/a/.local/state")
	for _, name := range []string{"bench-a", "Bench A"} {
		if err := instance.ValidateName(name); err != nil {
			t.Errorf("%q is refused: %v", name, err)
			continue
		}
		paths, err := instance.ApplicationPaths(name, os.Getenv)
		if err != nil {
			t.Fatalf("the paths of %q are refused: %v", name, err)
		}
		if filepath.Base(paths.Root) != name {
			t.Errorf("%q appears in its root as %q", name, filepath.Base(paths.Root))
		}
	}
	for _, name := range []string{"a/b", "..", ".", "a\x00b", ""} {
		if err := instance.ValidateName(name); err == nil {
			t.Errorf("%q is accepted", name)
		}
		if _, err := instance.ApplicationPaths(name, os.Getenv); err == nil {
			t.Errorf("paths are derived from %q", name)
		}
	}
}

// std: yoke:the-core-process.08
func TestEveryPathIsDerivedFromTheName(t *testing.T) {
	service := instance.ServicePaths("/run/yoke", "/var/lib/yoke")
	if service != (instance.Paths{Name: "yoke", Root: "/run/yoke", State: "/var/lib/yoke", Lock: "/run/yoke/instance.lock"}) {
		t.Errorf("the service form derives %+v", service)
	}
	lookup := func(name string) string {
		return map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000", "XDG_STATE_HOME": "/home/a/.local/state"}[name]
	}
	application, err := instance.ApplicationPaths("bench-a", lookup)
	if err != nil {
		t.Fatal(err)
	}
	want := instance.Paths{
		Name:  "bench-a",
		Root:  "/run/user/1000/yoke/bench-a",
		State: "/home/a/.local/state/yoke/instances/bench-a/state",
		Lock:  "/run/user/1000/yoke/bench-a/instance.lock",
	}
	if application != want {
		t.Errorf("the application form derives %+v, want %+v", application, want)
	}
}

// std: yoke:the-core-process.09
func TestASecondProcessIsRefusedNamingTheHolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench-a")
	other := holder(t, root)
	_, err := instance.Claim(root, 0o700)
	var running *instance.AlreadyRunning
	if !errors.As(err, &running) {
		t.Fatalf("a held instance is claimed, or refused for another reason: %v", err)
	}
	if running.PID != other.Process.Pid {
		t.Errorf("the refusal names process %d, and the holder is %d", running.PID, other.Process.Pid)
	}
	if !strings.Contains(err.Error(), strconv.Itoa(other.Process.Pid)) {
		t.Errorf("the refusal does not say who holds it: %v", err)
	}
}

// std: yoke:the-core-process.10
func TestTheClaimIsReleasedWhenItsHolderDies(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench-a")
	other := holder(t, root)
	if err := other.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	other.Wait()
	claim, err := instance.Claim(root, 0o700)
	if err != nil {
		t.Fatalf("the claim outlived its holder: %v", err)
	}
	defer claim.Release()
	if _, err := os.Stat(filepath.Join(root, "instance.lock")); err != nil {
		t.Errorf("the lock file is not where the holder left it: %v", err)
	}
}

// std: yoke:the-core-process.11
func TestASecondClaimInTheHolderNeitherSucceedsNorReleases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "bench-a")
	first, err := instance.Claim(root, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	if second, err := instance.Claim(root, 0o700); err == nil {
		second.Release()
		t.Fatal("a second claim in the holding process succeeded")
	}
	command := exec.Command(os.Args[0])
	command.Env = append(os.Environ(), holdRoot+"="+root)
	said, _ := command.Output()
	if !strings.HasPrefix(string(said), "refused") {
		t.Errorf("after a second claim in the holder, another process was not refused: %q", said)
	}
}
