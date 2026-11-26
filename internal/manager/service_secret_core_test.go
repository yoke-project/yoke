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

// rewriter is a Plugin that needs a secret it rewrites: its Manifest declares `rewrites: [ tokens ]`.
var rewriter = plugin.Declaration{ID: "com.example.rewriter", Needs: []string{"secret:tokens"}}

// runRewriter registers, then replaces the value it was handed at MANAGER_TEST_TOKENS by its own, as a unit
// renewing a token does: `value.new` beside it, renamed over `value`. It then waits to be told to stop.
func runRewriter() int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	u, err := plugin.Start(ctx, rewriter)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rewriter:", err)
		return 1
	}
	path := os.Getenv("MANAGER_TEST_TOKENS")
	old, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rewriter: the secret was not handed:", err)
		return 1
	}
	next := filepath.Join(filepath.Dir(path), "value.new")
	if err := os.WriteFile(next, append(old, []byte("-renewed")...), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "rewriter:", err)
		return 1
	}
	if err := os.Rename(next, path); err != nil {
		fmt.Fprintln(os.Stderr, "rewriter:", err)
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

// std: yoke:the-service-regime.10
func TestAServiceDeploymentStartsOnARewrittenSecretTheVerbSupplied(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	dir := t.TempDir()
	run, _ := os.MkdirTemp("", "yr")
	t.Cleanup(func() { os.RemoveAll(run) })
	manifests, executables := filepath.Join(dir, "plugins.d"), filepath.Join(dir, "plugins")
	os.MkdirAll(filepath.Join(manifests, rewriter.ID), 0o755)
	os.MkdirAll(executables, 0o755)
	os.WriteFile(filepath.Join(manifests, rewriter.ID, "manifest.yaml"), append(rewriter.Manifest(), []byte("rewrites: [ tokens ]\n")...), 0o644)
	self, _ := os.Executable()
	os.Symlink(self, filepath.Join(executables, rewriter.ID))
	composition := filepath.Join(dir, "rewriter.yaml")
	os.WriteFile(composition, []byte("units:\n  accounts: { kind: plugin, plugin: "+rewriter.ID+", env: { "+role+": rewriter, MANAGER_TEST_TOKENS: \"${bind.tokens}\" } }\n"), 0o644)
	core := filepath.Join(dir, "core.yaml")
	os.WriteFile(core, []byte(fmt.Sprintf("state_dir: %s/state\nruntime_dir: %s\nplugins:\n  manifests: %s\n  executables: %s\n", dir, run, manifests, executables)), 0o644)
	vars := map[string]string{"YOKE_CONFIG": core, "YOKE_COMPOSITION": composition}

	stdin = strings.NewReader("seeded\n")
	t.Cleanup(func() { stdin = os.Stdin })
	expectDone(t, vars, "service", "set-secret", "tokens")

	command := exec.Command(binary)
	command.Env = append(os.Environ(), "YOKE_CONFIG="+core, "YOKE_COMPOSITION="+composition)
	said := &strings.Builder{}
	command.Stdout, command.Stderr = said, said
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Process.Signal(syscall.SIGTERM); command.Wait() })

	value := filepath.Join(dir, "state", "secrets", "tokens", "value")
	eventually(t, "the unit's replacement of the value the verb wrote", 30*time.Second, func() bool {
		data, err := os.ReadFile(value)
		return err == nil && string(data) == "seeded-renewed"
	})
	if _, err := os.Stat(filepath.Join(filepath.Dir(value), "value.new")); err == nil {
		t.Error("value.new was left beside the value")
	}
	// Long enough for a restart the replacement provoked to be seen.
	time.Sleep(2 * time.Second)
	_, out := yoke(vars, "unit", "show", "accounts")
	if !strings.Contains(squash(out), `"state": "Running"`) || !strings.Contains(squash(out), `"incarnation": "1"`) {
		t.Errorf("the unit after its own replacement:\n%s\nthe Core said:\n%s", out, said)
	}
}
