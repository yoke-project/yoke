package manager

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// fakeBin puts on a PATH of the test's own a program of the name given whose body is the script given.
func fakeBin(t *testing.T, vars map[string]string, name, script string) {
	t.Helper()
	dir := vars["PATH"]
	if dir == "" {
		dir = t.TempDir()
		vars["PATH"] = dir
	}
	if err := os.WriteFile(filepath.Join(strings.SplitN(dir, string(os.PathListSeparator), 2)[0], name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// serviceForm is the variables of a host whose core.yaml and compositions are the test's own: one
// composition refusing two units, one with nothing to refuse.
func serviceForm(t *testing.T) (vars map[string]string, refused, clean string) {
	t.Helper()
	dir := t.TempDir()
	run, err := os.MkdirTemp("", "ys")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	config := filepath.Join(dir, "core.yaml")
	os.WriteFile(config, []byte("state_dir: "+dir+"/state\nruntime_dir: "+run+"\nplugins:\n  manifests: "+dir+"/plugins.d\n  executables: "+dir+"/bin\n"), 0o644)
	refused, clean = filepath.Join(dir, "refused.yaml"), filepath.Join(dir, "clean.yaml")
	os.WriteFile(refused, []byte("units:\n  a: { kind: oneshot, exec: /nonexistent/a }\n  b: { kind: oneshot, exec: /nonexistent/b }\n"), 0o644)
	os.WriteFile(clean, []byte("units: {}\n"), 0o644)
	return map[string]string{"YOKE_CONFIG": config, "YOKE_COMPOSITION": refused, "XDG_RUNTIME_DIR": run}, refused, clean
}

// std: yoke:the-service-regime.01
func TestServiceStartStopAndRestartAskTheInitSystem(t *testing.T) {
	vars, _, clean := serviceForm(t)
	vars["YOKE_COMPOSITION"] = clean
	asked := filepath.Join(t.TempDir(), "asked")
	fakeBin(t, vars, "systemctl", `echo "$@" >> `+asked+"\n")
	for _, verb := range []string{"start", "stop", "restart"} {
		expectDone(t, vars, "service", verb)
	}
	if got, _ := os.ReadFile(asked); string(got) != "start yoke.service\nstop yoke.service\nrestart yoke.service\n" {
		t.Errorf("systemctl was asked:\n%s", got)
	}
	fakeBin(t, vars, "systemctl", "echo 'Job for yoke.service failed.' >&2\nexit 1\n")
	if code, out := yoke(vars, "service", "start"); code != Fault || !strings.Contains(out, "Job for yoke.service failed.") {
		t.Errorf("a failing start exited %d:\n%s", code, out)
	}
}

// std: yoke:the-service-regime.02
func TestServiceStatusNamesADisagreement(t *testing.T) {
	vars, _, _ := serviceForm(t)
	// The account holds an instance record, and the deployment is still the one asked.
	vars["XDG_STATE_HOME"] = t.TempDir()
	os.MkdirAll(filepath.Join(vars["XDG_STATE_HOME"], "yoke", "instances", "bench-a"), 0o700)
	fakeBin(t, vars, "systemctl", "echo active\n")
	out := expectDone(t, vars, "service", "status")
	for _, want := range []string{"yoke.service is active", "does not answer", "disagreement: active according to the init system, not answering on its surface",
		filepath.Join(vars["XDG_RUNTIME_DIR"], "operator.sock")} {
		if !strings.Contains(out, want) {
			t.Errorf("the status does not say %q:\n%s", want, out)
		}
	}
	fakeBin(t, vars, "systemctl", "echo inactive\nexit 3\n")
	out = expectDone(t, vars, "service", "status")
	if !strings.Contains(out, "yoke.service is inactive") || !strings.Contains(out, "does not answer") || strings.Contains(out, "disagreement") {
		t.Errorf("the status of a stopped deployment:\n%s", out)
	}
}

// std: yoke:the-service-regime.03
func TestServiceCheckRunsTheGateOverTheCompositionInForce(t *testing.T) {
	vars, _, clean := serviceForm(t)
	code, out := yoke(vars, "service", "check")
	if code != Refused || strings.Count(out, "component.missing") != 2 {
		t.Errorf("checking the refused composition exited %d:\n%s", code, out)
	}
	if out := expectDone(t, vars, "service", "check", "--composition", clean); !strings.Contains(out, "passes") {
		t.Errorf("checking the clean composition:\n%s", out)
	}
}

// std: yoke:the-service-regime.04
func TestServiceSetSecretWritesIntoTheDeploymentsArea(t *testing.T) {
	vars, _, _ := serviceForm(t)
	state := filepath.Join(filepath.Dir(vars["YOKE_CONFIG"]), "state")
	stdin = strings.NewReader("s3cr3t-value\n")
	t.Cleanup(func() { stdin = os.Stdin })
	out := expectDone(t, vars, "service", "set-secret", "token")
	if strings.Contains(out, "s3cr3t-value") || !strings.Contains(out, "present") {
		t.Errorf("set-secret said:\n%s", out)
	}
	path := filepath.Join(state, "secrets", "token")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "s3cr3t-value" {
		t.Errorf("the secret holds %q: %v", data, err)
	}
	for p, mode := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != mode {
			t.Errorf("%s: %v %v, want %v", p, info.Mode(), err, mode)
		}
	}
	stdin = strings.NewReader("other\n")
	if code, out := yoke(vars, "service", "set-secret", "../token"); code != Refused {
		t.Errorf("a secret named ../token exited %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(state, "token")); err == nil {
		t.Error("a refused name wrote a file")
	}
}

// std: yoke:the-service-regime.05
func TestServiceExecGatesAndThenExecutesTheCoreInItsPlace(t *testing.T) {
	vars, refused, clean := serviceForm(t)
	bin := t.TempDir()
	yokeBinary := filepath.Join(bin, "yoke")
	if said, err := exec.Command("go", "build", "-o", yokeBinary, "github.com/yoke-project/yoke/cmd/yoke").CombinedOutput(); err != nil {
		t.Fatalf("yoke does not build: %v\n%s", err, said)
	}
	executed := filepath.Join(t.TempDir(), "executed")
	os.WriteFile(filepath.Join(bin, "yoke-core"), []byte("#!/bin/sh\necho \"$$ $@\" > "+executed+"\n"), 0o755)
	run := func(composition string) (*exec.Cmd, string, error) {
		cmd := exec.Command(yokeBinary, "service", "exec", "--composition", composition)
		cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "YOKE_CONFIG="+vars["YOKE_CONFIG"])
		out, err := cmd.CombinedOutput()
		return cmd, string(out), err
	}
	_, out, err := run(refused)
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != Refused || !strings.Contains(out, "component.missing") {
		t.Errorf("exec of the refused composition: %v\n%s", err, out)
	}
	if _, err := os.Stat(executed); err == nil {
		t.Fatal("a Core was executed for a refused composition")
	}
	cmd, out, err := run(clean)
	if err != nil {
		t.Fatalf("exec of the clean composition: %v\n%s", err, out)
	}
	got, _ := os.ReadFile(executed)
	if want := strconv.Itoa(cmd.Process.Pid) + " --composition " + clean + "\n"; string(got) != want {
		t.Errorf("the Core was executed as %q, want %q", got, want)
	}
}

// std: yoke:the-service-regime.07
func TestTheShippedUnitNeverRestartsARefusal(t *testing.T) {
	unit, err := os.ReadFile(filepath.Join("..", "..", "packaging", "yoke.service"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(unit), "\n")
	var restart, prevent []string
	for _, l := range lines {
		if v, ok := strings.CutPrefix(l, "Restart="); ok {
			restart = append(restart, v)
		}
		if v, ok := strings.CutPrefix(l, "RestartPreventExitStatus="); ok {
			prevent = append(prevent, strings.Fields(v)...)
		}
	}
	if len(restart) != 1 || restart[0] != "on-failure" {
		t.Errorf("Restart= %v", restart)
	}
	if len(prevent) != 1 || prevent[0] != strconv.Itoa(Refused) {
		t.Errorf("RestartPreventExitStatus= %v, where a refusal exits %d", prevent, Refused)
	}
}

// std: yoke:the-service-regime.08
func TestAServiceStartTheGateRefusesIsSaidAndNothingIsAsked(t *testing.T) {
	vars, _, _ := serviceForm(t)
	asked := filepath.Join(t.TempDir(), "asked")
	fakeBin(t, vars, "systemctl", `echo "$@" >> `+asked+"\n")
	for _, verb := range []string{"start", "restart"} {
		code, out := yoke(vars, "service", verb)
		if code != Refused || strings.Count(out, "component.missing") != 2 || !strings.Contains(out, "not asked") {
			t.Errorf("a %s the gate refuses exited %d:\n%s", verb, code, out)
		}
	}
	expectDone(t, vars, "service", "stop")
	if got, _ := os.ReadFile(asked); string(got) != "stop yoke.service\n" {
		t.Errorf("systemctl was asked:\n%s", got)
	}
}
