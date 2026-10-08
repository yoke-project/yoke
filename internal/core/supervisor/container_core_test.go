package supervisor_test

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// std: yoke:the-container-backend.06
func TestTheCoreRunsAOneshotInAContainer(t *testing.T) {
	image := announcingImage(t)
	socket := podmanSocket(t)
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	os.MkdirAll(filepath.Join(dir, "plugins.d"), 0o755)
	os.MkdirAll(filepath.Join(dir, "plugins"), 0o755)
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte("units:\n  announcer:\n    kind: oneshot\n    image: "+image+"\n"), 0o644)
	config := filepath.Join(dir, "core.yaml")
	os.WriteFile(config, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nengine: unix://%s\nplugins:\n  manifests: %s/plugins.d\n  executables: %s/plugins\n",
		dir, run, socket, dir, dir)), 0o644)
	core := exec.Command(binary)
	core.Env = append(os.Environ(), "YOKE_CONFIG="+config, "YOKE_COMPOSITION="+composition)
	out, _ := core.StderrPipe()
	if err := core.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { core.Process.Kill(); core.Wait() })
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	want := fmt.Sprintf("announced as uid=%d gid=%d", os.Getuid(), os.Getgid())
	announced, completed := false, false
	var said []string
	deadline := time.After(time.Minute)
	for !announced || !completed {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			if strings.Contains(line, want) && strings.Contains(line, "unit=announcer incarnation=1 stream=stdout") {
				announced = true
			}
			if strings.Contains(line, "type=unit.state.changed subject=unit:announcer#1") && strings.Contains(line, "to=Completed") {
				completed = true
			}
		case <-deadline:
			t.Fatalf("announced=%v completed=%v within a minute:\n%s", announced, completed, strings.Join(said, "\n"))
		}
	}
	core.Process.Signal(syscall.SIGTERM)
	for range lines {
	}
	core.Wait()
	left, _ := exec.Command("podman", "ps", "-aq", "--filter", "label=dev.yoke-project.unit=announcer",
		"--filter", "label=dev.yoke-project.instance="+instanceOf(said)).Output()
	if len(strings.TrimSpace(string(left))) != 0 {
		t.Errorf("containers are left under the instance's label: %s", left)
	}
}

// instanceOf is the instance's identity, as the Core's ready event names it.
func instanceOf(said []string) string {
	for _, line := range said {
		if _, after, ok := strings.Cut(line, "subject=instance:"); ok {
			name, _, _ := strings.Cut(after, " ")
			return name
		}
	}
	return "yoke"
}

// announcingImage builds, over scratch, a static copy of this test binary whose part is to announce
// itself, and returns it referenced by its digest.
func announcingImage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	build := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "unit"), ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the fixture does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY unit /unit\nENV "+role+"=announce\nENTRYPOINT [\"/unit\"]\n"), 0o644)
	tag := "localhost/yoke-l3-announcer:" + strconv.Itoa(os.Getpid())
	if said, err := exec.Command("podman", "build", "-q", "-t", tag, dir).CombinedOutput(); err != nil {
		t.Fatalf("the fixture image does not build: %v\n%s", err, said)
	}
	t.Cleanup(func() { exec.Command("podman", "rmi", "-f", tag).Run() })
	digest, err := exec.Command("podman", "images", "--digests", "--format", "{{.Digest}}", tag).Output()
	if err != nil {
		t.Fatal(err)
	}
	return "localhost/yoke-l3-announcer@" + strings.TrimSpace(string(digest))
}

// podmanSocket starts rootless Podman's API on a socket of the test's, and returns its path.
func podmanSocket(t *testing.T) string {
	t.Helper()
	dir, _ := os.MkdirTemp("", "eng-")
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "podman.sock")
	service := exec.Command("podman", "system", "service", "--time=0", "unix://"+socket)
	if err := service.Start(); err != nil {
		t.Fatalf("the environment declares rootless Podman, and it cannot be run: %v", err)
	}
	t.Cleanup(func() { service.Process.Kill(); service.Wait() })
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(socket); err == nil {
			return socket
		}
		if time.Now().After(deadline) {
			t.Fatal("Podman's API never appeared")
		}
	}
}
