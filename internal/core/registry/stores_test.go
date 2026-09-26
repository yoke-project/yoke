package registry_test

import (
	"bufio"
	"bytes"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/yoke-project/yoke/internal/core/registry"
	"github.com/yoke-project/yoke/internal/core/trunk"
)

// upToStores is the trunk's first six steps, the stores the last of them.
func upToStores() []trunk.Step {
	steps := trunk.Steps()
	for i, s := range steps {
		if s.Name == "stores" {
			return steps[:i+1]
		}
	}
	panic("the trunk has no step called stores")
}

func service(t *testing.T) (*trunk.State, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "core.yaml")
	document := "state_dir: " + dir + "/state\nruntime_dir: " + dir + "/run\n"
	if err := os.WriteFile(path, []byte(document), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"YOKE_CONFIG": path}
	return &trunk.State{Form: trunk.Service, Env: func(k string) string { return env[k] }, Stderr: &bytes.Buffer{}}, dir
}

// std: yoke:the-registry.01
func TestTheRegistryIsRegistryDBInTheStateDirectory(t *testing.T) {
	st, dir := service(t)
	if err := trunk.Run(st, upToStores()); err != nil {
		t.Fatal(err)
	}
	st.Stop()
	if _, err := os.Stat(filepath.Join(dir, "state", registry.File)); err != nil {
		t.Errorf("the service form's Registry is not in state_dir: %v", err)
	}

	home := t.TempDir()
	env := map[string]string{"XDG_RUNTIME_DIR": home + "/run", "XDG_STATE_HOME": home + "/state"}
	app := &trunk.State{Form: trunk.Application, Name: "bench-a", Env: func(k string) string { return env[k] }, Stderr: &bytes.Buffer{}}
	if err := trunk.Run(app, upToStores()); err != nil {
		t.Fatal(err)
	}
	app.Stop()
	if _, err := os.Stat(filepath.Join(home, "state", "yoke", "instances", "bench-a", "state", registry.File)); err != nil {
		t.Errorf("the application form's Registry is not under its instance's state: %v", err)
	}
}

// std: yoke:the-registry.05
func TestAHigherNumberIsRefusedNamingBothNumbers(t *testing.T) {
	st, dir := service(t)
	os.MkdirAll(filepath.Join(dir, "state"), 0o700)
	path := filepath.Join(dir, "state", registry.File)
	db, _ := sql.Open("sqlite", path)
	if _, err := db.Exec("PRAGMA user_version = 99"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	before, _ := os.ReadFile(path)
	_, err := registry.Open(path)
	if err == nil {
		t.Fatal("a store written by a newer Core was opened")
	}
	said := strings.ReplaceAll(err.Error(), path, "")
	for _, number := range []string{"99", strconv.Itoa(registry.Schema())} {
		if !strings.Contains(said, number) {
			t.Errorf("the refusal %q does not name %s", err, number)
		}
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("the refused file was changed")
	}
	err = trunk.Run(st, trunk.Steps())
	defer st.Stop()
	if err == nil || !strings.Contains(err.Error(), "stores") {
		t.Fatalf("the trunk gave %v, want it stopped at stores", err)
	}
}

// std: yoke:the-registry.14
func TestACoreThatCannotOpenItsRegistryDoesNotStart(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	start := func(state string) (*exec.Cmd, *bufio.Scanner) {
		dir := t.TempDir()
		path := filepath.Join(dir, "core.yaml")
		os.WriteFile(path, []byte("state_dir: "+state+"\nruntime_dir: "+dir+"/run\n"), 0o644)
		command := exec.Command(binary)
		command.Env = append(os.Environ(), "YOKE_CONFIG="+path, "YOKE_COMPOSITION="+dir+"/deployment.yaml")
		out, _ := command.StderrPipe()
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { command.Process.Kill(); command.Wait() })
		return command, bufio.NewScanner(out)
	}

	notADirectory := filepath.Join(t.TempDir(), "state")
	os.WriteFile(notADirectory, nil, 0o600)
	command, lines := start(notADirectory)
	var said []string
	for lines.Scan() {
		said = append(said, lines.Text())
	}
	err := command.Wait()
	all := strings.Join(said, "\n")
	if err == nil || strings.Contains(all, "msg=ready") || !strings.Contains(all, "stores") {
		t.Fatalf("with a state_dir that is a file the Core gave %v and said:\n%s", err, all)
	}

	state := filepath.Join(t.TempDir(), "state")
	_, lines = start(state)
	deadline := time.After(10 * time.Second)
	readyLine := make(chan bool, 1)
	go func() {
		for lines.Scan() {
			if strings.Contains(lines.Text(), "msg=ready") {
				readyLine <- true
				return
			}
		}
		readyLine <- false
	}()
	select {
	case ok := <-readyLine:
		if !ok {
			t.Fatal("the Core exited before it was ready")
		}
	case <-deadline:
		t.Fatal("the Core was not ready within ten seconds")
	}
	if _, err := os.Stat(filepath.Join(state, registry.File)); err != nil {
		t.Fatalf("the ready Core has no Registry: %v", err)
	}
}
