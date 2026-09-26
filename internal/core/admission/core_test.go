package admission_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"
)

// The test binary is also the Plugin the Core launches: this variable, declared in the composition, says so.
const role = "TEST_UNIT_ROLE"

func TestMain(m *testing.M) {
	if os.Getenv(role) == "register" {
		os.Exit(registerAsAUnit())
	}
	os.Exit(m.Run())
}

// registerAsAUnit does what a unit does first: read the environment, register on the plugin channel with
// its token, and say what it was answered. It then stays, as a unit that was admitted does.
func registerAsAUnit() int {
	conn, err := grpc.NewClient("unix://"+os.Getenv("YOKE_SOCKET"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("no channel:", err)
		return 1
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := pluginv1.NewRegisterClient(conn).Register(ctx, &pluginv1.RegisterRequest{
		Plugin: os.Getenv("YOKE_PLUGIN"), Unit: os.Getenv("YOKE_UNIT"), Token: os.Getenv("YOKE_TOKEN"), Protocol: 1,
		ArtifactVersion: "0.0.1", Language: "go", SdkLine: "none",
		Declared: &pluginv1.Surface{Capabilities: []string{"stream.data.publish"}, Streams: []string{"station.data"}},
	})
	if err != nil {
		fmt.Println("no answer:", err)
		return 1
	}
	fmt.Printf("answered outcome=%s session=%d\n", resp.Outcome, len(resp.SessionId))
	time.Sleep(time.Hour)
	return 0
}

// std: yoke:admission.13
func TestALaunchedUnitRegistersAndIsAdmitted(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yk")
	t.Cleanup(func() { os.RemoveAll(run) })
	manifests, executables := filepath.Join(dir, "plugins.d"), filepath.Join(dir, "plugins")
	os.MkdirAll(filepath.Join(manifests, "com.example.station"), 0o755)
	os.MkdirAll(executables, 0o755)
	os.WriteFile(filepath.Join(manifests, "com.example.station", "manifest.yaml"), []byte(
		"manifest: 1\nid: com.example.station\nprotocol: 1\nstreams: [ { id: station.data } ]\ncapabilities: [ { name: stream.data.publish, governs: { stream: station.data } } ]\n"), 0o644)
	self, _ := os.Executable()
	if err := os.Symlink(self, filepath.Join(executables, "com.example.station")); err != nil {
		t.Fatal(err)
	}
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte("units:\n  acquire: { kind: plugin, plugin: com.example.station, env: { "+role+": register } }\n"), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s\n  executables: %s\n", dir, run, manifests, executables)), 0o644)

	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+composition)
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
	answered, admitted := false, false
	deadline := time.After(20 * time.Second)
	for !answered || !admitted {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited:\n%s", strings.Join(said, "\n"))
			}
			said = append(said, line)
			answered = answered || (strings.Contains(line, "unit=acquire") && strings.Contains(line, "outcome=OUTCOME_ACCEPTED_WITH_RESTRICTIONS session=43"))
			admitted = admitted || (strings.Contains(line, "msg=admission") && strings.Contains(line, "unit=acquire") && strings.Contains(line, "outcome=accepted"))
		case <-deadline:
			t.Fatalf("within twenty seconds the Core said:\n%s", strings.Join(said, "\n"))
		}
	}
	info, err := os.Stat(filepath.Join(run, "plugin.sock"))
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode().Perm() != 0o660 {
		t.Errorf("the plugin channel is %v (%v), want a socket of mode 0660", info, err)
	}
}
