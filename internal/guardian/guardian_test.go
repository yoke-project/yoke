package guardian_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/guardian"
	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/managerconf"
	"github.com/yoke-project/yoke/internal/truststore"
)

// The Core the guardian executes is this test binary in a role, which a file beside its invocations
// names per instance: `beat` claims the instance, says it is alive and stays; `silent` claims it and never
// says so; `exit` fails at once. Each invocation's arguments are appended, one line each.
const witness = "YOKE_TEST_WITNESS"

// child makes this test binary a process a fake Core starts and leaves running; serve makes it a
// container's process that runs until it is ended.
const (
	child = "YOKE_TEST_CHILD"
	serve = "YOKE_TEST_SERVE"
)

// second makes this test binary a second guardian of the account whose runtime directory it names.
const second = "YOKE_TEST_SECOND_GUARDIAN"

func TestMain(m *testing.M) {
	if i := slices.Index(os.Args, "--instance"); i > 0 && i+1 < len(os.Args) {
		os.Exit(fakeCore(os.Args[i+1]))
	}
	if os.Getenv(child) != "" || os.Getenv(serve) != "" {
		// It runs until it is killed: a signal it waits for and is never sent.
		hold := make(chan os.Signal, 1)
		signal.Notify(hold, syscall.SIGUSR2)
		<-hold
		os.Exit(0)
	}
	if runtime := os.Getenv(second); runtime != "" {
		err := guardian.Run(context.Background(), guardian.Options{Runtime: runtime, Instances: os.Getenv("YOKE_TEST_INSTANCES"), Stderr: os.Stderr})
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func fakeCore(name string) int {
	dir := os.Getenv(witness)
	f, _ := os.OpenFile(filepath.Join(dir, name+".args"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	fmt.Fprintln(f, strings.Join(os.Args[1:], "\x00"))
	f.Close()
	role, _ := os.ReadFile(filepath.Join(dir, name+".role"))
	if strings.TrimSpace(string(role)) == "exit" {
		return 1
	}
	runtime := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "yoke")
	if _, err := instance.Claim(filepath.Join(runtime, name), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, "fake core:", err)
		return 1
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM)
	switch r := strings.TrimSpace(string(role)); r {
	case "parent":
		// It starts a child, which stays in its process group, and then behaves as a Core that beats.
		self, _ := os.Executable()
		c := exec.Command(self, "-test.run=^$")
		c.Env = append(os.Environ(), child+"=1")
		if err := c.Start(); err != nil {
			return 1
		}
		os.WriteFile(filepath.Join(dir, name+".child"), []byte(strconv.Itoa(c.Process.Pid)), 0o600)
		alive, err := dialAlive()
		if err != nil || alive.Send(&channel.Beat{Instance: name}) != nil {
			return 1
		}
		<-stop
		return 0
	case "sparse":
		// It says it is alive at once, then every 1.9 s: always within the tolerance, never early.
		alive, err := dialAlive()
		if err != nil || alive.Send(&channel.Beat{Instance: name}) != nil {
			return 1
		}
		for {
			select {
			case <-stop:
				return 0
			case <-time.After(1900 * time.Millisecond):
				if alive.Send(&channel.Beat{Instance: name}) != nil {
					return 1
				}
			}
		}
	case "mute", "retire":
		// It says it is alive once; then a mute Core falls silent and will not stop when asked, and a retiring
		// one says it is stopping, stays silent past the tolerance, and ends.
		if r == "mute" {
			signal.Ignore(syscall.SIGTERM)
		}
		alive, err := dialAlive()
		if err != nil || alive.Send(&channel.Beat{Instance: name}) != nil {
			return 1
		}
		if r == "mute" {
			select {}
		}
		alive.Send(&channel.Beat{Instance: name, Stopping: true})
		time.Sleep(2500 * time.Millisecond)
		alive.CloseAndRecv()
		return 0
	}
	if strings.TrimSpace(string(role)) == "beat" {
		conn, err := grpc.NewClient("unix://"+filepath.Join(runtime, "guardian.sock"), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return 1
		}
		alive, err := channel.NewGuardianClient(conn).Alive(context.Background())
		if err != nil || alive.Send(&channel.Beat{Instance: name}) != nil {
			fmt.Fprintln(os.Stderr, "fake core: no beat:", err)
			return 1
		}
	}
	<-stop
	return 0
}

const program = "#!/bin/sh\nexit 0\n"

// host is one account: its runtime and state directories, an install root holding com.yoke.station
// 2.1.0, and a configuration naming this test binary as the Core.
type host struct {
	t                     *testing.T
	run, state, instances string
	witness               string
	path, runtime         string // what PATH and XDG_RUNTIME_DIR were, for a case the account's systemd runs
	out                   buffer
	o                     guardian.Options
	descriptor            []byte
}

func newHost(t *testing.T, policy string) *host {
	t.Helper()
	run, err := os.MkdirTemp("", "yg")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(run) })
	h := &host{t: t, run: run, state: t.TempDir(), witness: t.TempDir(), path: os.Getenv("PATH"), runtime: os.Getenv("XDG_RUNTIME_DIR")}
	// Nothing on the PATH: the guardian finds no systemd to run a slice in, and confines by the fallback,
	// so no case here empties a slice another run, or the account itself, is using.
	t.Setenv("PATH", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", run)
	t.Setenv("XDG_STATE_HOME", h.state)
	t.Setenv(witness, h.witness)
	h.instances = filepath.Join(h.state, "yoke", "instances")
	apps := filepath.Join(t.TempDir(), "apps")
	bundle := filepath.Join(apps, "com.yoke.station", "2.1.0", "bundle")
	os.MkdirAll(filepath.Join(bundle, "components"), 0o755)
	os.WriteFile(filepath.Join(bundle, "components", "probe"), []byte(program), 0o755)
	h.descriptor = []byte(strings.Replace(fmt.Sprintf(`model: 1
id: com.yoke.station
version: 2.1.0
arch: {{HOSTARCH}}
core: ">=0.0.0-0"
parameters:
  shared:
    label: { type: string, required: true, label: Label }
    site: { type: string, required: false, default: Bench, label: Site }
policy: { startup_window: 5s, stop_window: 2s%s }
units:
  probe: { kind: oneshot, exec: components/probe, digest: "sha256:%x", args: [ "${label}" ] }
`, policy, sha256.Sum256([]byte(program))), "{{HOSTARCH}}", gate.HostArch(), 1))
	os.WriteFile(filepath.Join(bundle, "yoke.yaml"), h.descriptor, 0o644)
	os.WriteFile(filepath.Join(apps, "com.yoke.station", "2.1.0", "install.yaml"), []byte("state: complete\nsealed: false\nshared: true\n"), 0o664)
	self, _ := os.Executable()
	h.o = guardian.Options{Runtime: filepath.Join(run, "yoke"), Instances: h.instances, Stderr: &h.out,
		Config: managerconf.Config{InstallRoot: apps, Core: self, Engine: "unix:///nonexistent/engine.sock", Retention: map[string]string{"bytes": "20MB"}}}
	return h
}

// buffer is what the guardian wrote, readable while it runs.
type buffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (h *host) said() string {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	return h.out.b.String()
}

// declare writes a record, desired as given, with the descriptor's digest pinned unless pin says another.
func (h *host) declare(name, desired, role string, edit func(*instancerecord.Record)) {
	h.t.Helper()
	sum := sha256.Sum256(h.descriptor)
	r := instancerecord.Record{ID: "com.yoke.station", Version: "2.1.0", Name: name, DesiredState: desired, Owner: "davide",
		CreatedAt: time.Now(), DescriptorDigest: fmt.Sprintf("sha256:%x", sum), Parameters: map[string]map[string]string{"shared": {"label": "Bench A"}}}
	if edit != nil {
		edit(&r)
	}
	if err := instancerecord.Create(h.instances, r); err != nil {
		h.t.Fatal(err)
	}
	os.WriteFile(filepath.Join(h.witness, name+".role"), []byte(role), 0o600)
}

// invocations are the argument vectors the Core was executed with for an instance, one per line, each
// argument ended by a NUL.
func (h *host) invocations(name string) []string {
	data, _ := os.ReadFile(filepath.Join(h.witness, name+".args"))
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func (h *host) executed(name string) int {
	data, _ := os.ReadFile(filepath.Join(h.witness, name+".args"))
	return strings.Count(string(data), "\n")
}

// start runs the guardian until it returns, which the result reports.
func (h *host) start(boot bool) <-chan error {
	o := h.o
	o.Boot = boot
	done, finished := make(chan error, 1), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	h.t.Cleanup(func() { cancel(); <-finished })
	go func() { done <- guardian.Run(ctx, o); close(finished) }()
	return done
}

// client waits for the guardian's socket and speaks the channel on it.
func (h *host) client() channel.GuardianClient {
	h.t.Helper()
	sock := filepath.Join(h.o.Runtime, "guardian.sock")
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(sock); err == nil {
			break
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("the guardian never bound its socket:\n%s", h.said())
		}
	}
	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(func() { conn.Close() })
	return channel.NewGuardianClient(conn)
}

// until reads the report until the instance named satisfies holds.
func (h *host) until(c channel.GuardianClient, name, what string, holds func(*channel.Watched) bool) *channel.Watched {
	h.t.Helper()
	var last *channel.Watched
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		r, err := c.Report(context.Background(), &channel.ReportRequest{})
		if err != nil {
			continue
		}
		for _, w := range r.Instances {
			if w.Name == name {
				last = w
				if holds(w) {
					return w
				}
			}
		}
	}
	h.t.Fatalf("%s never held; last %v:\n%s", what, last, h.said())
	return nil
}

func in(state string) func(*channel.Watched) bool {
	return func(w *channel.Watched) bool { return w.State == state }
}

func (h *host) setDesired(name, desired string) {
	h.t.Helper()
	if err := instancerecord.Update(h.instances, name, func(r *instancerecord.Record) error { r.DesiredState = desired; return nil }); err != nil {
		h.t.Fatal(err)
	}
}

func ended(t *testing.T, done <-chan error, within time.Duration) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(within):
		t.Fatalf("the guardian was still running after %v", within)
		return nil
	}
}

// std: yoke:the-guardian.01
func TestOneGuardianPerAccount(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Running, "beat", nil)
	h.start(false)
	c := h.client()
	h.until(c, "bench-a", "bench-a observable", in("observable"))
	info, err := os.Stat(filepath.Join(h.o.Runtime, "guardian.sock"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the socket: %v %v", info, err)
	}
	self, _ := os.Executable()
	cmd := exec.Command(self, "-test.run=^$")
	cmd.Env = append(os.Environ(), second+"="+h.o.Runtime, "YOKE_TEST_INSTANCES="+h.instances)
	said, _ := cmd.Output()
	if want := fmt.Sprintf("already running, as process %d", os.Getpid()); !strings.Contains(string(said), want) {
		t.Errorf("a second guardian said %q, want %q", said, want)
	}
	if _, err := os.Stat(filepath.Join(h.o.Runtime, "guardian.sock")); err != nil {
		t.Errorf("the second removed the first's socket: %v", err)
	}
}

// std: yoke:the-guardian.02
func TestAStartExecutesTheCoreAndIsFinishedAtItsFirstBeat(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Running, "beat", nil)
	before, _ := os.ReadFile(instancerecord.File(h.instances, "bench-a"))
	h.start(false)
	c := h.client()
	w := h.until(c, "bench-a", "bench-a observable", in("observable"))
	if w.Pid == 0 {
		t.Error("the report names no process")
	}
	args := h.invocations("bench-a")[0] + "\x00"
	bundle := filepath.Join(h.o.Config.InstallRoot, "com.yoke.station", "2.1.0", "bundle")
	for _, want := range []string{"--instance\x00bench-a\x00", "--bundle\x00" + bundle + "\x00", "--retention\x00bytes=20MB\x00",
		"--param\x00shared.label=Bench A\x00", "--param\x00shared.site=Bench\x00"} {
		if !strings.Contains(args, want) {
			t.Errorf("the invocation lacks %q: %q", want, args)
		}
	}
	if strings.Contains(args, "--engine") {
		t.Errorf("the engine is named where nothing names an image: %q", args)
	}
	if after, _ := os.ReadFile(instancerecord.File(h.instances, "bench-a")); string(after) != string(before) {
		t.Errorf("the guardian wrote the record:\n%s", after)
	}
}

// std: yoke:the-guardian.03
func TestARefusedStartIsNotRetriedAndSaysWhy(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Running, "beat", func(r *instancerecord.Record) { r.DescriptorDigest = "sha256:" + strings.Repeat("0", 64) })
	h.declare("bench-b", instancerecord.Running, "beat", nil)
	h.start(false)
	c := h.client()
	w := h.until(c, "bench-a", "bench-a refused", in("refused"))
	if !slices.ContainsFunc(w.Findings, func(f string) bool { return strings.HasPrefix(f, "descriptor.digest") }) {
		t.Errorf("the findings: %v", w.Findings)
	}
	if !strings.Contains(h.said(), "descriptor.digest") {
		t.Errorf("the output does not name the finding:\n%s", h.said())
	}
	if _, err := c.Reconcile(context.Background(), &channel.ReconcileRequest{Name: "bench-a"}); err != nil {
		t.Fatal(err)
	}
	h.until(c, "bench-b", "bench-b observable", in("observable"))
	time.Sleep(300 * time.Millisecond)
	h.until(c, "bench-a", "bench-a refused again", in("refused"))
	if n := h.executed("bench-a"); n != 0 {
		t.Errorf("a refused instance was executed %d times", n)
	}
	if r, _ := instancerecord.Read(h.instances, "bench-a"); r.DesiredState != instancerecord.Running {
		t.Errorf("the desired state became %q", r.DesiredState)
	}
}

// std: yoke:the-guardian.04
func TestAFailedStartIsRetriedWithAGrowingWait(t *testing.T) {
	h := newHost(t, ", restart: { backoff: 1s, ceiling: 2s }")
	h.declare("bench-a", instancerecord.Running, "exit", nil)
	done := h.start(false)
	c := h.client()
	var waits []time.Duration
	for attempt := uint32(1); attempt <= 3; attempt++ {
		w := h.until(c, "bench-a", fmt.Sprintf("attempt %d waiting", attempt), func(w *channel.Watched) bool {
			return w.State == "waiting" && w.Attempt == attempt
		})
		waits = append(waits, w.Wait.AsDuration())
	}
	if !slices.Equal(waits, []time.Duration{time.Second, 2 * time.Second, 2 * time.Second}) {
		t.Errorf("the waits were %v", waits)
	}
	if n := h.executed("bench-a"); n < 3 {
		t.Errorf("executed %d times", n)
	}
	select {
	case err := <-done:
		t.Errorf("the guardian exited while an instance was failing to start: %v", err)
	default:
	}
}

// std: yoke:the-guardian.05
func TestStoppingBringsTheCoreDownAndTheGuardianExits(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Running, "beat", nil)
	done := h.start(false)
	c := h.client()
	w := h.until(c, "bench-a", "bench-a observable", in("observable"))
	h.setDesired("bench-a", instancerecord.Stopped)
	before, _ := os.ReadFile(instancerecord.File(h.instances, "bench-a"))
	if _, err := c.Reconcile(context.Background(), &channel.ReconcileRequest{Name: "bench-a"}); err != nil {
		t.Fatal(err)
	}
	if err := ended(t, done, 20*time.Second); err != nil {
		t.Errorf("the guardian exited %v:\n%s", err, h.said())
	}
	if syscall.Kill(int(w.Pid), 0) == nil {
		t.Errorf("the Core %d is still running", w.Pid)
	}
	if _, err := os.Stat(filepath.Join(h.o.Runtime, "guardian.sock")); err == nil {
		t.Error("the socket outlived the guardian")
	}
	if after, _ := os.ReadFile(instancerecord.File(h.instances, "bench-a")); string(after) != string(before) {
		t.Errorf("the guardian wrote the record:\n%s", after)
	}
}

// std: yoke:the-guardian.06
func TestAnEphemeralRecordsTwoEndings(t *testing.T) {
	h := newHost(t, ", restart: { backoff: 1s, ceiling: 1s }")
	ephemeral := func(r *instancerecord.Record) { r.Ephemeral = true }
	h.declare("run-1", instancerecord.Running, "beat", ephemeral)
	h.declare("run-2", instancerecord.Running, "exit", ephemeral)
	h.start(false)
	c := h.client()
	h.until(c, "run-1", "run-1 observable", in("observable"))
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if r, err := instancerecord.Read(h.instances, "run-2"); err == nil && r.Outcome == instancerecord.Failed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run-2 was never marked failed:\n%s", h.said())
		}
	}
	time.Sleep(1500 * time.Millisecond)
	if n := h.executed("run-2"); n != 1 {
		t.Errorf("a failed ephemeral run was executed %d times", n)
	}
	h.setDesired("run-1", instancerecord.Stopped)
	if _, err := c.Reconcile(context.Background(), &channel.ReconcileRequest{Name: "run-1"}); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(h.instances, "run-1")); errors.Is(err, os.ErrNotExist) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run-1's record outlived its orderly stop:\n%s", h.said())
		}
	}
}

// std: yoke:the-guardian.07
func TestTheBootPassBringsUpWhatBothFieldsSay(t *testing.T) {
	h := newHost(t, "")
	h.declare("a", instancerecord.Running, "beat", func(r *instancerecord.Record) { r.Autostart = true })
	h.declare("b", instancerecord.Stopped, "beat", func(r *instancerecord.Record) { r.Autostart = true })
	h.declare("c", instancerecord.Running, "beat", nil)
	h.declare("d", instancerecord.Running, "beat", func(r *instancerecord.Record) { r.Ephemeral = true })
	h.start(true)
	c := h.client()
	h.until(c, "a", "a observable", in("observable"))
	time.Sleep(300 * time.Millisecond)
	for name, want := range map[string]int{"a": 1, "b": 0, "c": 0, "d": 0} {
		if n := h.executed(name); n != want {
			t.Errorf("%s executed %d times, want %d", name, n, want)
		}
	}
}

// std: yoke:the-guardian.08
func TestTheChannelRefusesAnUnknownRecordAndTellsASubscriber(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Running, "beat", nil)
	h.start(false)
	c := h.client()
	h.until(c, "bench-a", "bench-a observable", in("observable"))
	sub, err := c.Subscribe(context.Background(), &channel.SubscribeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	seen := make(chan *channel.Watched, 16)
	go func() {
		for {
			w, err := sub.Recv()
			if err != nil {
				close(seen)
				return
			}
			seen <- w
		}
	}()
	next := func(state string) {
		t.Helper()
		for deadline := time.After(10 * time.Second); ; {
			select {
			case w, open := <-seen:
				if !open {
					t.Fatalf("the subscription ended before bench-a was %s", state)
				}
				if w.Name == "bench-a" && w.State == state {
					return
				}
			case <-deadline:
				t.Fatalf("the subscription never said bench-a was %s", state)
			}
		}
	}
	next("observable")
	_, err = c.Reconcile(context.Background(), &channel.ReconcileRequest{Name: "nobody"})
	if s, _ := status.FromError(err); s.Code() != codes.NotFound || !strings.Contains(s.Message(), "nobody") ||
		!slices.ContainsFunc(s.Details(), func(d any) bool { r, ok := d.(*channel.Refusal); return ok && r.Code == "record.unknown" }) {
		t.Errorf("reconciling nobody: %v", err)
	}
	h.setDesired("bench-a", instancerecord.Stopped)
	c.Reconcile(context.Background(), &channel.ReconcileRequest{Name: "bench-a"})
	next("stopping")
}

// std: yoke:the-guardian.09
func TestABeatIsAcceptedOnlyFromTheProcessTheGuardianExecuted(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Running, "silent", nil)
	h.start(false)
	c := h.client()
	h.until(c, "bench-a", "bench-a starting", in("starting"))
	alive, err := c.Alive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	alive.Send(&channel.Beat{Instance: "bench-a"})
	_, err = alive.CloseAndRecv()
	if s, _ := status.FromError(err); s.Code() != codes.PermissionDenied || !strings.Contains(s.Message(), "bench-a") {
		t.Errorf("a beat from another process: %v", err)
	}
	h.until(c, "bench-a", "bench-a still starting", in("starting"))
}

// dialAlive opens a fake Core's liveness stream to the account's guardian.
func dialAlive() (channel.Guardian_AliveClient, error) {
	sock := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "yoke", "guardian.sock")
	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return channel.NewGuardianClient(conn).Alive(context.Background())
}

// delegate gives the guardian the account's own systemd, for a case that runs in a slice: the PATH and the
// runtime directory it reaches systemd through. The case names its instance so no other run uses it.
func (h *host) delegate() {
	h.t.Setenv("PATH", h.path)
	h.t.Setenv("XDG_RUNTIME_DIR", h.runtime)
}

// std: yoke:the-guardian.11
func TestAStartIsCheckedAgainstTheDigestsAndNeverTheTrustStore(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Stopped, "beat", nil)
	t.Setenv(truststore.Tighten, "1")
	rec, err := instancerecord.Read(h.instances, "bench-a")
	if err != nil {
		t.Fatal(err)
	}
	if gated := guardian.GateStart(h.o, rec, false); gated.Refused() {
		t.Errorf("the start of an unsigned installed version was refused under a policy requiring signatures: %v", gated.Findings)
	}
}

// std: yoke:the-guardian.12
func TestStoredValuesAreCheckedAgainAtEveryStart(t *testing.T) {
	h := newHost(t, "")
	h.declare("bench-a", instancerecord.Stopped, "beat", func(r *instancerecord.Record) {
		r.Parameters = map[string]map[string]string{"shared": {"label": "Bench A", "operator": "nobody declared me"}}
	})
	h.declare("bench-b", instancerecord.Stopped, "beat", func(r *instancerecord.Record) {
		r.Parameters = map[string]map[string]string{"shared": {"site": "Bench"}}
	})
	for name, parameter := range map[string]string{"bench-a": "operator", "bench-b": "label"} {
		rec, err := instancerecord.Read(h.instances, name)
		if err != nil {
			t.Fatal(err)
		}
		gated := guardian.GateStart(h.o, rec, false)
		if !gated.Refused() || !slices.ContainsFunc(gated.Findings, func(f gate.Finding) bool { return strings.Contains(f.Location+" "+f.Message, parameter) }) {
			t.Errorf("%s's hand-edited values were not refused naming %s: %v", name, parameter, gated.Findings)
		}
	}
}
