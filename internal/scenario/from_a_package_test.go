package scenario_test

import (
	"context"
	"crypto/sha256"
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

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
	"github.com/yoke-project/yoke/internal/core/instance"
)

// role makes this test binary the Plugin the image runs: written against the Go library, as an author
// would write it.
const role = "SCENARIO_ROLE"

var bench = plugin.Declaration{ID: "com.example.bench"}

func TestMain(m *testing.M) {
	if os.Getenv(role) == "plugin" {
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
		defer stop()
		u, err := plugin.Start(ctx, bench)
		if err != nil {
			fmt.Fprintln(os.Stderr, "bench:", err)
			os.Exit(1)
		}
		<-ctx.Done()
		u.Close()
		<-u.Done()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const probe = "#!/bin/sh\nprintf '%s' \"$1\" > \"$2\"\n"

// std: yoke:from-a-package-to-a-running-instance.01
func TestAPackageBecomesARunningInstanceAndStops(t *testing.T) {
	bin := t.TempDir()
	for _, program := range []string{"yoke", "yoke-compose", "yoke-guardian", "yoke-core"} {
		if said, err := exec.Command("go", "build", "-o", filepath.Join(bin, program), "github.com/yoke-project/yoke/cmd/"+program).CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
	}
	images := t.TempDir()
	fixture := exec.Command("go", "test", "-c", "-o", filepath.Join(images, "unit"), ".")
	fixture.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("the Plugin does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(images, "Containerfile"), []byte("FROM scratch\nCOPY unit /unit\nENV "+role+"=plugin\nENTRYPOINT [\"/unit\"]\n"), 0o644)
	image := enginetest.Image(t, images, "yoke-l3-scenario")
	engine := enginetest.Serve(t)

	// The project, as an author lays it out.
	project := t.TempDir()
	written := filepath.Join(t.TempDir(), "label")
	os.MkdirAll(filepath.Join(project, "components"), 0o755)
	os.MkdirAll(filepath.Join(project, "manifests"), 0o755)
	os.WriteFile(filepath.Join(project, "components", "probe"), []byte(probe), 0o755)
	os.WriteFile(filepath.Join(project, "manifests", bench.ID+".yaml"), bench.Manifest(), 0o644)
	os.WriteFile(filepath.Join(project, "yoke.yaml"), []byte(fmt.Sprintf(`model: 1
id: com.example.station
version: 1.0.0
arch: %s
core: ">=0.0.0-0"
parameters:
  shared:
    label: { type: string, required: true, label: Label }
    out: { type: string, required: true, label: Where the probe writes }
policy: { startup_window: 30s }
units:
  probe: { kind: oneshot, exec: components/probe, digest: "sha256:%x", args: [ "${label}", "${out}" ] }
  bench: { kind: plugin, plugin: %s, image: "%s" }
`, compose.HostArch(), sha256.Sum256([]byte(probe)), bench.ID, image)), 0o644)

	// The account: its directories, its configuration, and the programs on its PATH.
	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	config := t.TempDir()
	os.MkdirAll(filepath.Join(config, "yoke"), 0o755)
	os.WriteFile(filepath.Join(config, "yoke", "manager.conf"), []byte(fmt.Sprintf("install_root: %s\ncore: %s\nengine: unix://%s\n",
		apps, filepath.Join(bin, "yoke-core"), engine.Socket)), 0o644)
	run, err := os.MkdirTemp("", "ys")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	state := t.TempDir()
	env := append(os.Environ(), "XDG_RUNTIME_DIR="+run, "XDG_STATE_HOME="+state, "XDG_CONFIG_HOME="+config, "HOME="+t.TempDir(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Cleanup(func() {
		if pid, _ := instance.Holder(filepath.Join(run, "yoke", "guardian.lock")); pid != 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
		enginetest.Remove(enginetest.Labelled("dev.yoke-project.instance=bench-a")...)
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

	built := strings.Fields(sh("yoke-compose", "build", "--dir", project, "--out", t.TempDir()))
	pkg := built[len(built)-1]
	sh("yoke", "app", "install", pkg)
	values := filepath.Join(t.TempDir(), "values.toml")
	os.WriteFile(values, []byte(fmt.Sprintf("[shared]\nlabel = \"Bench A\"\nout = %q\n", written)), 0o644)
	sh("yoke", "instance", "create", "com.example.station", "--name", "bench-a", "--params", values)
	sh("yoke", "instance", "start", "bench-a")

	eventually("bench-a listed observable", 2*time.Minute, func() bool {
		return strings.Contains(strings.Join(strings.Fields(sh("yoke", "instance", "list")), " "), "bench-a com.example.station 1.0.0 observable")
	})
	unit := func(name string) string {
		return strings.Join(strings.Fields(sh("yoke", "unit", "show", name, "--instance", "bench-a")), " ")
	}
	eventually("probe completed and bench running", 2*time.Minute, func() bool {
		return strings.Contains(unit("probe"), `"state": "Completed"`) && strings.Contains(unit("bench"), `"state": "Running"`)
	})
	if got, err := os.ReadFile(written); err != nil || string(got) != "Bench A" {
		t.Errorf("the probe wrote %q: %v", got, err)
	}
	if found := enginetest.Labelled("dev.yoke-project.instance=bench-a", "dev.yoke-project.unit=bench"); len(found) != 1 || !enginetest.Running(found[0]) {
		t.Errorf("the Plugin's container: %v", found)
	}

	sh("yoke", "instance", "stop", "bench-a")
	eventually("bench-a's claim free, its containers ended, the guardian gone", 2*time.Minute, func() bool {
		for _, c := range enginetest.Labelled("dev.yoke-project.instance=bench-a") {
			if enginetest.Running(c) {
				return false
			}
		}
		core, _ := instance.Holder(filepath.Join(run, "yoke", "bench-a", instance.LockName))
		guardian, _ := instance.Holder(filepath.Join(run, "yoke", "guardian.lock"))
		return core == 0 && guardian == 0
	})
}
