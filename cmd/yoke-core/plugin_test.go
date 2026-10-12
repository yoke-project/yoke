package main_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke-sdk-go/plugin"

	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
)

// role makes this test binary the Plugin a container runs: the same code a host would run.
const role = "CORE_TEST_ROLE"

var bench = plugin.Declaration{ID: "com.example.bench"}

func TestMain(m *testing.M) {
	if os.Getenv(role) == "plugin" {
		os.Exit(runBench())
	}
	os.Exit(m.Run())
}

// runBench registers and waits to be told to stop.
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

// std: yoke:a-plugin-in-a-container.01
func TestAPluginRegistersFromAContainerUnchanged(t *testing.T) {
	dir := t.TempDir()
	fixture := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "unit"), ".")
	fixture.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("the Plugin does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY unit /unit\nENV "+role+"=plugin\nENTRYPOINT [\"/unit\"]\n"), 0o644)
	image := enginetest.Image(t, dir, "yoke-l3-bench")
	s := enginetest.Serve(t)
	binary := build(t)
	env := account(t)
	b := bundle(t, "  bench: { kind: plugin, plugin: "+bench.ID+", image: \""+image+"\" }\n")
	os.MkdirAll(filepath.Join(b, "manifests"), 0o755)
	os.WriteFile(filepath.Join(b, "manifests", bench.ID+".yaml"), bench.Manifest(), 0o644)
	t.Cleanup(func() { enginetest.Remove(enginetest.Labelled("dev.yoke-project.instance=bench-p")...) })

	args := append([]string{"--instance", "bench-p", "--bundle", b, "--engine", "unix://" + s.Socket}, values(filepath.Join(t.TempDir(), "out"))...)
	cmd, lines := application(t, binary, env, args...)
	lines.until(10*time.Second, "msg=ready")
	lines.until(2*time.Minute, "subject=unit:bench#1", "to=Running")
	found := enginetest.Labelled("dev.yoke-project.instance=bench-p", "dev.yoke-project.unit=bench")
	if len(found) != 1 || !enginetest.Running(found[0]) {
		t.Errorf("the Plugin's container: %v", found)
	}
	cmd.Process.Signal(syscall.SIGTERM)
	said := lines.rest()
	if err := cmd.Wait(); err != nil {
		t.Errorf("the Core exited %v:\n%s", err, said)
	}
	for _, c := range enginetest.Labelled("dev.yoke-project.instance=bench-p") {
		if enginetest.Running(c) {
			t.Errorf("the container %s is still running", c)
		}
	}
}
