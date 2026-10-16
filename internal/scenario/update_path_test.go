package scenario_test

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

// The units of the application the update carries: provision lays the datasets out once, migrate
// rewrites the layout and counts each of its runs.
const (
	provision = "#!/bin/sh\n[ -f \"$1/layout\" ] || printf v1 > \"$1/layout\"\n"
	migrate   = "#!/bin/sh\nprintf v2 > \"$1/layout\"\necho ran >> \"$1/migrations\"\n"
)

// std: yoke:the-update-path.01
func TestAnInstanceIsCarriedToANewVersionAndBack(t *testing.T) {
	bin := t.TempDir()
	for _, program := range []string{"yoke", "yoke-compose", "yoke-guardian", "yoke-core"} {
		if said, err := exec.Command("go", "build", "-o", filepath.Join(bin, program), "github.com/yoke-project/yoke/cmd/"+program).CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
	}
	// The Plugin on the host: this test binary, in the role TestMain gives it.
	plugin := filepath.Join(t.TempDir(), "bench")
	if said, err := exec.Command("go", "test", "-c", "-o", plugin, ".").CombinedOutput(); err != nil {
		t.Fatalf("the Plugin does not build: %v\n%s", err, said)
	}
	benchBytes, err := os.ReadFile(plugin)
	if err != nil {
		t.Fatal(err)
	}

	// One project, at two versions.
	project := func(version, units string) string {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "components"), 0o755)
		os.MkdirAll(filepath.Join(dir, "manifests"), 0o755)
		os.WriteFile(filepath.Join(dir, "components", "provision"), []byte(provision), 0o755)
		os.WriteFile(filepath.Join(dir, "components", "migrate"), []byte(migrate), 0o755)
		os.WriteFile(filepath.Join(dir, "components", "bench"), benchBytes, 0o755)
		os.WriteFile(filepath.Join(dir, "manifests", bench.ID+".yaml"), bench.Manifest(), 0o644)
		os.WriteFile(filepath.Join(dir, "yoke.yaml"), []byte(fmt.Sprintf(`model: 1
id: com.example.station
version: %s
arch: %s
core: ">=0.0.0-0"
data: ">=1.0"
policy: { startup_window: 30s }
units:
  provision: { kind: oneshot, exec: components/provision, digest: "sha256:%x", needs: [ "storage:datasets" ], args: [ "${bind.datasets}" ] }
%s`, version, compose.HostArch(), sha256.Sum256([]byte(provision)), units)), 0o644)
		return dir
	}
	benchUnit := func(after string) string {
		return fmt.Sprintf("  bench: { kind: plugin, plugin: %s, exec: components/bench, digest: \"sha256:%x\", env: { %s: plugin }, depends_on: [ %s ] }\n",
			bench.ID, sha256.Sum256(benchBytes), role, after)
	}
	v1 := project("1.0.0", benchUnit("provision"))
	v2 := project("1.1.0", fmt.Sprintf("  migrate: { kind: oneshot, exec: components/migrate, digest: \"sha256:%x\", needs: [ \"storage:datasets\" ], args: [ \"${bind.datasets}\" ], migrates_from: \"<1.1\", depends_on: [ provision ] }\n",
		sha256.Sum256([]byte(migrate)))+benchUnit("migrate"))

	// The account: its directories, its configuration, and the programs on its PATH.
	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	config := t.TempDir()
	os.MkdirAll(filepath.Join(config, "yoke"), 0o755)
	os.WriteFile(filepath.Join(config, "yoke", "manager.conf"), []byte(fmt.Sprintf("install_root: %s\ncore: %s\n",
		apps, filepath.Join(bin, "yoke-core"))), 0o644)
	run, err := os.MkdirTemp("", "ys")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	stateHome := t.TempDir()
	env := append(os.Environ(), "XDG_RUNTIME_DIR="+run, "XDG_STATE_HOME="+stateHome, "XDG_CONFIG_HOME="+config, "HOME="+t.TempDir(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() {
		if pid, _ := instance.Holder(filepath.Join(run, "yoke", "guardian.lock")); pid != 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	})
	yoke := func(program string, args ...string) (string, error) {
		cmd := exec.Command(filepath.Join(bin, program), args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	sh := func(program string, args ...string) string {
		t.Helper()
		out, err := yoke(program, args...)
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", program, strings.Join(args, " "), err, out)
		}
		return out
	}
	eventually := func(what string, within time.Duration, holds func() bool) {
		t.Helper()
		for deadline := time.Now().Add(within); !holds(); time.Sleep(200 * time.Millisecond) {
			if time.Now().After(deadline) {
				log, _ := os.ReadFile(filepath.Join(stateHome, "yoke", "guardian.log"))
				t.Fatalf("%s never held within %v; the guardian said:\n%s", what, within, log)
			}
		}
	}
	instances := instancerecord.Root(func(k string) string {
		if k == "XDG_STATE_HOME" {
			return stateHome
		}
		return ""
	})
	state := instancerecord.StateDir(instances, "bench-a")
	read := func(path string) string {
		b, _ := os.ReadFile(path)
		return strings.TrimSpace(string(b))
	}
	stamp := func() string { return read(filepath.Join(state, "version")) }
	layout := func() string { return read(filepath.Join(state, "storage", "datasets", "layout")) }
	migrations := func() int {
		return strings.Count(read(filepath.Join(state, "storage", "datasets", "migrations")), "ran")
	}
	unit := func(name string) string {
		out, _ := yoke("yoke", "unit", "show", name, "--instance", "bench-a")
		return strings.Join(strings.Fields(out), " ")
	}
	recorded := func() string {
		r, err := instancerecord.Read(instances, "bench-a")
		if err != nil {
			t.Fatal(err)
		}
		return r.Version
	}
	up := func(what string) {
		t.Helper()
		eventually(what, 2*time.Minute, func() bool { return strings.Contains(unit("bench"), `"state": "Running"`) })
	}
	stopped := func() {
		t.Helper()
		sh("yoke", "instance", "stop", "bench-a")
		eventually("bench-a's claim free", 2*time.Minute, func() bool {
			pid, _ := instance.Holder(filepath.Join(run, "yoke", "bench-a", instance.LockName))
			return pid == 0
		})
	}
	build := func(dir string) string {
		t.Helper()
		built := strings.Fields(sh("yoke-compose", "build", "--dir", dir, "--out", t.TempDir()))
		return built[len(built)-1]
	}
	pkg1, pkg2 := build(v1), build(v2)

	// 1.0.0, running.
	sh("yoke", "app", "install", pkg1)
	sh("yoke", "instance", "create", "com.example.station", "--name", "bench-a", "--version", "1.0.0")
	sh("yoke", "instance", "start", "bench-a")
	up("bench-a up at 1.0.0")
	if layout() != "v1" || stamp() != "1.0.0" {
		t.Fatalf("at 1.0.0 the layout is %q and the stamp %q", layout(), stamp())
	}

	// Installed beside, and running nothing.
	sh("yoke", "app", "install", pkg2)
	if got := recorded(); got != "1.0.0" {
		t.Errorf("installing 1.1.0 moved bench-a to %s", got)
	}

	// The copy, taken first, while stopped.
	stopped()
	backup := filepath.Join(t.TempDir(), "bench-a-backup")
	sh("yoke", "instance", "copy", "bench-a", backup)
	var copied string
	filepath.WalkDir(backup, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == "version" && copied == "" {
			copied = read(path)
		}
		return nil
	})
	if copied != "1.0.0" {
		t.Errorf("the copy holds the stamp %q", copied)
	}

	// The update itself: re-pointed, restarted, migrated once.
	sh("yoke", "instance", "set-version", "bench-a", "1.1.0")
	sh("yoke", "instance", "restart", "bench-a")
	up("bench-a up at 1.1.0")
	if !strings.Contains(unit("migrate"), `"state": "Completed"`) || layout() != "v2" || migrations() != 1 || stamp() != "1.1.0" {
		t.Errorf("after the update migrate is %s, the layout %q, %d migrations, the stamp %q", unit("migrate"), layout(), migrations(), stamp())
	}
	sh("yoke", "instance", "restart", "bench-a")
	eventually("bench-a up again at 1.1.0", 2*time.Minute, func() bool {
		return strings.Contains(unit("bench"), `"state": "Running"`) && strings.Contains(unit("migrate"), `"state": "Completed"`)
	})
	if migrations() != 1 {
		t.Errorf("the second start ran the migration again: %d runs", migrations())
	}

	// The rollback that does not work.
	stopped()
	out, err := yoke("yoke", "instance", "set-version", "bench-a", "1.0.0")
	t.Logf("the rollback was answered: %s", out)
	if err == nil {
		t.Errorf("the rollback onto the migrated state was accepted: %s", out)
	} else if !strings.Contains(out, "data.range") || !strings.Contains(out, "1.1.0") {
		t.Errorf("the rollback was refused without naming the range and the version: %s", out)
	}
	if got := recorded(); got != "1.1.0" {
		t.Errorf("the refused rollback moved the record to %s", got)
	}

	// The one that does.
	sh("yoke", "instance", "restore", backup, "bench-a")
	sh("yoke", "instance", "set-version", "bench-a", "1.0.0")
	sh("yoke", "instance", "start", "bench-a")
	up("bench-a up again at 1.0.0")
	if layout() != "v1" || migrations() != 0 || stamp() != "1.0.0" {
		t.Errorf("after the rollback the layout is %q, %d migrations, the stamp %q", layout(), migrations(), stamp())
	}

	// Forward again, and the superseded version reclaimed.
	stopped()
	sh("yoke", "instance", "set-version", "bench-a", "1.1.0")
	sh("yoke", "instance", "start", "bench-a")
	up("bench-a up at 1.1.0 once more")
	stopped()
	pruned := sh("yoke", "app", "prune", "com.example.station")
	t.Logf("prune said: %s", pruned)
	if !strings.Contains(pruned, "1.0.0") || strings.Contains(pruned, "1.1.0") {
		t.Errorf("prune said: %s", pruned)
	}
	if _, err := os.Stat(filepath.Join(apps, "com.example.station", "1.0.0")); !os.IsNotExist(err) {
		t.Errorf("1.0.0 is still installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(apps, "com.example.station", "1.1.0", "bundle")); err != nil {
		t.Errorf("1.1.0 is not installed any more: %v", err)
	}
}
