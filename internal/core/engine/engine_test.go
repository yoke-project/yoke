package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
)

// fake is an engine's API as each case scripts it: what /version and /info answer, and every path it
// was asked on.
type fake struct {
	version map[string]any
	info    map[string]any

	mu    sync.Mutex
	paths []string
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.paths = append(f.paths, r.URL.Path)
	f.mu.Unlock()
	switch {
	case strings.HasSuffix(r.URL.Path, "/version"):
		json.NewEncoder(w).Encode(f.version)
	case strings.HasSuffix(r.URL.Path, "/info"):
		json.NewEncoder(w).Encode(f.info)
	default:
		http.NotFound(w, r)
	}
}

func (f *fake) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

// short is a directory short enough for a socket path.
func short(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "eng-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// serve binds f on a local socket and returns its address as the configuration names it.
func serve(t *testing.T, f *fake) string {
	t.Helper()
	path := filepath.Join(short(t), "engine.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	s := &http.Server{Handler: f}
	go s.Serve(l)
	t.Cleanup(func() { s.Close() })
	return "unix://" + path
}

func podman(rootless bool) *fake {
	security := []string{"name=seccomp,profile=default"}
	if rootless {
		security = append(security, "name=rootless")
	}
	return &fake{
		version: map[string]any{"Version": "6.1.3", "ApiVersion": "1.44", "MinAPIVersion": "1.24",
			"Components": []map[string]any{{"Name": "Podman Engine"}, {"Name": "Conmon"}}},
		info: map[string]any{"SecurityOptions": security},
	}
}

func docker(rootless bool, min, max string) *fake {
	security := []string{"name=seccomp,profile=builtin", "name=cgroupns"}
	if rootless {
		security = append(security, "name=rootless")
	}
	return &fake{
		version: map[string]any{"Version": "28.5.2", "ApiVersion": max, "MinAPIVersion": min,
			"Components": []map[string]any{{"Name": "Engine"}, {"Name": "containerd"}}},
		info: map[string]any{"SecurityOptions": security},
	}
}

func reach(t *testing.T, address string) (*engine.Engine, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return engine.Reach(ctx, address)
}

// std: yoke:the-engine.01
func TestTheEnginesAddressIsALocalSocket(t *testing.T) {
	if _, err := reach(t, serve(t, podman(true))); err != nil {
		t.Fatalf("a local socket was not reached: %v", err)
	}
	for _, address := range []string{"tcp://127.0.0.1:2375", "/a/bare/path", ""} {
		_, err := reach(t, address)
		var refused *engine.Refused
		if !errors.As(err, &refused) || refused.Address != address {
			t.Errorf("%q was answered %v, not refused naming it", address, err)
		}
	}
}

// std: yoke:the-engine.02
func TestWhichEngineAndWhetherRootlessIsAskedOfIt(t *testing.T) {
	for _, c := range []struct {
		engine   *fake
		kind     string
		rootless bool
		version  string
	}{
		{podman(true), engine.Podman, true, "6.1.3"},
		{docker(false, "1.40", "1.51"), engine.Docker, false, "28.5.2"},
		{docker(true, "1.40", "1.51"), engine.Docker, true, "28.5.2"},
	} {
		e, err := reach(t, serve(t, c.engine))
		if err != nil {
			t.Fatalf("not reached: %v", err)
		}
		if e.Kind != c.kind || e.Rootless != c.rootless || e.Version != c.version {
			t.Errorf("reached as %s rootless=%v %s, want %s rootless=%v %s", e.Kind, e.Rootless, e.Version, c.kind, c.rootless, c.version)
		}
	}
}

// std: yoke:the-engine.03
func TestOneVersionOfTheAPI(t *testing.T) {
	served := podman(true)
	if _, err := reach(t, serve(t, served)); err != nil {
		t.Fatalf("an engine serving 1.24 to 1.44 was not reached: %v", err)
	}
	for _, path := range served.asked() {
		if !strings.HasPrefix(path, "/v"+engine.API+"/") {
			t.Errorf("the engine was asked on %s", path)
		}
	}
	_, err := reach(t, serve(t, docker(false, "1.44", "1.56")))
	var unsupported *engine.Unsupported
	if !errors.As(err, &unsupported) || unsupported.Min != "1.44" || unsupported.Max != "1.56" ||
		!strings.Contains(err.Error(), engine.API) {
		t.Errorf("an engine serving 1.44 to 1.56 was answered %v", err)
	}
}

// std: yoke:the-engine.04
func TestAnEngineThatDoesNotAnswerIsUnreachable(t *testing.T) {
	nothing := "unix://" + filepath.Join(short(t), "nothing.sock")
	silent := filepath.Join(short(t), "silent.sock")
	l, err := net.Listen("unix", silent)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	for _, address := range []string{nothing, "unix://" + silent} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		began := time.Now()
		e, err := engine.Reach(ctx, address)
		cancel()
		var unreachable *engine.Unreachable
		if e != nil || !errors.As(err, &unreachable) || unreachable.Address != address {
			t.Errorf("%s was answered %v %v, not unreachable", address, e, err)
		}
		if waited := time.Since(began); waited > 2*time.Second {
			t.Errorf("%s was waited on for %v", address, waited)
		}
	}
}

// std: yoke:the-engine.05
func TestTheReferenceEngineIsReachedAndRecognised(t *testing.T) {
	stated, err := exec.Command("podman", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		t.Fatalf("the environment declares rootless Podman, and podman cannot be run: %v", err)
	}
	socket := filepath.Join(short(t), "podman.sock")
	service := exec.Command("podman", "system", "service", "--time=0", "unix://"+socket)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.Process.Kill(); service.Wait() })
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Podman's API never appeared")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	e, err := engine.Reach(ctx, "unix://"+socket)
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != engine.Podman || !e.Rootless || e.Version != strings.TrimSpace(string(stated)) {
		t.Errorf("reached as %s rootless=%v %s, and podman states %s", e.Kind, e.Rootless, e.Version, stated)
	}
}
