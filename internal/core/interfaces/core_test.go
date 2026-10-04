package interfaces_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	interfacev1 "github.com/yoke-project/yoke/proto/yoke/interface/v1"
)

// The test binary is also the managed interface the Core launches: this variable, declared in the
// composition, says so.
const role = "TEST_UNIT_ROLE"

func TestMain(m *testing.M) {
	if os.Getenv(role) == "panel" {
		os.Exit(reachTheChannel())
	}
	os.Exit(m.Run())
}

// reachTheChannel connects to the address its environment names, says so, and stays.
func reachTheChannel() int {
	address := os.Getenv("YOKE_SOCKET")
	conn, err := net.Dial("unix", address)
	if err != nil {
		fmt.Println("no channel at", address, err)
		return 1
	}
	defer conn.Close()
	fmt.Println("connected to", address)
	time.Sleep(time.Hour)
	return 0
}

// startCore builds yoke-core and starts it with the composition given, whose %s is the test binary and
// %s the role variable; it returns the runtime directory and the Core's output, line by line.
func startCore(t *testing.T, composition string) (string, <-chan string) {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	manifests, executables := filepath.Join(dir, "plugins.d"), filepath.Join(dir, "plugins")
	os.MkdirAll(manifests, 0o755)
	os.MkdirAll(executables, 0o755)
	self, _ := os.Executable()
	path := filepath.Join(dir, "bench.yaml")
	os.WriteFile(path, []byte(fmt.Sprintf(composition, self, role)), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s\n  executables: %s\n", dir, run, manifests, executables)), 0o644)
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+path)
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
	return run, lines
}

// std: yoke:channels-bound.05
func TestThroughTheCoreChannelsAreBoundAndTheManagedInterfaceReachesItsOwn(t *testing.T) {
	run, lines := startCore(t, `units:
  panel-ui: { kind: interface, exec: %s, env: { %s: panel } }
channels:
  front: { unit: panel-ui, transport: local, clients: single }
  remote: { transport: http+ws, clients: multiple, address: { class: loopback, port: "`+freePort(t)+`" } }
`)
	socket := filepath.Join(run, "interfaces", "front.sock")
	weaker, reached := false, false
	var said []string
	deadline := time.After(20 * time.Second)
	for !weaker || !reached {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			weaker = weaker || (strings.Contains(line, "level=WARN") && strings.Contains(line, "finding") && strings.Contains(line, "remote"))
			reached = reached || strings.Contains(line, "connected to "+socket)
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
}

// std: yoke:the-local-projection.05
func TestThroughTheCoreAClientAttachesAndIsGivenTheOpening(t *testing.T) {
	run, lines := startCore(t, `# The test binary, %s, runs nothing here; %s is unused.
units: {}
channels:
  bench: { transport: local, clients: multiple }
`)
	var said []string
	deadline := time.After(20 * time.Second)
	for ready := false; !ready; {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			ready = strings.Contains(line, "msg=ready")
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
	go func() {
		for range lines {
		}
	}()
	conn, err := grpc.NewClient("unix://"+filepath.Join(run, "interfaces", "bench.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := interfacev1.NewInterfaceClient(conn).Attach(ctx)
	if err != nil {
		t.Fatalf("%v; the Core said:\n%s", err, strings.Join(said, "\n"))
	}
	f, err := stream.Recv()
	if err != nil || f.GetOpening().GetVersion() != 1 {
		t.Errorf("the first frame is %v %v", f, err)
	}
}
