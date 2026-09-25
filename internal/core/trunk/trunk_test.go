package trunk_test

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/core/trunk"
)

// A test binary re-executed with this variable set claims the root it names, says so, and waits; or,
// with holdOnce set too, tries once and says what happened.
const (
	holdRoot = "YOKE_TEST_HOLD_ROOT"
	holdOnce = "YOKE_TEST_HOLD_ONCE"
)

func TestMain(m *testing.M) {
	if root := os.Getenv(holdRoot); root != "" {
		claim, err := instance.Claim(root, trunk.Application.RootMode())
		if err != nil {
			os.Stdout.WriteString("refused\n")
			os.Exit(3)
		}
		os.Stdout.WriteString("held\n")
		if os.Getenv(holdOnce) != "" {
			claim.Release()
			os.Exit(0)
		}
		select {}
	}
	os.Exit(m.Run())
}

// holder starts another process that holds the claim on root.
func holder(t *testing.T, root string) {
	t.Helper()
	command := exec.Command(os.Args[0])
	command.Env = append(os.Environ(), holdRoot+"="+root)
	out, _ := command.StdoutPipe()
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if line, _ := bufio.NewReader(out).ReadString('\n'); strings.TrimSpace(line) != "held" {
		t.Fatalf("the other process did not hold the claim: %q", line)
	}
	t.Cleanup(func() { command.Process.Kill(); command.Wait() })
}

// claimable says whether another process can take the claim on root now.
func claimable(root string) bool {
	command := exec.Command(os.Args[0])
	command.Env = append(os.Environ(), holdRoot+"="+root, holdOnce+"=1")
	said, _ := command.Output()
	return strings.TrimSpace(string(said)) == "held"
}

// service is a service-form state whose core.yaml puts both directories under the test's own.
func service(t *testing.T) (*trunk.State, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "core.yaml")
	document := "state_dir: " + dir + "/state\nruntime_dir: " + dir + "/run\nlog:\n  level: debug\n"
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	logged := &bytes.Buffer{}
	env := map[string]string{"YOKE_CONFIG": path}
	return &trunk.State{Form: trunk.Service, Env: func(k string) string { return env[k] }, Stderr: logged}, logged
}

// application is an application-form state named bench-a, under the test's own directories.
func application(t *testing.T) (*trunk.State, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	logged := &bytes.Buffer{}
	env := map[string]string{"XDG_RUNTIME_DIR": dir + "/run", "XDG_STATE_HOME": dir + "/state"}
	return &trunk.State{Form: trunk.Application, Name: "bench-a", Env: func(k string) string { return env[k] }, Stderr: logged}, logged
}

// recorded wraps every step so that its name is written to started when it starts.
func recorded(steps []trunk.Step, started *[]string) []trunk.Step {
	out := make([]trunk.Step, len(steps))
	for i, s := range steps {
		s := s
		out[i] = trunk.Step{Name: s.Name, Start: func(st *trunk.State) error {
			*started = append(*started, s.Name)
			return s.Start(st)
		}}
	}
	return out
}

// replaced returns steps with the one named name starting as start.
func replaced(steps []trunk.Step, name string, start func(*trunk.State) error) []trunk.Step {
	for i := range steps {
		if steps[i].Name == name {
			steps[i].Start = start
		}
	}
	return steps
}

func failing(*trunk.State) error { return errors.New("the step failed on purpose") }

var theEleven = []string{
	"identity and paths", "parameters", "logging", "claim", "debris", "stores",
	"inherited runtime facts", "declarations", "channels", "ready", "units",
}

// std: yoke:the-trunk.01
func TestTheElevenStepsRunInTheirOrder(t *testing.T) {
	st, _ := service(t)
	var started []string
	if err := trunk.Run(st, recorded(trunk.Steps(), &started)); err != nil {
		t.Fatalf("the trunk did not run: %v", err)
	}
	defer st.Stop()
	if strings.Join(started, ",") != strings.Join(theEleven, ",") {
		t.Errorf("the steps ran as %q, want %q", started, theEleven)
	}
}

// std: yoke:the-trunk.02
func TestAFailureBeforeReadinessIsFatal(t *testing.T) {
	st, _ := service(t)
	var started []string
	err := trunk.Run(st, recorded(replaced(trunk.Steps(), "stores", failing), &started))
	if err == nil || !strings.Contains(err.Error(), "stores") {
		t.Errorf("a failing stores step gave %v, want an error naming it", err)
	}
	if last := started[len(started)-1]; last != "stores" {
		t.Errorf("after the stores step failed, %q started", last)
	}

	st, _ = service(t)
	steps := trunk.Steps()
	var inserted []trunk.Step
	for _, s := range steps {
		if s.Name == "ready" {
			inserted = append(inserted, trunk.Step{Name: "inserted", Start: failing})
		}
		inserted = append(inserted, s)
	}
	started = nil
	if err := trunk.Run(st, recorded(inserted, &started)); err == nil || !strings.Contains(err.Error(), "inserted") {
		t.Errorf("a step inserted before readiness and failing gave %v, want a fatal error naming it", err)
	}
	for _, name := range started {
		if name == "ready" || name == "units" {
			t.Errorf("%s started after a failure before readiness", name)
		}
	}
}

// std: yoke:the-trunk.03
func TestAFailureAfterReadinessIsReported(t *testing.T) {
	st, logged := service(t)
	if err := trunk.Run(st, replaced(trunk.Steps(), "units", failing)); err != nil {
		t.Errorf("a failure after readiness stopped the trunk: %v", err)
	}
	defer st.Stop()
	if !strings.Contains(logged.String(), "units") || !strings.Contains(logged.String(), "failed on purpose") {
		t.Errorf("the failure was not written to the process logger: %s", logged)
	}
}

// std: yoke:the-trunk.04
func TestDeclarationsAreFatalInTheApplicationFormAlone(t *testing.T) {
	st, _ := service(t)
	var started []string
	if err := trunk.Run(st, recorded(replaced(trunk.Steps(), "declarations", failing), &started)); err != nil {
		t.Errorf("in the service form, failing declarations stopped the trunk: %v", err)
	}
	st.Stop()
	if !strings.Contains(strings.Join(started, ","), "ready") {
		t.Errorf("in the service form, readiness was not reached: %v", started)
	}

	app, _ := application(t)
	if err := trunk.Run(app, replaced(trunk.Steps(), "declarations", failing)); err == nil {
		app.Stop()
		t.Error("in the application form, failing declarations did not stop the trunk")
	}
}

// std: yoke:the-trunk.05
func TestAChannelThatCannotBeBoundIsFatal(t *testing.T) {
	st, _ := service(t)
	var started []string
	steps := recorded(replaced(trunk.Steps(), "parameters", func(st *trunk.State) error {
		if err := trunk.Steps()[1].Start(st); err != nil {
			return err
		}
		// A root deep enough that its channel's path is 108 characters.
		root := st.Paths.Root
		deep := filepath.Join(root, strings.Repeat("d", 108-len(root)-len("//plugin.sock")))
		st.Paths = instance.ServicePaths(deep, st.Paths.State)
		return nil
	}), &started)
	st.Channels = []trunk.Channel{{Name: "plugin", Path: "plugin.sock", Serve: func(trunk.Listener) {}}}
	err := trunk.Run(st, steps)
	if err == nil {
		st.Stop()
		t.Fatal("a channel whose path is 108 characters was bound")
	}
	if !strings.Contains(err.Error(), "108") {
		t.Errorf("the refusal does not name the path's length: %v", err)
	}
	for _, name := range started {
		if name == "ready" {
			t.Error("readiness was reached with a channel unbound")
		}
	}
}

// std: yoke:the-trunk.06
func TestDebrisIsEverythingButTheClaim(t *testing.T) {
	root := filepath.Join(t.TempDir(), "run")
	for _, file := range []string{"plugin.sock", "plugins/acquire.sock", "plugins/acquire/streams/spectra.sock", "instance.lock"} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, file)), 0o700)
		os.WriteFile(filepath.Join(root, file), nil, 0o600)
	}
	claim, err := instance.Claim(root, trunk.Application.RootMode())
	if err != nil {
		t.Fatal(err)
	}
	defer claim.Release()
	if err := trunk.ClearDebris(root); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if strings.Join(left, ",") != "instance.lock" {
		t.Errorf("the root holds %v after clearing, want only instance.lock", left)
	}
}

// std: yoke:the-trunk.07
func TestTheClaimIsTakenBeforeAnythingIsCleared(t *testing.T) {
	st, _ := service(t)
	if err := trunk.Run(st, trunk.Steps()[:2]); err != nil {
		t.Fatal(err)
	}
	root := st.Paths.Root
	os.MkdirAll(root, 0o700)
	stray := filepath.Join(root, "plugin.sock")
	os.WriteFile(stray, nil, 0o600)
	holder(t, root)

	st, _ = service(t)
	st.Env = func(k string) string {
		if k == "YOKE_CONFIG" {
			return filepath.Join(filepath.Dir(root), "core.yaml")
		}
		return ""
	}
	if err := trunk.Run(st, trunk.Steps()); err == nil || !strings.Contains(err.Error(), "claim") {
		st.Stop()
		t.Errorf("a trunk on a held instance gave %v, want a failure at the claim", err)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Errorf("the stray socket of the process holding the instance was removed: %v", err)
	}
}

// std: yoke:the-trunk.08
func TestTheRootsModeIsTheFormsWhateverTheUmask(t *testing.T) {
	for _, mask := range []int{0o000, 0o077} {
		before := syscall.Umask(mask)
		for form, want := range map[trunk.Form]os.FileMode{trunk.Service: os.ModeSetgid | 0o750, trunk.Application: 0o700} {
			root := filepath.Join(t.TempDir(), "run")
			claim, err := instance.Claim(root, form.RootMode())
			if err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(root)
			if got := info.Mode() & (os.ModePerm | os.ModeSetgid); got != want {
				t.Errorf("under umask %03o the root is %v, want %v", mask, got, want)
			}
			stat := info.Sys().(*syscall.Stat_t)
			if int(stat.Uid) != os.Getuid() || int(stat.Gid) != os.Getgid() {
				t.Errorf("the root is owned by %d:%d, not the process's %d:%d", stat.Uid, stat.Gid, os.Getuid(), os.Getgid())
			}
			claim.Release()
		}
		syscall.Umask(before)
	}
}

// std: yoke:the-trunk.09
func TestAChannelsSocketTakesTheFormsModeAndThePathTheCeiling(t *testing.T) {
	for form, want := range map[trunk.Form]os.FileMode{trunk.Service: 0o660, trunk.Application: 0o600} {
		dir := t.TempDir()
		// A name of as many characters as brings the path to 107, the ceiling.
		path := filepath.Join(dir, strings.Repeat("a", 107-len(dir)-len("/.sock"))+".sock")
		listener, err := trunk.Bind(path, form)
		if err != nil {
			t.Fatalf("a path of %d characters was refused: %v", len(path), err)
		}
		info, _ := os.Stat(path)
		if info.Mode()&os.ModePerm != want {
			t.Errorf("the socket's mode is %v, want %v", info.Mode()&os.ModePerm, want)
		}
		listener.Close()

		longer := strings.TrimSuffix(path, ".sock") + "a.sock"
		if l, err := trunk.Bind(longer, form); err == nil {
			l.Close()
			t.Errorf("a path of %d characters was bound", len(longer))
		} else if !strings.Contains(err.Error(), "108") {
			t.Errorf("the refusal does not name the length: %v", err)
		}
	}
}

// std: yoke:the-trunk.10
func TestStoppingUndoesTheTrunkInReverse(t *testing.T) {
	st, _ := service(t)
	var stopped []string
	steps := trunk.Steps()
	var withStoppers []trunk.Step
	for _, s := range steps {
		s := s
		withStoppers = append(withStoppers, trunk.Step{Name: s.Name, Start: func(st *trunk.State) error {
			if err := s.Start(st); err != nil {
				return err
			}
			st.OnStop(func() error { stopped = append(stopped, s.Name); return nil })
			return nil
		}})
	}
	st.Channels = []trunk.Channel{{Name: "plugin", Path: "plugin.sock", Serve: func(trunk.Listener) {}}}
	if err := trunk.Run(st, withStoppers); err != nil {
		t.Fatal(err)
	}
	root := st.Paths.Root
	if _, err := os.Stat(filepath.Join(root, "plugin.sock")); err != nil {
		t.Fatalf("the channel was not bound: %v", err)
	}
	if err := st.Stop(); err != nil {
		t.Errorf("stopping failed: %v", err)
	}
	for i, name := range stopped {
		if want := theEleven[len(theEleven)-1-i]; name != want {
			t.Errorf("stop %d undid %s, want %s", i, name, want)
			break
		}
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Errorf("the runtime directory is still there: %v", err)
	}
	if !claimable(root) {
		t.Error("another process could not take the claim after the stop")
	}
}
