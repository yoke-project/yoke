package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/instance"
)

// writes is a program writing its first argument to the file OUT names.
const writes = "#!/bin/sh\nprintf '%s' \"$1\" > \"$OUT\"\n"

// writerBlock is the block writer: a oneshot running components/probe with ${line}, writing to ${target}.
const writerBlock = `block: writer
model: 1
unit:
  kind: oneshot
  exec: components/probe
  args: [ "${line}" ]
  env: { OUT: "${target}" }
parameters:
  line: { type: string, required: false, default: "unset", label: "What the unit writes" }
  target: { type: string, required: false, default: "/dev/null", label: "Where it writes it" }
`

// std: yoke:a-project-written-by-verbs-alone.01
func TestAProjectWrittenByVerbsAloneRuns(t *testing.T) {
	bin := programs(t, "yoke", "yoke-core", "yoke-guardian", "yoke-compose")
	apps := filepath.Join(t.TempDir(), "apps")
	os.MkdirAll(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	vars := conf(t, "install_root: "+apps+"\ncore: "+filepath.Join(bin, "yoke-core")+"\n")
	run, err := os.MkdirTemp("", "ym")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	vars["XDG_RUNTIME_DIR"], vars["XDG_STATE_HOME"], vars["HOME"] = run, t.TempDir(), t.TempDir()
	vars["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Cleanup(func() {
		if pid := holder(vars, "guardian.lock"); pid != 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	})

	work := t.TempDir()
	project := filepath.Join(work, "station")
	block := filepath.Join(work, "blocks", "writer.yaml")
	os.MkdirAll(filepath.Dir(block), 0o755)
	os.WriteFile(block, []byte(writerBlock), 0o644)
	greeting, line := filepath.Join(work, "greeting"), filepath.Join(work, "line")
	compose := func(args ...string) string {
		t.Helper()
		c := exec.Command(filepath.Join(bin, "yoke-compose"), append(args, "--dir", project)...)
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("yoke-compose %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	compose("init", "com.yoke.composed", "--version", "1.0.0", "--core", ">=0.0.0-0")
	os.WriteFile(filepath.Join(project, "components", "probe"), []byte(writes), 0o755)
	compose("add", "parameter", "probe.greeting", "--type", "string", "--default", "hello from the verbs", "--label", "What the probe writes")
	compose("add", "parameter", "shared.out", "--type", "string", "--default", greeting, "--label", "Where the probe writes")
	compose("add", "unit", "probe", "--kind", "oneshot", "--exec", "components/probe", "--arg", "${greeting}")
	compose("set", "units.probe.env.OUT", "${out}")
	compose("set", "units.probe.policy.restart.on_failure", "true")
	compose("block", "add", block, "--as", "second")
	compose("set", "parameters.second.line.default", "placed by a block")
	compose("set", "parameters.second.target.default", line)

	out := compose("check")
	if strings.Contains(out, "refusal") || !strings.Contains(out, "oneshot.restart.unchecked  units.probe.policy.restart.on_failure") {
		t.Fatalf("the first check:\n%s", out)
	}
	compose("waive", "oneshot.restart.unchecked", "--at", "units.probe.policy.restart.on_failure", "--reason", "The probe only writes a file; running it twice is harmless.")
	if out := compose("check"); strings.Contains(out, "\nadvice") || strings.HasPrefix(out, "advice") || !strings.Contains(out, "waived: 1") {
		t.Fatalf("the second check:\n%s", out)
	}
	packaged := strings.TrimSpace(lastLine(compose("build", "--out", work)))
	if _, err := os.Stat(packaged); err != nil {
		t.Fatalf("build names %q: %v", packaged, err)
	}

	if out := expectDone(t, vars, "app", "install", packaged); !strings.Contains(out, "1.0.0") {
		t.Errorf("the install:\n%s", out)
	}
	expectDone(t, vars, "instance", "create", "com.yoke.composed", "--name", "bench")
	expectDone(t, vars, "instance", "start", "bench")
	eventually(t, "bench listed observable", 30*time.Second, func() bool {
		_, out := yoke(vars, "instance", "list")
		return strings.Contains(squash(out), "bench com.yoke.composed 1.0.0 observable")
	})
	eventually(t, "both units written", 30*time.Second, func() bool {
		g, _ := os.ReadFile(greeting)
		l, _ := os.ReadFile(line)
		return string(g) == "hello from the verbs" && string(l) == "placed by a block"
	})
	expectDone(t, vars, "instance", "stop", "bench")
	eventually(t, "bench's claim free", 30*time.Second, func() bool { return holder(vars, "bench", instance.LockName) == 0 })
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return lines[len(lines)-1]
}
