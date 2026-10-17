package manager

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/compose"
)

// std: yoke:the-manager-command-line.04
func TestYokeAppInstallsListsAndShows(t *testing.T) {
	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	core := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", core, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	vars := conf(t, fmt.Sprintf("install_root: %s\ncore: %s\n", apps, core))

	project := t.TempDir()
	program := "#!/bin/sh\n"
	for name, body := range map[string]string{
		"yoke.yaml": fmt.Sprintf("model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: %s\ncore: \">=0.0.0-0\"\n"+
			"units:\n  setup: { kind: oneshot, exec: components/setup, digest: \"sha256:%x\" }\n", compose.HostArch(), sha256.Sum256([]byte(program))),
		"components/setup": program, "manifests/.keep": "",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(project, name)), 0o755)
		os.WriteFile(filepath.Join(project, name), []byte(body), 0o755)
	}
	pkg, report, err := compose.Build(project, compose.BuildOptions{Out: t.TempDir()})
	if err != nil {
		t.Fatalf("build: %v %v", err, report.Findings)
	}

	code, out := yoke(vars, "app", "install", pkg)
	if code != Done || !strings.Contains(out, filepath.Join(apps, "com.yoke.station", "2.1.0")) {
		t.Fatalf("the first install exited %d:\n%s", code, out)
	}
	code, out = yoke(vars, "app", "install", pkg)
	if code != Done || !strings.Contains(out, "already") || !strings.Contains(out, "nothing") {
		t.Errorf("the second install exited %d:\n%s", code, out)
	}
	code, out = yoke(vars, "app", "list")
	if code != Done || !strings.Contains(squash(out), "com.yoke.station 2.1.0 complete") {
		t.Errorf("the list exited %d:\n%s", code, out)
	}
	code, out = yoke(vars, "app", "show", "com.yoke.station")
	if code != Done || !strings.Contains(out, "2.1.0") || !strings.Contains(out, "unsigned") || !strings.Contains(out, "sha256:") {
		t.Errorf("the show exited %d:\n%s", code, out)
	}
	// An unsigned package was verified by no key, and is not said to have been verified by one.
	if strings.Contains(out, "verified by unsigned") {
		t.Errorf("the show names a key called unsigned:\n%s", out)
	}
	junk := filepath.Join(t.TempDir(), "junk.yoke")
	os.WriteFile(junk, []byte("not a package"), 0o644)
	if code, out := yoke(vars, "app", "install", junk); code != Refused {
		t.Errorf("a file that is not a package exited %d:\n%s", code, out)
	}
}

// std: yoke:the-manager-command-line.07
func TestYokeAppRemovesAndReclaims(t *testing.T) {
	apps := filepath.Join(t.TempDir(), "apps")
	for _, ver := range []string{"2.0.0", "2.1.0", "2.2.0"} {
		dir := filepath.Join(apps, "com.yoke.station", ver)
		os.MkdirAll(filepath.Join(dir, "bundle", "components"), 0o775)
		os.WriteFile(filepath.Join(dir, "bundle", "components", "setup"), []byte(setupProgram), 0o755)
		os.WriteFile(filepath.Join(dir, "bundle", "yoke.yaml"), []byte("model: 1\nid: com.yoke.station\nversion: "+ver+
			"\narch: "+compose.HostArch()+"\ncore: \">=0.1\"\nunits:\n  setup: { kind: oneshot, exec: components/setup, digest: \""+
			setupDigest+"\" }\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "install.yaml"), []byte("state: complete\nsealed: false\nshared: true\n"), 0o664)
	}
	vars := conf(t, "install_root: "+apps+"\nengine: unix:///nonexistent/engine.sock\n")
	vars["XDG_STATE_HOME"] = t.TempDir()
	vars["XDG_RUNTIME_DIR"] = t.TempDir()
	if code, out := yoke(vars, "instance", "create", "com.yoke.station", "--name", "bench-a", "--version", "2.0.0"); code != Done {
		t.Fatalf("create: %s", out)
	}
	there := func(ver string) bool {
		_, err := os.Stat(filepath.Join(apps, "com.yoke.station", ver))
		return err == nil
	}
	if code, out := yoke(vars, "app", "remove", "com.yoke.station"); code != Refused || !strings.Contains(out, "bench-a") ||
		!there("2.0.0") || !there("2.1.0") || !there("2.2.0") {
		t.Errorf("removing what bench-a points at exited %d:\n%s", code, out)
	}
	if code, out := yoke(vars, "app", "prune", "com.yoke.station"); code != Done || !strings.Contains(out, "removed com.yoke.station 2.1.0") ||
		!strings.Contains(out, "images were not considered") || there("2.1.0") || !there("2.0.0") || !there("2.2.0") {
		t.Errorf("the prune exited %d:\n%s", code, out)
	}
	if code, out := yoke(vars, "app", "remove", "com.yoke.station", "--version", "2.2.0"); code != Done ||
		!strings.Contains(out, "removed com.yoke.station 2.2.0") || there("2.2.0") {
		t.Errorf("removing 2.2.0 exited %d:\n%s", code, out)
	}
	if code, out := yoke(vars, "app", "prune", "com.yoke.other"); code != Refused {
		t.Errorf("pruning an application not installed exited %d:\n%s", code, out)
	}
}
