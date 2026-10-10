// Package enginetest is the container engine an L3 case runs against: the one the environment declares
// in YOKE_TEST_ENGINE, which is rootless Podman where it declares none. Podman is served on a socket of the
// case's own, which a case may take away and bring back; Docker is the daemon the host runs, at
// DOCKER_HOST or its default socket, which no case may take away.
package enginetest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
)

const (
	Podman = "podman"
	Docker = "docker"
)

// registry is the image a Docker run pushes its fixtures through, pinned by the digest of its index.
const registry = "docker.io/library/registry@sha256:a3d8aaa63ed8681a604f1dea0aa03f100d5895b6a58ace528858a7b332415373"

// Under is the engine the environment declares.
func Under() string {
	if os.Getenv("YOKE_TEST_ENGINE") == Docker {
		return Docker
	}
	return Podman
}

// Not skips a case under an engine its description declares it not applicable in; the record computes
// the result from the declaration, and the skip only keeps the run from reporting a failure it was told
// to expect.
func Not(t *testing.T, engine string) {
	t.Helper()
	if Under() == engine {
		t.Skipf("not applicable under %s, as the description declares", engine)
	}
}

// CLI is the engine's own command line, given the arguments.
func CLI(args ...string) *exec.Cmd { return exec.Command(Under(), args...) }

// Service is the engine's API on a socket.
type Service struct {
	t       *testing.T
	Socket  string
	running *exec.Cmd
}

// Serve returns the engine's API, once it answers.
func Serve(t *testing.T) *Service {
	t.Helper()
	if Under() == Docker {
		socket := "/var/run/docker.sock"
		if host, ok := strings.CutPrefix(os.Getenv("DOCKER_HOST"), "unix://"); ok {
			socket = host
		}
		s := &Service{t: t, Socket: socket}
		s.answers()
		return s
	}
	dir, _ := os.MkdirTemp("", "eng-")
	t.Cleanup(func() { os.RemoveAll(dir) })
	s := &Service{t: t, Socket: filepath.Join(dir, "podman.sock")}
	s.Start()
	t.Cleanup(s.Kill)
	return s
}

// Start serves Podman's API on the socket, and returns once the API answers. A socket that exists is not
// yet an engine that answers: on a loaded runner Podman's first answer can take longer than the Core
// waits at its start.
func (s *Service) Start() {
	s.t.Helper()
	if Under() != Podman {
		s.t.Fatalf("%s is the host's daemon, and a case does not start it", Under())
	}
	s.running = exec.Command("podman", "system", "service", "--time=0", "unix://"+s.Socket)
	if err := s.running.Start(); err != nil {
		s.t.Fatalf("the environment declares rootless Podman, and it cannot be run: %v", err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(s.Socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			s.t.Fatal("Podman's API never appeared")
		}
	}
	s.answers()
}

// answers waits for the API to answer. The socket is bound before the service listens on it, and a
// refused connection is retried.
func (s *Service) answers() {
	s.t.Helper()
	for deadline := time.Now().Add(time.Minute); ; time.Sleep(100 * time.Millisecond) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := engine.Reach(ctx, "unix://"+s.Socket)
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("the %s API never answered: %v", Under(), err)
		}
	}
}

// Kill takes Podman's API away as a crash would, leaving what it ran running, and its socket's file
// removed as a restart would remove it.
func (s *Service) Kill() {
	if s.running == nil {
		return
	}
	s.running.Process.Kill()
	s.running.Wait()
	s.running = nil
	os.Remove(s.Socket)
}

// Image builds the Containerfile in dir and returns the image referenced by its digest, held by the
// engine under that digest. Docker's classic store keeps no digest for an image it built or loaded, so
// there the image is pushed to a registry on loopback and pulled back by digest.
func Image(t *testing.T, dir, name string) string {
	t.Helper()
	tag := "localhost/" + name + ":" + strings.ReplaceAll(t.Name(), "/", "-")
	if Under() == Podman {
		run(t, "podman", "build", "-q", "-t", tag, dir)
		t.Cleanup(func() { exec.Command("podman", "rmi", "-f", tag).Run() })
		digest := run(t, "podman", "images", "--digests", "--format", "{{.Digest}}", tag)
		return "localhost/" + name + "@" + digest
	}
	address := loopbackRegistry(t)
	pushed := address + "/" + name + ":latest"
	run(t, "docker", "build", "-q", "-f", filepath.Join(dir, "Containerfile"), "-t", pushed, dir)
	run(t, "docker", "push", "-q", pushed)
	repoDigest := run(t, "docker", "inspect", "--format", "{{index .RepoDigests 0}}", pushed)
	run(t, "docker", "rmi", "-f", pushed)
	run(t, "docker", "pull", "-q", repoDigest)
	t.Cleanup(func() { exec.Command("docker", "rmi", "-f", repoDigest).Run() })
	return repoDigest
}

// loopbackRegistry runs a registry on 127.0.0.1 for the case, and returns its address.
func loopbackRegistry(t *testing.T) string {
	t.Helper()
	id := run(t, "docker", "run", "-d", "-p", "127.0.0.1::5000", registry)
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", id).Run() })
	port := run(t, "docker", "port", id, "5000/tcp")
	address := "localhost:" + port[strings.LastIndex(port, ":")+1:]
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		if exec.Command("curl", "-sf", "http://"+address+"/v2/").Run() == nil {
			return address
		}
		if time.Now().After(deadline) {
			t.Fatal("the registry on loopback never answered")
		}
	}
}

// Labelled are the containers carrying every label given, ended or not, by their full identity.
func Labelled(labels ...string) []string {
	args := []string{"ps", "-aq", "--no-trunc"}
	for _, l := range labels {
		args = append(args, "--filter", "label="+l)
	}
	out, _ := CLI(args...).Output()
	return strings.Fields(string(out))
}

// State is the state of the one container carrying every label given, as the engine names it.
func State(labels ...string) string {
	args := []string{"ps", "-a", "--format", "{{.State}}"}
	for _, l := range labels {
		args = append(args, "--filter", "label="+l)
	}
	out, _ := CLI(args...).Output()
	return strings.TrimSpace(string(out))
}

// Exists says whether the engine holds the container.
func Exists(id string) bool { return CLI("inspect", id).Run() == nil }

// Running says whether the container runs.
func Running(id string) bool {
	out, _ := CLI("inspect", "-f", "{{.State.Running}}", id).Output()
	return strings.TrimSpace(string(out)) == "true"
}

// Remove removes the containers, whatever their state.
func Remove(ids ...string) {
	if len(ids) > 0 {
		CLI(append([]string{"rm", "-f"}, ids...)...).Run()
	}
}

func run(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		var said []byte
		if exit, ok := err.(*exec.ExitError); ok {
			said = exit.Stderr
		}
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, said)
	}
	return strings.TrimSpace(string(out))
}
