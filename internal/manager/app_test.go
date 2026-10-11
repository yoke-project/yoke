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
		"yoke.yaml": fmt.Sprintf("model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: %s\ncore: \">=0.0.1-0\"\n"+
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
	junk := filepath.Join(t.TempDir(), "junk.yoke")
	os.WriteFile(junk, []byte("not a package"), 0o644)
	if code, out := yoke(vars, "app", "install", junk); code != Refused {
		t.Errorf("a file that is not a package exited %d:\n%s", code, out)
	}
}
