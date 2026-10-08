package engine_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
)

// role says which part a copy of this test binary plays; the fixture image runs it as the probe.
const role = "TEST_UNIT_ROLE"

func TestMain(m *testing.M) {
	if os.Getenv(role) == "probe" {
		os.Exit(probe())
	}
	os.Exit(m.Run())
}

// owned says whether the file is the launching account's.
func owned(info os.FileInfo) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

// request is one thing a scripted engine was asked.
type request struct {
	method, path string
	query        url.Values
	body         map[string]any
}

// scripted is an engine that answers /version and /info as identity says, and the container operations
// as each case scripts them, recording every request.
type scripted struct {
	identity *fake
	absent   bool             // no image is held
	output   []string         // "1 line" or "2 line": what the container writes once started
	events   []map[string]any // what the event stream carries before it ends
	started  chan struct{}    // closed when the container is started
	mu       sync.Mutex
	asked    []request
}

func (s *scripted) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/version") || strings.HasSuffix(path, "/info") {
		s.identity.ServeHTTP(w, r)
		return
	}
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.asked = append(s.asked, request{r.Method, path, r.URL.Query(), body})
	s.mu.Unlock()
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/containers/create"):
		if s.absent {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(w, `{"message":"no such image: %s"}`, body["Image"])
			return
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"Id":"c0ffee"}`)
	case strings.HasSuffix(path, "/attach"):
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		buf.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n")
		buf.Flush()
		<-s.started
		for _, o := range s.output {
			stream, line, _ := strings.Cut(o, " ")
			header := make([]byte, 8)
			header[0] = map[string]byte{"1": 1, "2": 2}[stream]
			binary.BigEndian.PutUint32(header[4:], uint32(len(line)+1))
			conn.Write(append(header, line+"\n"...))
		}
	case strings.HasSuffix(path, "/start"):
		close(s.started)
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(path, "/kill"), r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	case strings.HasSuffix(path, "/events"):
		for _, e := range s.events {
			json.NewEncoder(w).Encode(e)
		}
	default:
		http.NotFound(w, r)
	}
}

func (s *scripted) requests() []request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]request(nil), s.asked...)
}

func reached(t *testing.T, s *scripted) *engine.Engine {
	t.Helper()
	if s.started == nil {
		s.started = make(chan struct{})
	}
	e, err := reach(t, serve(t, s))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func launch() engine.Launch {
	return engine.Launch{Image: "localhost/fixture@sha256:" + strings.Repeat("ab", 32), Args: []string{"--once", "two words"},
		Env: []string{"YOKE_UNIT=calibrate", "DECLARED=yes"}, Directory: "/run/yk", Instance: "bench", Unit: "calibrate",
		Incarnation: 4, UID: 1000, GID: 1000}
}

func ctx(t *testing.T) context.Context { return bounded(t, 5*time.Second) }

// patient is the bound for a real engine, which can be slow on its first answers when the host is busy.
func patient(t *testing.T) context.Context { return bounded(t, time.Minute) }

func bounded(t *testing.T, d time.Duration) context.Context {
	c, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return c
}

// std: yoke:the-containers.01
func TestAContainerIsCreatedFromItsDigestWithNothingElse(t *testing.T) {
	s := &scripted{identity: podman(true)}
	if _, err := reached(t, s).Create(ctx(t), launch()); err != nil {
		t.Fatal(err)
	}
	created := s.requests()[0].body
	l := launch()
	host, _ := created["HostConfig"].(map[string]any)
	labels, _ := created["Labels"].(map[string]any)
	want := map[string]any{"dev.yoke-project.instance": "bench", "dev.yoke-project.unit": "calibrate", "dev.yoke-project.incarnation": "4"}
	if created["Image"] != l.Image || fmt.Sprint(created["Cmd"]) != fmt.Sprint([]any{"--once", "two words"}) ||
		fmt.Sprint(created["Env"]) != fmt.Sprint([]any{"YOKE_UNIT=calibrate", "DECLARED=yes"}) || fmt.Sprint(labels) != fmt.Sprint(want) {
		t.Errorf("created %v", created)
	}
	if host["NetworkMode"] != "none" || fmt.Sprint(host["Binds"]) != "[/run/yk:/run/yk]" || host["CgroupParent"] != "yoke-bench.slice" ||
		host["PortBindings"] != nil || host["Mounts"] != nil || created["ExposedPorts"] != nil {
		t.Errorf("the host configuration is %v", host)
	}
}

// std: yoke:the-containers.02
func TestTheIdentityIsHeldConstantPerEngineAndMode(t *testing.T) {
	asRoot := podman(false)
	for _, c := range []struct {
		name         string
		identity     *fake
		user, userns string
	}{
		{"podman rootless", podman(true), "", "keep-id"},
		{"podman as root", asRoot, "1000:1000", ""},
		{"docker through a daemon", docker(false, "1.40", "1.51"), "1000:1000", ""},
		{"docker rootless", docker(true, "1.40", "1.51"), "0:0", ""},
	} {
		s := &scripted{identity: c.identity}
		if _, err := reached(t, s).Create(ctx(t), launch()); err != nil {
			t.Fatal(err)
		}
		created := s.requests()[0].body
		host, _ := created["HostConfig"].(map[string]any)
		user, _ := created["User"].(string)
		userns, _ := host["UsernsMode"].(string)
		if user != c.user || userns != c.userns {
			t.Errorf("%s: user %q userns %q, want %q %q", c.name, user, userns, c.user, c.userns)
		}
	}
}

// std: yoke:the-containers.03
func TestAnImageTheEngineDoesNotHoldIsAbsent(t *testing.T) {
	s := &scripted{identity: podman(true), absent: true}
	_, err := reached(t, s).Create(ctx(t), launch())
	var absent *engine.Absent
	if !errors.As(err, &absent) || absent.Image != launch().Image {
		t.Errorf("an absent image was answered %v", err)
	}
	for _, r := range s.requests() {
		if !strings.HasSuffix(r.path, "/containers/create") {
			t.Errorf("the engine was also asked %s %s", r.method, r.path)
		}
	}
}

// std: yoke:the-containers.04
func TestTheOutputIsAttachedBeforeTheStart(t *testing.T) {
	s := &scripted{identity: podman(true), output: []string{"1 first", "2 trouble", "1 second"}}
	e := reached(t, s)
	var stdout, stderr bytes.Buffer
	done, err := e.Attach(ctx(t), "c0ffee", &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(ctx(t), "c0ffee"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the attachment never ended")
	}
	if stdout.String() != "first\nsecond\n" || stderr.String() != "trouble\n" {
		t.Errorf("the output was %q and the error stream %q", stdout.String(), stderr.String())
	}
	paths := []string{}
	for _, r := range s.requests() {
		paths = append(paths, filepath.Base(r.path))
	}
	if strings.Join(paths, " ") != "attach start" {
		t.Errorf("the engine was asked %v", paths)
	}
	if q := s.requests()[0].query; q.Get("stream") != "1" || q.Get("stdout") != "1" || q.Get("stderr") != "1" {
		t.Errorf("the attachment asked %v", q)
	}
}

// std: yoke:the-containers.05
func TestAContainerIsSignalledAndRemoved(t *testing.T) {
	s := &scripted{identity: podman(true)}
	e := reached(t, s)
	for _, signal := range []string{"SIGTERM", "SIGKILL"} {
		if err := e.Signal(ctx(t), "c0ffee", signal); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Remove(ctx(t), "c0ffee"); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, r := range s.requests() {
		got = append(got, r.method+" "+strings.TrimPrefix(r.path, "/v"+engine.API)+" "+r.query.Get("signal")+r.query.Get("force")+r.query.Get("v"))
	}
	want := []string{"POST /containers/c0ffee/kill SIGTERM", "POST /containers/c0ffee/kill SIGKILL", "DELETE /containers/c0ffee 11"}
	if !slices.Equal(got, want) {
		t.Errorf("the engine was asked %q", got)
	}
}

// std: yoke:the-containers.06
func TestTheEventsOfThisInstancesContainers(t *testing.T) {
	s := &scripted{identity: podman(true), events: []map[string]any{
		{"Type": "container", "Action": "start", "id": "c0ffee", "Actor": map[string]any{"ID": "c0ffee", "Attributes": map[string]any{}}},
		{"Type": "container", "Action": "die", "id": "c0ffee", "Actor": map[string]any{"ID": "c0ffee", "Attributes": map[string]any{"exitCode": "3"}}},
	}}
	events, err := reached(t, s).Events(ctx(t), "bench")
	if err != nil {
		t.Fatal(err)
	}
	var got []engine.Event
	for e := range events {
		got = append(got, e)
	}
	if len(got) != 2 || got[1].Action != "die" || got[1].ID != "c0ffee" || got[1].ExitCode != 3 {
		t.Errorf("the events were %+v", got)
	}
	var filters map[string][]string
	json.Unmarshal([]byte(s.requests()[0].query.Get("filters")), &filters)
	if fmt.Sprint(filters["label"]) != "[dev.yoke-project.instance=bench]" || fmt.Sprint(filters["type"]) != "[container]" || len(filters) != 2 {
		t.Errorf("the events were asked for with %v", filters)
	}
}

// std: yoke:the-containers.07
func TestAnInstancesSliceIsNamedAfterIt(t *testing.T) {
	for instance, want := range map[string]string{"yoke": "yoke-yoke.slice", "bench-a": `yoke-bench\x2da.slice`, "lab.2": "yoke-lab.2.slice"} {
		if got := engine.Slice(instance); got != want {
			t.Errorf("%s's slice is %s, want %s", instance, got, want)
		}
	}
}

// std: yoke:the-containers.08
func TestUnderRootlessPodmanAContainerRunsAsTheLauncher(t *testing.T) {
	image := fixtureImage(t)
	e := podmanService(t)
	instance := "l3-" + strconv.Itoa(os.Getpid())
	events, err := e.Events(patient(t), instance)
	if err != nil {
		t.Fatal(err)
	}
	dir := short(t)
	id, err := e.Create(patient(t), engine.Launch{Image: image, Env: []string{"WRITE=" + filepath.Join(dir, "written")}, Directory: dir,
		Instance: instance, Unit: "probe", Incarnation: 1, UID: os.Getuid(), GID: os.Getgid()})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	attached, err := e.Attach(patient(t), id, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Start(patient(t), id); err != nil {
		t.Fatal(err)
	}
	status := -1
	for ev := range events {
		if ev.ID == id && ev.Action == "die" {
			status = ev.ExitCode
			break
		}
	}
	<-attached
	if err := e.Remove(patient(t), id); err != nil {
		t.Error(err)
	}
	said := stdout.String()
	if !strings.Contains(said, fmt.Sprintf("uid=%d gid=%d", os.Getuid(), os.Getgid())) || !strings.Contains(said, "interfaces=lo\n") {
		t.Errorf("the container said %q, and on its error stream %q", said, stderr.String())
	}
	if info, err := os.Stat(filepath.Join(dir, "written")); err != nil || !owned(info) {
		t.Errorf("the file the container wrote is %v %v", info, err)
	}
	if status != 3 {
		t.Errorf("the container ended with %d", status)
	}
	if left, _ := exec.Command("podman", "ps", "-aq", "--filter", "label=dev.yoke-project.instance="+instance).Output(); len(bytes.TrimSpace(left)) != 0 {
		t.Errorf("containers are left under the label: %s", left)
	}
}

// fixtureImage builds, over scratch, a static copy of this test binary whose role is the probe, and returns
// it referenced by its digest.
func fixtureImage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	build := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "probe"), ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the probe does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY probe /probe\nENV "+role+"=probe\nENTRYPOINT [\"/probe\"]\n"), 0o644)
	tag := "localhost/yoke-l3-probe:" + strconv.Itoa(os.Getpid())
	if said, err := exec.Command("podman", "build", "-q", "-t", tag, dir).CombinedOutput(); err != nil {
		t.Fatalf("the fixture image does not build: %v\n%s", err, said)
	}
	t.Cleanup(func() { exec.Command("podman", "rmi", "-f", tag).Run() })
	digest, err := exec.Command("podman", "images", "--digests", "--format", "{{.Digest}}", tag).Output()
	if err != nil {
		t.Fatal(err)
	}
	return "localhost/yoke-l3-probe@" + strings.TrimSpace(string(digest))
}

// podmanService starts rootless Podman's API on a socket of the test's, and reaches it.
func podmanService(t *testing.T) *engine.Engine {
	t.Helper()
	socket := filepath.Join(short(t), "podman.sock")
	service := exec.Command("podman", "system", "service", "--time=0", "unix://"+socket)
	if err := service.Start(); err != nil {
		t.Fatalf("the environment declares rootless Podman, and it cannot be run: %v", err)
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
	e, err := engine.Reach(patient(t), "unix://"+socket)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// probe is the fixture's process: it says who it runs as and which interfaces it has, writes the file it
// is told to, and exits 3.
func probe() int {
	interfaces := []string{}
	if entries, err := os.ReadDir("/sys/class/net"); err == nil {
		for _, e := range entries {
			interfaces = append(interfaces, e.Name())
		}
	} else if raw, err := os.ReadFile("/proc/net/dev"); err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(raw))
		for scanner.Scan() {
			if name, _, ok := strings.Cut(scanner.Text(), ":"); ok {
				interfaces = append(interfaces, strings.TrimSpace(name))
			}
		}
	}
	fmt.Printf("uid=%d gid=%d\ninterfaces=%s\n", os.Getuid(), os.Getgid(), strings.Join(interfaces, ","))
	if path := os.Getenv("WRITE"); path != "" {
		if err := os.WriteFile(path, []byte("written\n"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "cannot write:", err)
		}
	}
	return 3
}
