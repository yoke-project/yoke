package manager

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke-sdk-go/plugin"
)

// role makes this test binary the Plugin a composition runs.
const role = "MANAGER_TEST_ROLE"

var bench = plugin.Declaration{ID: "com.example.bench"}

func TestMain(m *testing.M) {
	if os.Getenv(role) == "plugin" {
		os.Exit(runBench())
	}
	if os.Getenv(role) == "claim" {
		os.Exit(holdClaim())
	}
	os.Exit(m.Run())
}

// runBench is a Plugin that registers and waits to be told to stop.
func runBench() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	u, err := plugin.Start(ctx, bench)
	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		return 1
	}
	for {
		select {
		case _, open := <-u.Events():
			if !open {
				return 0
			}
		case <-ctx.Done():
			u.Close()
			<-u.Done()
			return 0
		}
	}
}

// startCore runs a Core in the service form, composing one unit of the bench Plugin, and returns the
// environment that reaches it through its core.yaml.
func startCore(t *testing.T) map[string]string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "ym")
	t.Cleanup(func() { os.RemoveAll(run) })
	manifests, executables := filepath.Join(dir, "plugins.d"), filepath.Join(dir, "plugins")
	os.MkdirAll(filepath.Join(manifests, bench.ID), 0o755)
	os.MkdirAll(executables, 0o755)
	os.WriteFile(filepath.Join(manifests, bench.ID, "manifest.yaml"), bench.Manifest(), 0o644)
	self, _ := os.Executable()
	os.Symlink(self, filepath.Join(executables, bench.ID))
	composition := filepath.Join(dir, "bench.yaml")
	os.WriteFile(composition, []byte("units:\n  bench: { kind: plugin, plugin: "+bench.ID+", env: { "+role+": plugin } }\n"), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s\n  executables: %s\n", dir, run, manifests, executables)), 0o644)
	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+composition)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Signal(syscall.SIGTERM); command.Wait() })
	vars := map[string]string{"YOKE_CONFIG": core}
	deadline := time.Now().Add(30 * time.Second)
	for {
		_, status := yoke(vars, "core", "status")
		_, units := yoke(vars, "unit", "show", "bench")
		if strings.Contains(squash(status), `"ready": true`) && strings.Contains(units, `"Running"`) {
			return vars
		}
		if time.Now().After(deadline) {
			_, log := yoke(vars, "log", "query", "--unit", "bench")
			t.Fatalf("the instance did not come up:\n%s\n%s\n%s", status, units, log)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// squash collapses white space, which the JSON a read prints does not hold steady.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// std: yoke:the-manager-command-line.03
func TestAClientVerbReachesARunningInstance(t *testing.T) {
	vars := startCore(t)
	expect := func(want []string, args ...string) {
		t.Helper()
		code, out := yoke(vars, args...)
		if code != Done {
			t.Fatalf("yoke %s exited %d:\n%s", strings.Join(args, " "), code, out)
		}
		for _, w := range want {
			if !strings.Contains(squash(out), w) {
				t.Errorf("yoke %s does not say %q:\n%s", strings.Join(args, " "), w, out)
			}
		}
	}
	expect([]string{`"ready": true`}, "core", "status")
	expect([]string{"bench"}, "unit", "list")
	expect([]string{bench.ID, `"enabled": true`}, "plugin", "show", bench.ID)
	expect([]string{"previously enabled", "effective immediately", "consequence bench"}, "plugin", "disable", bench.ID)
	expect([]string{"nothing changed"}, "plugin", "disable", bench.ID)
	expect([]string{"previously disabled"}, "plugin", "enable", bench.ID)
	expect([]string{"consequence bench"}, "unit", "restart", "bench")
	expect([]string{"bench/"}, "log", "query", "--unit", "bench")
}
