package main_test

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	grpcpeer "google.golang.org/grpc/peer"

	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
	"github.com/yoke-project/yoke/internal/core/peer"
	"github.com/yoke-project/yoke/internal/guardian/channel"
)

// account is an account's two directories, each of the test's own; the runtime one is short, since a
// socket path has a ceiling.
func account(t *testing.T) []string {
	t.Helper()
	run, err := os.MkdirTemp("", "ya")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	return append(os.Environ(), "XDG_RUNTIME_DIR="+run, "XDG_STATE_HOME="+t.TempDir())
}

// from is the value env gives key: the last, as a process executed with it sees.
func from(env []string, key string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], key+"="); ok {
			return v
		}
	}
	return ""
}

// probe is the unit .02 runs: it writes each argument on a line of its own to the file OUT names.
const probe = "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$OUT\"\n"

// bundle writes a bundle whose descriptor declares the unit probe, with extra appended to the descriptor.
func bundle(t *testing.T, extra string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	os.MkdirAll(filepath.Join(dir, "components"), 0o755)
	os.WriteFile(filepath.Join(dir, "components", "probe"), []byte(probe), 0o755)
	descriptor := fmt.Sprintf(`model: 1
id: com.yoke.station
version: 2.1.0
arch: aarch64
core: ">=0.0.0-0"
parameters:
  shared:
    label: { type: string, required: true, label: Label }
    site: { type: string, required: true, label: Site }
    out: { type: string, required: true, label: Out }
  probe:
    label: { type: string, required: false, default: none, label: Label }
units:
  probe:
    kind: oneshot
    exec: components/probe
    digest: "sha256:%x"
    args: [ "${label}", "${site}" ]
    env: { OUT: "${out}" }
%s`, sha256.Sum256([]byte(probe)), extra)
	os.WriteFile(filepath.Join(dir, "yoke.yaml"), []byte(descriptor), 0o644)
	return dir
}

// values are the resolved parameter values .02 executes the Core with, writing to out.
func values(out string) []string {
	return []string{"--param", "shared.label=Shared", "--param", "shared.site=Bench A", "--param", "probe.label=Inner",
		"--param", "shared.out=" + out}
}

// application starts the Core with the arguments given and reads its lines.
func application(t *testing.T, binary string, env []string, args ...string) (*exec.Cmd, *lines) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = env
	out, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	l := &lines{t: t, c: make(chan string)}
	go func() {
		s := bufio.NewScanner(out)
		for s.Scan() {
			l.c <- s.Text()
		}
		close(l.c)
	}()
	return cmd, l
}

type lines struct {
	t    *testing.T
	c    chan string
	said []string
}

// until reads lines until one holds every part, failing if the Core ends or the time passes first.
func (l *lines) until(within time.Duration, parts ...string) {
	l.t.Helper()
	deadline := time.After(within)
	for {
		select {
		case line, open := <-l.c:
			if !open {
				l.t.Fatalf("the Core ended before it said %q:\n%s", parts, strings.Join(l.said, "\n"))
			}
			l.said = append(l.said, line)
			held := true
			for _, p := range parts {
				held = held && strings.Contains(line, p)
			}
			if held {
				return
			}
		case <-deadline:
			l.t.Fatalf("the Core did not say %q within %v:\n%s", parts, within, strings.Join(l.said, "\n"))
		}
	}
}

// rest reads every line left.
func (l *lines) rest() string {
	for line := range l.c {
		l.said = append(l.said, line)
	}
	return strings.Join(l.said, "\n")
}

// std: yoke:the-application-invocation.01
func TestAnInvocationThatDoesNotHoldIsRefusedBeforeAnythingIsCreated(t *testing.T) {
	binary := build(t)
	env := account(t)
	b := bundle(t, "")
	root := filepath.Join(from(env, "XDG_RUNTIME_DIR"), "yoke", "bench-a")
	for _, c := range []struct {
		args []string
		says string
	}{
		{[]string{"--instance", "bench-a", "--bundle", b, "--composition", "/etc/yoke/bench.yaml"}, "--composition"},
		{[]string{"--instance", "bench-a"}, "--bundle"},
		{[]string{"--instance", "bench-a", "--bundle", b, "--param", "shared.label"}, "shared.label"},
		{[]string{"--instance", "bench-a", "--bundle", b, "--retention", "bytes=lots"}, "lots"},
		{[]string{"--instance", "bench-a", "--bundle", b, "--retention", "size=1MB"}, "size"},
	} {
		cmd := exec.Command(binary, c.args...)
		cmd.Env = env
		said, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(said), c.says) {
			t.Errorf("%v: %v, saying %s", c.args, err, said)
		}
		if _, err := os.Stat(root); err == nil {
			t.Fatalf("%v created %s", c.args, root)
		}
	}
}

// std: yoke:the-application-invocation.02
func TestACoreExecutedOnABundleRunsWhatTheDescriptorDeclares(t *testing.T) {
	binary := build(t)
	env := account(t)
	out := filepath.Join(t.TempDir(), "probe.out")
	cmd, l := application(t, binary, env, append([]string{"--instance", "bench-a", "--bundle", bundle(t, "")}, values(out)...)...)
	l.until(10*time.Second, "msg=ready")
	l.until(30*time.Second, "subject=unit:probe#1", "to=Completed")
	if written, err := os.ReadFile(out); err != nil || string(written) != "Inner\nBench A\n" {
		t.Errorf("the unit wrote %q: %v", written, err)
	}
	stamp, err := os.ReadFile(filepath.Join(from(env, "XDG_STATE_HOME"), "yoke", "instances", "bench-a", "state", "version"))
	if err != nil || strings.TrimSpace(string(stamp)) != "2.1.0" {
		t.Errorf("the version stamp says %q: %v", stamp, err)
	}
	cmd.Process.Signal(syscall.SIGTERM)
	said := l.rest()
	if err := cmd.Wait(); err != nil {
		t.Errorf("the Core exited %v:\n%s", err, said)
	}
}

// std: yoke:the-application-invocation.03
func TestADescriptorOrAValueTheGateRefusesKeepsTheCoreFromStarting(t *testing.T) {
	binary := build(t)
	env := account(t)
	out := filepath.Join(t.TempDir(), "probe.out")
	root := filepath.Join(from(env, "XDG_RUNTIME_DIR"), "yoke", "bench-a")
	for _, args := range [][]string{
		append([]string{"--instance", "bench-a", "--bundle", bundle(t, "colour: red\n")}, values(out)...),
		append([]string{"--instance", "bench-a", "--bundle", bundle(t, ""), "--param", "probe.colour=red"}, values(out)...),
	} {
		cmd := exec.Command(binary, args...)
		cmd.Env = env
		said, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || !strings.Contains(string(said), "key.unknown") {
			t.Errorf("%v: %v, saying %s", args, err, said)
		}
		if _, err := os.Stat(filepath.Join(root, "operator.sock")); err == nil {
			t.Errorf("%v bound a channel", args)
		}
	}
}

// std: yoke:the-application-invocation.04
func TestTheDescriptorIsReadOnceAndClosed(t *testing.T) {
	binary := build(t)
	env := account(t)
	b := bundle(t, "")
	descriptor := filepath.Join(b, "yoke.yaml")
	cmd, l := application(t, binary, env, append([]string{"--instance", "bench-a", "--bundle", b}, values(filepath.Join(t.TempDir(), "out"))...)...)
	l.until(10*time.Second, "msg=ready")
	fds, err := os.ReadDir("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, fd := range fds {
		if target, _ := os.Readlink("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/fd/" + fd.Name()); target == descriptor {
			t.Errorf("the process holds the descriptor open, as %s", fd.Name())
		}
	}
	os.WriteFile(descriptor, []byte("model: 1\ncolour: red\n"), 0o644)
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("the Core is no longer serving after its descriptor was replaced: %v", err)
	}
	cmd.Process.Signal(syscall.SIGTERM)
	said := l.rest()
	if err := cmd.Wait(); err != nil {
		t.Errorf("the Core exited %v:\n%s", err, said)
	}
}

// std: yoke:the-application-invocation.05
func TestAnUnverifiedRunSaysSoAboutItself(t *testing.T) {
	binary := build(t)
	for _, unverified := range []bool{true, false} {
		env := account(t)
		args := append([]string{"--instance", "bench-a", "--bundle", bundle(t, "")}, values(filepath.Join(t.TempDir(), "out"))...)
		if unverified {
			args = append(args, "--unverified")
		}
		cmd, l := application(t, binary, env, args...)
		l.until(10*time.Second, "msg=ready")
		said := strings.Join(l.said, "\n")
		if strings.Contains(said, "bundle.unverified") != unverified {
			t.Errorf("unverified %t, and the Core said:\n%s", unverified, said)
		}
		cmd.Process.Signal(syscall.SIGTERM)
		l.rest()
		cmd.Wait()
	}
}

// std: yoke:the-application-invocation.06
func TestTheEngineTheInvocationNamesIsTheOneTheCoreClears(t *testing.T) {
	enginetest.Not(t, enginetest.Docker)
	ref, archive := enginetest.OCIArchive(t, "yoke-invocation")
	if said, err := enginetest.CLI("load", "-i", archive).CombinedOutput(); err != nil {
		t.Fatalf("load: %v\n%s", err, said)
	}
	created, err := enginetest.CLI("create", "--label", "dev.yoke-project.instance=bench-c", "--label", "dev.yoke-project.unit=keeper",
		"--label", "dev.yoke-project.incarnation=1", ref, "/hello").Output()
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	left := strings.TrimSpace(string(created))
	t.Cleanup(func() { enginetest.Remove(left) })
	s := enginetest.Serve(t)
	b := bundle(t, "  keeper:\n    kind: oneshot\n    image: "+ref+"\n    autostart: false\n")
	cmd, l := application(t, build(t), account(t), append([]string{"--instance", "bench-c", "--bundle", b, "--engine", "unix://" + s.Socket},
		values(filepath.Join(t.TempDir(), "out"))...)...)
	l.until(time.Minute, left[:12])
	l.until(time.Minute, "msg=ready")
	if enginetest.Exists(left) {
		t.Errorf("the inherited container %s still exists", left)
	}
	cmd.Process.Signal(syscall.SIGTERM)
	l.rest()
	cmd.Wait()
}

// listener is a guardian's channel that only hears beats, each with the process it came from.
type listener struct {
	channel.UnimplementedGuardianServer
	beats chan string
}

func (l *listener) Alive(stream channel.Guardian_AliveServer) error {
	pid := int32(0)
	if p, ok := grpcpeer.FromContext(stream.Context()); ok {
		if info, ok := p.AuthInfo.(peer.Info); ok {
			pid = info.PID
		}
	}
	for {
		b, err := stream.Recv()
		if err != nil {
			return nil
		}
		l.beats <- fmt.Sprintf("%s %d %t", b.GetInstance(), pid, b.GetStopping())
	}
}

// std: yoke:the-application-invocation.07
func TestACoreUnderAGuardianSaysItIsAliveOnceReady(t *testing.T) {
	binary := build(t)
	env := account(t)
	dir := filepath.Join(from(env, "XDG_RUNTIME_DIR"), "yoke")
	os.MkdirAll(dir, 0o700)
	sock, err := net.Listen("unix", filepath.Join(dir, "guardian.sock"))
	if err != nil {
		t.Fatal(err)
	}
	l := &listener{beats: make(chan string, 64)}
	server := grpc.NewServer(grpc.Creds(peer.Credentials{}))
	channel.RegisterGuardianServer(server, l)
	go server.Serve(sock)
	t.Cleanup(server.Stop)

	args := append([]string{"--instance", "bench-a", "--bundle", bundle(t, "")}, values(filepath.Join(t.TempDir(), "out"))...)
	cmd, lines := application(t, binary, env, args...)
	lines.until(10*time.Second, "msg=ready")
	select {
	case b := <-l.beats:
		if want := fmt.Sprintf("bench-a %d false", cmd.Process.Pid); b != want {
			t.Errorf("the beat was %q, want %q", b, want)
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Signal(syscall.SIGTERM)
		t.Fatalf("no beat arrived:\n%s", lines.rest())
	}
	cmd.Process.Signal(syscall.SIGTERM)
	lines.rest()
	cmd.Wait()

	server.Stop()
	os.Remove(filepath.Join(dir, "guardian.sock"))
	alone, lines := application(t, binary, env, args...)
	lines.until(10*time.Second, "msg=ready")
	alone.Process.Signal(syscall.SIGTERM)
	lines.rest()
	alone.Wait()
}

// std: yoke:the-application-invocation.08
func TestACoreSaysItIsAliveAtItsIntervalAndWhenItStops(t *testing.T) {
	binary := build(t)
	env := account(t)
	dir := filepath.Join(from(env, "XDG_RUNTIME_DIR"), "yoke")
	os.MkdirAll(dir, 0o700)
	sock, err := net.Listen("unix", filepath.Join(dir, "guardian.sock"))
	if err != nil {
		t.Fatal(err)
	}
	l := &listener{beats: make(chan string, 64)}
	server := grpc.NewServer(grpc.Creds(peer.Credentials{}))
	channel.RegisterGuardianServer(server, l)
	go server.Serve(sock)
	t.Cleanup(server.Stop)

	b := bundle(t, "policy: { heartbeat: { interval: 1s } }\n")
	cmd, lines := application(t, binary, env, append([]string{"--instance", "bench-a", "--bundle", b}, values(filepath.Join(t.TempDir(), "out"))...)...)
	lines.until(10*time.Second, "msg=ready")
	time.Sleep(3200 * time.Millisecond)
	cmd.Process.Signal(syscall.SIGTERM)
	lines.rest()
	cmd.Wait()
	var beats []string
	for len(l.beats) > 0 {
		beats = append(beats, <-l.beats)
	}
	alive := fmt.Sprintf("bench-a %d false", cmd.Process.Pid)
	if len(beats) < 4 || strings.Count(strings.Join(beats, "\n"), alive) < 3 || beats[len(beats)-1] != fmt.Sprintf("bench-a %d true", cmd.Process.Pid) {
		t.Errorf("the beats were %q", beats)
	}
}

// std: yoke:the-application-invocation.09
func TestACoreSaysWhetherItIsConfined(t *testing.T) {
	binary := build(t)
	env := account(t)
	cmd, lines := application(t, binary, env, append([]string{"--instance", "bench-a", "--bundle", bundle(t, "")}, values(filepath.Join(t.TempDir(), "out"))...)...)
	lines.until(10*time.Second, "msg=ready")
	if said := strings.Join(lines.said, "\n"); !strings.Contains(said, "code=confinement.no_cgroup_delegation") {
		t.Errorf("a Core outside its slice did not say so:\n%s", said)
	}
	cmd.Process.Signal(syscall.SIGTERM)
	lines.rest()
	cmd.Wait()
}
