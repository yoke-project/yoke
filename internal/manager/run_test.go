package manager

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/instances"
)

// programs builds the named commands of this tree into a directory of their own.
func programs(t *testing.T, names ...string) string {
	t.Helper()
	bin := t.TempDir()
	for _, program := range names {
		if said, err := exec.Command("go", "build", "-o", filepath.Join(bin, program), "github.com/yoke-project/yoke/cmd/"+program).CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
	}
	return bin
}

// output is what a process has written so far, read while it runs.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

// following is `yoke app run` as a process of its own, whoever typed it able to interrupt it.
type following struct {
	cmd  *exec.Cmd
	out  *output
	done chan struct{}
	code int
}

func follows(t *testing.T, bin string, vars map[string]string, args ...string) *following {
	t.Helper()
	f := &following{out: &output{}, done: make(chan struct{})}
	f.cmd = exec.Command(filepath.Join(bin, "yoke"), append([]string{"app", "run"}, args...)...)
	for k, v := range vars {
		f.cmd.Env = append(f.cmd.Env, k+"="+v)
	}
	f.cmd.Stdout, f.cmd.Stderr = f.out, f.out
	if err := f.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		err := f.cmd.Wait()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			f.code = exit.ExitCode()
		}
		close(f.done)
	}()
	t.Cleanup(func() {
		f.cmd.Process.Kill()
		<-f.done
	})
	return f
}

// ended waits for the run to exit and returns its status.
func (f *following) ended(t *testing.T, within time.Duration) int {
	t.Helper()
	select {
	case <-f.done:
		return f.code
	case <-time.After(within):
		t.Fatalf("the run did not exit within %v:\n%s", within, f.out)
		return -1
	}
}

// runAccount is an account with com.yoke.station installed, the Core configured and the guardian on the
// PATH; its guardian, where one is left, is stopped when the test ends.
func runAccount(t *testing.T) (string, map[string]string) {
	t.Helper()
	bin := programs(t, "yoke", "yoke-core", "yoke-guardian")
	vars := account(t, "core: "+filepath.Join(bin, "yoke-core")+"\n")
	vars["PATH"] = bin + string(os.PathListSeparator) + os.Getenv("PATH")
	t.Cleanup(func() {
		if pid := holder(vars, "guardian.lock"); pid != 0 {
			syscall.Kill(pid, syscall.SIGTERM)
		}
	})
	return bin, vars
}

// theOnly is the one instance the account declares.
func theOnly(t *testing.T, vars map[string]string) (string, instancerecord.Record) {
	t.Helper()
	root := instancerecord.Root(environment(vars))
	listed, err := instancerecord.List(root)
	if err != nil || len(listed) != 1 || listed[0].Record == nil {
		t.Fatalf("the account declares %+v: %v", listed, err)
	}
	return listed[0].Name, *listed[0].Record
}

func observable(t *testing.T, f *following) {
	t.Helper()
	eventually(t, "the run observable", 30*time.Second, func() bool { return strings.Contains(f.out.String(), "is observable") })
}

// std: yoke:the-ephemeral-run.01
func TestARunWithNoRecordCreatesAnEphemeralOne(t *testing.T) {
	bin, vars := runAccount(t)
	f := follows(t, bin, vars, "com.yoke.station")
	observable(t, f)
	name, r := theOnly(t, vars)
	if !r.Ephemeral || r.Autostart || r.DesiredState != instancerecord.Running || r.Version != "2.1.0" ||
		r.Source != "" || r.Verification != "" || r.Admission != "" {
		t.Errorf("the record is %+v", r)
	}
	out := squash(expectDone(t, vars, "instance", "list"))
	if !strings.Contains(out, name+" com.yoke.station 2.1.0 observable") || !strings.Contains(out, "ephemeral") {
		t.Errorf("the list:\n%s", out)
	}

	f.cmd.Process.Signal(os.Interrupt)
	if code := f.ended(t, 30*time.Second); code != Done || !strings.Contains(f.out.String(), "has stopped") {
		t.Errorf("the interrupted run exited %d:\n%s", code, f.out)
	}
	if _, err := os.Stat(filepath.Join(instancerecord.Root(environment(vars)), name)); err == nil {
		t.Error("an orderly stop left the ephemeral record")
	}
	if pid := holder(vars, name, instance.LockName); pid != 0 {
		t.Errorf("the instance's claim is held by %d", pid)
	}
	eventually(t, "the guardian gone", 30*time.Second, func() bool { return holder(vars, "guardian.lock") == 0 })
}

// std: yoke:the-ephemeral-run.02
func TestARunThatFailsLeavesItsRecordUntilReclaimed(t *testing.T) {
	bin, vars := runAccount(t)
	f := follows(t, bin, vars, "com.yoke.station")
	observable(t, f)
	name, _ := theOnly(t, vars)
	core := holder(vars, name, instance.LockName)
	if core == 0 {
		t.Fatal("no Core holds the instance's claim")
	}
	syscall.Kill(core, syscall.SIGKILL)
	if code := f.ended(t, 30*time.Second); code != Fault || !strings.Contains(f.out.String(), "failed") || !strings.Contains(f.out.String(), "kept") {
		t.Errorf("the failed run exited %d:\n%s", code, f.out)
	}
	if _, r := theOnly(t, vars); r.Outcome != instancerecord.Failed || r.DesiredState != instancerecord.Running {
		t.Errorf("the record left is %+v", r)
	}
	if out := squash(expectDone(t, vars, "instance", "list")); !strings.Contains(out, name+" com.yoke.station 2.1.0 failed") {
		t.Errorf("the list:\n%s", out)
	}
	if out := expectDone(t, vars, "app", "prune", "com.yoke.station"); !strings.Contains(out, "removed record "+name) {
		t.Errorf("the prune:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(instancerecord.Root(environment(vars)), name)); err == nil {
		t.Error("the reclaiming pass left the failed record")
	}
}

// workingBundle is a bundle being written: com.yoke.bench 0.1.0, installed nowhere, with extra in its
// descriptor.
func workingBundle(t *testing.T, extra string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bench")
	os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	os.WriteFile(filepath.Join(dir, "components", "probe"), []byte(probe), 0o755)
	os.WriteFile(filepath.Join(dir, "yoke.yaml"), []byte(fmt.Sprintf("model: 1\nid: com.yoke.bench\nversion: 0.1.0\n%sarch: "+compose.HostArch()+"\n"+
		"core: \">=0.0.0-0\"\npolicy: { stop_window: 5s }\nunits:\n  probe: { kind: oneshot, exec: components/probe, digest: \"sha256:%x\" }\n",
		extra, sha256.Sum256([]byte(probe)))), 0o644)
	return dir
}

// std: yoke:the-ephemeral-run.03
func TestABundleBeingWrittenRunsUnverifiedAndSaysSo(t *testing.T) {
	bin, vars := runAccount(t)
	dir := workingBundle(t, "")
	f := follows(t, bin, vars, "--from", dir)
	observable(t, f)
	if !strings.Contains(f.out.String(), "not verified") {
		t.Errorf("the run does not say the directory is unverified:\n%s", f.out)
	}
	name, r := theOnly(t, vars)
	if r.Source != dir || r.Verification != instancerecord.Waived || r.Admission != "" || r.DescriptorDigest != "" || r.ID != "com.yoke.bench" {
		t.Errorf("the record is %+v", r)
	}
	if out := expectDone(t, vars, "core", "status", "--instance", name); !strings.Contains(out, "bundle.unverified at "+dir) {
		t.Errorf("the instance does not say it is unverified:\n%s", out)
	}
	f.cmd.Process.Signal(os.Interrupt)
	if code := f.ended(t, 30*time.Second); code != Done {
		t.Errorf("the interrupted run exited %d:\n%s", code, f.out)
	}
	if _, err := os.Stat(filepath.Join(instancerecord.Root(environment(vars)), name)); err == nil {
		t.Error("an orderly stop left the ephemeral record")
	}
}

// std: yoke:the-ephemeral-run.04
func TestOnlyAnInvocationCarriesAWaiver(t *testing.T) {
	vars := account(t, "")
	env := environment(vars)
	root := instancerecord.Root(env)
	name, err := instances.Ephemeral(instances.Options{InstallRoot: installRoot(t, vars), Instances: root,
		Runtime: filepath.Join(vars["XDG_RUNTIME_DIR"], "yoke")}, instances.Run{ID: "com.yoke.station", AdmitUnlaunched: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, r := theOnly(t, vars); r.Admission != instancerecord.UnlaunchedPermitted || r.Source != "" || r.Verification != "" || !r.Ephemeral {
		t.Errorf("the record of %s is %+v", name, r)
	}
	if code, out := yoke(vars, "instance", "create", "com.yoke.station", "--admit-unlaunched"); code != Refused || !strings.Contains(out, "admit-unlaunched") {
		t.Errorf("a declared record asked for the waiver: %d\n%s", code, out)
	}
	if code, out := yoke(vars, "app", "run", "--from", workingBundle(t, "verification: waived\n")); code != Refused || !strings.Contains(out, "verification") {
		t.Errorf("a descriptor exempting itself: %d\n%s", code, out)
	}
	theOnly(t, vars)
}

// std: yoke:the-ephemeral-run.05
func TestWhatARunCannotBeIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	vars := account(t, "")
	empty := t.TempDir()
	for _, args := range [][]string{
		{"app", "run"},
		{"app", "run", "com.yoke.other"},
		{"app", "run", "--from", workingBundle(t, ""), "com.yoke.station"},
		{"app", "run", "--from", empty},
	} {
		if code, out := yoke(vars, args...); code != Refused {
			t.Errorf("yoke %s exited %d:\n%s", strings.Join(args, " "), code, out)
		}
	}
	if listed, _ := instancerecord.List(instancerecord.Root(environment(vars))); len(listed) != 0 {
		t.Errorf("a refused run left %+v", listed)
	}
}

// std: yoke:the-ephemeral-run.06
func TestAStartThatNeverHappenedTakesItsRecordWithIt(t *testing.T) {
	bin, vars := runAccount(t)
	dir := workingBundle(t, "")
	descriptor := filepath.Join(dir, "yoke.yaml")
	data, _ := os.ReadFile(descriptor)
	os.WriteFile(descriptor, []byte(strings.Replace(string(data), `" }`+"\n", `", needs: [ "secret:token" ] }`+"\n", 1)), 0o644)
	f := follows(t, bin, vars, "--from", dir)
	if code := f.ended(t, 30*time.Second); code != Refused || !strings.Contains(f.out.String(), "secret.missing") ||
		!strings.Contains(f.out.String(), "nothing ran") {
		t.Errorf("the refused run exited %d:\n%s", code, f.out)
	}
	if listed, _ := instancerecord.List(instancerecord.Root(environment(vars))); len(listed) != 0 {
		t.Errorf("a start that never happened left %+v", listed)
	}
}
