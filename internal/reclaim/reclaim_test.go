package reclaim

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

func setup(t *testing.T) Options {
	t.Helper()
	apps := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(apps, 0o755)
	os.Chmod(apps, 0o775|os.ModeSetgid)
	run, _ := os.MkdirTemp("", "yr")
	t.Cleanup(func() { os.RemoveAll(run) })
	return Options{InstallRoot: apps, Instances: filepath.Join(t.TempDir(), "instances"), Runtime: run}
}

func install(t *testing.T, o Options, ver string, state installroot.State) {
	t.Helper()
	inst, err := installroot.Root{Path: o.InstallRoot}.Begin("com.yoke.station", ver)
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(inst.Dir, "bundle"), 0o775)
	inst.Write(installroot.Record{State: state, Shared: true})
}

func pin(t *testing.T, o Options, name, ver string) {
	t.Helper()
	err := instancerecord.Create(o.Instances, instancerecord.Record{ID: "com.yoke.station", Version: ver, Name: name,
		DesiredState: instancerecord.Stopped, Owner: "davide", CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
}

func there(o Options, ver string) bool {
	_, err := os.Stat(filepath.Join(o.InstallRoot, "com.yoke.station", ver))
	return err == nil
}

// std: yoke:the-removal.01
func TestARemovalIsRefusedWhileARecordPointsAtIt(t *testing.T) {
	o := setup(t)
	install(t, o, "2.0.0", installroot.Complete)
	install(t, o, "2.1.0", installroot.Complete)
	pin(t, o, "bench-a", "2.0.0")
	_, err := Remove(o, "com.yoke.station", "")
	if err == nil || !strings.Contains(err.Error(), "bench-a") || !strings.Contains(err.Error(), "2.0.0") {
		t.Errorf("removing what an instance points at: %v", err)
	}
	if !there(o, "2.0.0") || !there(o, "2.1.0") {
		t.Error("a refused removal removed something")
	}
	removed, err := Remove(o, "com.yoke.station", "2.1.0")
	if err != nil || !slices.Equal(removed, []string{"2.1.0"}) || there(o, "2.1.0") || !there(o, "2.0.0") {
		t.Errorf("removing 2.1.0: %v %v", removed, err)
	}
}

// std: yoke:the-removal.02
func TestARemovalWritesWithdrawnBeforeItDeletes(t *testing.T) {
	o := setup(t)
	install(t, o, "2.1.0", installroot.Complete)
	var seen installroot.State
	o.withdrawn = func(dir string) {
		rec, _ := installroot.ReadRecord(dir)
		seen = rec.State
	}
	if _, err := Remove(o, "com.yoke.station", "2.1.0"); err != nil {
		t.Fatal(err)
	}
	if seen != installroot.Withdrawn {
		t.Errorf("between the two acts the record said %q", seen)
	}
	if _, err := os.Stat(filepath.Join(o.InstallRoot, "com.yoke.station")); err == nil {
		t.Error("the application's directory outlived its last version")
	}
}

// std: yoke:the-removal.03
func TestThePassCollectsSupersededAndIncompleteAndNeverAPrerelease(t *testing.T) {
	o := setup(t)
	for ver, state := range map[string]installroot.State{"1.0.0": installroot.Complete, "2.0.0": installroot.Complete,
		"2.1.0": installroot.Complete, "2.2.0-rc.1": installroot.Complete, "2.3.0": installroot.Incomplete} {
		install(t, o, ver, state)
	}
	pin(t, o, "bench-a", "1.0.0")
	r, err := Prune(o, "com.yoke.station")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(r.Removed)
	if !slices.Equal(r.Removed, []string{"com.yoke.station 2.0.0", "com.yoke.station 2.3.0"}) {
		t.Errorf("removed %v", r.Removed)
	}
	for _, kept := range []string{"1.0.0", "2.1.0", "2.2.0-rc.1"} {
		if !there(o, kept) {
			t.Errorf("%s was removed", kept)
		}
	}
}

// fakeEngine holds images by their tags, as each case scripts it.
type fakeEngine struct {
	mu      sync.Mutex
	tags    map[string]string // tag → image
	deleted []string
}

func (f *fakeEngine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := r.URL.Path
	switch {
	case strings.HasSuffix(p, "/version"):
		json.NewEncoder(w).Encode(map[string]any{"Version": "6.1.3", "ApiVersion": "1.44", "MinAPIVersion": "1.24",
			"Components": []map[string]any{{"Name": "Podman Engine"}}})
	case strings.HasSuffix(p, "/info"):
		json.NewEncoder(w).Encode(map[string]any{"SecurityOptions": []string{"name=rootless"}})
	case strings.HasSuffix(p, "/images/json"):
		byImage := map[string][]string{}
		for tag, image := range f.tags {
			byImage[image] = append(byImage[image], tag)
		}
		out := []map[string]any{}
		for image, tags := range byImage {
			out = append(out, map[string]any{"Id": image, "RepoTags": tags})
		}
		json.NewEncoder(w).Encode(out)
	case r.Method == http.MethodDelete:
		ref := p[strings.Index(p, "/images/")+len("/images/"):]
		f.deleted = append(f.deleted, ref)
		delete(f.tags, ref)
		w.Write([]byte("[]"))
	default:
		http.NotFound(w, r)
	}
}

func serve(t *testing.T, f http.Handler) string {
	t.Helper()
	dir, _ := os.MkdirTemp("", "re")
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "engine.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	s := &http.Server{Handler: f}
	go s.Serve(l)
	t.Cleanup(func() { s.Close() })
	return "unix://" + path
}

// std: yoke:the-removal.04
func TestThePassCollectsYokesImagesNoBundleReferences(t *testing.T) {
	o := setup(t)
	install(t, o, "2.0.0", installroot.Complete)
	install(t, o, "2.1.0", installroot.Complete)
	f := &fakeEngine{tags: map[string]string{"yoke/com.yoke.station:2.0.0": "sha256:old", "yoke/com.yoke.station:2.1.0": "sha256:new",
		"localhost/mine:latest": "sha256:mine"}}
	address := serve(t, f)
	o.Engine = func(ctx context.Context) (*engine.Engine, error) { return engine.Reach(ctx, address) }
	if _, err := Prune(o, ""); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.deleted, []string{"yoke/com.yoke.station:2.0.0", "sha256:old"}) {
		t.Errorf("deleted %v", f.deleted)
	}
	untagged := &fakeEngine{tags: map[string]string{"localhost/mine:latest": "sha256:mine"}}
	empty := setup(t)
	empty.Engine = func(ctx context.Context) (*engine.Engine, error) { return engine.Reach(ctx, serve(t, untagged)) }
	if _, err := Prune(empty, ""); err != nil {
		t.Fatal(err)
	}
	if len(untagged.deleted) > 0 {
		t.Errorf("with no image tagged, the engine was asked to remove %v", untagged.deleted)
	}
}

// holdClaim runs this test binary as another process holding an instance's claim.
func holdClaim(t *testing.T, root string) {
	t.Helper()
	self, _ := os.Executable()
	cmd := exec.Command(self, "-test.run=^$")
	cmd.Env = append(os.Environ(), claimRole+"="+root)
	out, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	buf := make([]byte, 5)
	if _, err := io.ReadFull(out, buf); err != nil || string(buf) != "held\n" {
		t.Fatalf("the claim was not taken: %q %v", buf, err)
	}
}

// std: yoke:the-removal.05
func TestThePassClearsAFailedEphemeralRecordAndAFreeRuntimeDirectory(t *testing.T) {
	o := setup(t)
	install(t, o, "2.1.0", installroot.Complete)
	instancerecord.Create(o.Instances, instancerecord.Record{ID: "com.yoke.station", Version: "2.1.0", Name: "run-1",
		DesiredState: instancerecord.Running, Owner: "davide", CreatedAt: time.Now(), Ephemeral: true, Outcome: instancerecord.Failed})
	pin(t, o, "bench-a", "2.1.0")
	free, held := filepath.Join(o.Runtime, "gone"), filepath.Join(o.Runtime, "bench-a")
	os.MkdirAll(free, 0o700)
	os.WriteFile(filepath.Join(free, "instance.lock"), nil, 0o600)
	holdClaim(t, held)
	r, err := Prune(o, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(o.Instances, "run-1")); err == nil {
		t.Error("the failed ephemeral record is still there")
	}
	if _, err := os.Stat(filepath.Join(o.Instances, "bench-a")); err != nil {
		t.Error("the declared record was removed")
	}
	if _, err := os.Stat(free); err == nil {
		t.Error("the free runtime directory is still there")
	}
	if _, err := os.Stat(held); err != nil {
		t.Error("the held runtime directory was removed")
	}
	if len(r.Removed) < 2 {
		t.Errorf("the pass named %v", r.Removed)
	}
}
