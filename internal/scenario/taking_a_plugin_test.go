package scenario_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/instance"
)

// std: yoke:taking-a-plugin.07
func TestAPluginTakenFromItsSourceIsAdmittedByACore(t *testing.T) {
	bin := t.TempDir()
	for _, program := range []string{"yoke", "yoke-compose", "yoke-guardian", "yoke-core"} {
		if said, err := exec.Command("go", "build", "-o", filepath.Join(bin, program), "github.com/yoke-project/yoke/cmd/"+program).CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
	}
	source, err := filepath.Abs(filepath.Join("testdata", "greeter"))
	if err != nil {
		t.Fatal(err)
	}

	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	config := t.TempDir()
	os.MkdirAll(filepath.Join(config, "yoke"), 0o755)
	os.WriteFile(filepath.Join(config, "yoke", "manager.conf"), []byte(fmt.Sprintf("install_root: %s\ncore: %s\n",
		apps, filepath.Join(bin, "yoke-core"))), 0o644)
	run, err := os.MkdirTemp("", "yt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	state := t.TempDir()
	env := append(os.Environ(), "XDG_RUNTIME_DIR="+run, "XDG_STATE_HOME="+state, "XDG_CONFIG_HOME="+config,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() {
		if pid, _ := instance.Holder(filepath.Join(run, "yoke", "guardian.lock")); pid != 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	})
	sh := func(program string, args ...string) string {
		t.Helper()
		cmd := exec.Command(filepath.Join(bin, program), args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", program, strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	eventually := func(what string, within time.Duration, holds func() bool) {
		t.Helper()
		for deadline := time.Now().Add(within); !holds(); time.Sleep(200 * time.Millisecond) {
			if time.Now().After(deadline) {
				log, _ := os.ReadFile(filepath.Join(state, "yoke", "guardian.log"))
				t.Fatalf("%s never held within %v; the guardian said:\n%s", what, within, log)
			}
		}
	}

	project := filepath.Join(t.TempDir(), "station")
	// The Core is built here from a tree that is no release, so the project wants any Core.
	sh("yoke-compose", "init", "com.example.station", "--dir", project, "--core", ">=0.0.0-0")
	sh("yoke-compose", "add", "plugin", source, "--yes", "--dir", project)
	built := strings.Fields(sh("yoke-compose", "build", "--dir", project, "--out", t.TempDir()))
	sh("yoke", "app", "install", built[len(built)-1])
	sh("yoke", "instance", "create", "com.example.station", "--name", "station")
	sh("yoke", "instance", "start", "station")

	eventually("the greeter admitted and running", 2*time.Minute, func() bool {
		return strings.Contains(strings.Join(strings.Fields(sh("yoke", "unit", "show", "greeter", "--instance", "station")), " "), `"state": "Running"`)
	})
	filepath.Walk(apps, func(path string, info os.FileInfo, err error) error {
		if err == nil && info.Name() == "plugins.yaml" {
			t.Errorf("what was installed holds %s", path)
		}
		return nil
	})
	sh("yoke", "instance", "stop", "station")
}
