package install

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
	"github.com/yoke-project/yoke/internal/installroot"
)

// fakeEngine answers what installing asks of an engine about images.
type fakeEngine struct {
	mu        sync.Mutex
	held      map[string]bool
	loadsKeep bool // a load leaves the image held under its reference, as Podman's store does
	loading   string
	pulled    []string
	loaded    int
	tagged    []string
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
	case strings.HasSuffix(p, "/images/create"):
		from := r.URL.Query().Get("fromImage")
		f.pulled = append(f.pulled, from)
		f.held[from] = true
		json.NewEncoder(w).Encode(map[string]string{"status": "Pulled " + from})
	case strings.HasSuffix(p, "/images/load"):
		io.Copy(io.Discard, r.Body)
		f.loaded++
		if f.loadsKeep {
			f.held[f.loading] = true
		}
		json.NewEncoder(w).Encode(map[string]string{"stream": "Loaded image"})
	case strings.HasSuffix(p, "/tag"):
		i := strings.Index(p, "/images/")
		f.tagged = append(f.tagged, strings.TrimSuffix(p[i+len("/images/"):], "/tag")+" "+r.URL.Query().Get("repo")+":"+r.URL.Query().Get("tag"))
		w.WriteHeader(http.StatusCreated)
	case strings.HasSuffix(p, "/json"):
		i := strings.Index(p, "/images/")
		if !f.held[strings.TrimSuffix(p[i+len("/images/"):], "/json")] {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{}`))
	default:
		http.NotFound(w, r)
	}
}

func serveEngine(t *testing.T, f *fakeEngine) string {
	t.Helper()
	dir, _ := os.MkdirTemp("", "ie")
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

func digestOf(s string) string { return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(s))) }

// imaged writes a project whose descriptor names the images given, by unit, carrying the archives of
// those listed in carried.
func imaged(t *testing.T, refs map[string]string, carried []string) string {
	t.Helper()
	dir := project(t, "2.1.0", "aarch64", "")
	descriptor, _ := os.ReadFile(filepath.Join(dir, "yoke.yaml"))
	var units strings.Builder
	for unit, ref := range refs {
		fmt.Fprintf(&units, "  %s: { kind: oneshot, image: \"%s\" }\n", unit, ref)
	}
	os.WriteFile(filepath.Join(dir, "yoke.yaml"), []byte(string(descriptor)+units.String()), 0o644)
	for _, ref := range carried {
		digest := ref[strings.Index(ref, "@")+1:]
		p := filepath.Join(dir, "components", "images", strings.Replace(digest, ":", "-", 1)+".tar")
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("an OCI archive of "+ref), 0o644)
	}
	return dir
}

func withEngine(o Options, address string) Options {
	o.Engine = func(ctx context.Context) (*engine.Engine, error) { return engine.Reach(ctx, address) }
	return o
}

// std: yoke:the-installation.09
func TestEveryImageIsObtainedByItsDigest(t *testing.T) {
	heldRef, carriedRef, pulledRef := "localhost/held@"+digestOf("held"), "localhost/carried@"+digestOf("carried"), "localhost/pulled@"+digestOf("pulled")
	f := &fakeEngine{held: map[string]bool{heldRef: true}, loadsKeep: true, loading: carriedRef}
	apps := root(t)
	pkg := build(t, imaged(t, map[string]string{"held": heldRef, "carried": carriedRef, "again": carriedRef, "pulled": pulledRef}, []string{carriedRef}), compose.BuildOptions{Sign: priv})
	var said strings.Builder
	o := withEngine(options(apps, pkg), serveEngine(t, f))
	o.Out = &said
	res, err := Install(o)
	if err != nil {
		t.Fatal(err)
	}
	if f.loaded != 1 || len(f.pulled) != 1 || f.pulled[0] != pulledRef {
		t.Errorf("loaded %d, pulled %v", f.loaded, f.pulled)
	}
	for unit, ref := range map[string]string{"held": heldRef, "carried": carriedRef, "again": carriedRef, "pulled": pulledRef} {
		if !slices.Contains(f.tagged, ref+" yoke/com.yoke.station/"+unit+":2.1.0") {
			t.Errorf("%s was not tagged for %s: %v", ref, unit, f.tagged)
		}
		if !strings.Contains(said.String(), ref) {
			t.Errorf("nothing was reported about %s:\n%s", ref, said.String())
		}
	}
	if rec, _ := installroot.ReadRecord(res.Dir); rec.State != installroot.Complete {
		t.Errorf("the record is %s", rec.State)
	}
}

// std: yoke:the-installation.10
func TestAnArchiveThatLoadsWithNoDigestIsRefused(t *testing.T) {
	ref := "localhost/carried@" + digestOf("carried")
	f := &fakeEngine{held: map[string]bool{}, loadsKeep: false}
	apps := root(t)
	pkg := build(t, imaged(t, map[string]string{"carried": ref}, []string{ref}), compose.BuildOptions{Sign: priv})
	_, err := Install(withEngine(options(apps, pkg), serveEngine(t, f)))
	refused(t, err, "image.undigested", ref)
	assertIncomplete(t, apps, "2.1.0")
}

// std: yoke:the-installation.11
func TestAnEngineThatCannotBeReachedIsARefusal(t *testing.T) {
	apps := root(t)
	nowhere := "unix:///nonexistent/engine.sock"
	pkg := build(t, imaged(t, map[string]string{"pulled": "localhost/pulled@" + digestOf("pulled")}, nil), compose.BuildOptions{Sign: priv})
	_, err := Install(withEngine(options(apps, pkg), nowhere))
	refused(t, err, "engine.unreachable", "/nonexistent/engine.sock")
	os.RemoveAll(filepath.Join(apps, "com.yoke.station"))
	asked := false
	o := options(apps, build(t, project(t, "2.1.0", "aarch64", ""), compose.BuildOptions{Sign: priv}))
	o.Engine = func(context.Context) (*engine.Engine, error) { asked = true; return nil, fmt.Errorf("no engine") }
	if _, err := Install(o); err != nil || asked {
		t.Errorf("a descriptor naming no image: %v, engine asked %t", err, asked)
	}
}

// std: yoke:the-installation.12
func TestAPackageCarryingAnImageInstallsWithTheEngine(t *testing.T) {
	enginetest.Not(t, enginetest.Docker)
	refs := map[string]string{}
	written := map[string]string{}
	for _, unit := range []string{"panel", "pipeline"} {
		refs[unit], written[unit] = enginetest.OCIArchive(t, "yoke-install-"+unit)
	}
	proj := imaged(t, refs, nil)
	for unit, ref := range refs {
		archive := filepath.Join(proj, "components", "images", strings.Replace(ref[strings.Index(ref, "@")+1:], ":", "-", 1)+".tar")
		os.MkdirAll(filepath.Dir(archive), 0o755)
		data, _ := os.ReadFile(written[unit])
		os.WriteFile(archive, data, 0o644)
		t.Cleanup(func() { exec.Command("podman", "rmi", "-f", "yoke/com.yoke.station/"+unit+":2.1.0").Run() })
	}
	s := enginetest.Serve(t)
	pkg := build(t, proj, compose.BuildOptions{Sign: priv})
	var said strings.Builder
	o := withEngine(options(root(t), pkg), "unix://"+s.Socket)
	o.Host.Arch = "aarch64"
	o.Out = &said
	if _, err := Install(o); err != nil {
		t.Fatalf("%v\n%s", err, said.String())
	}
	if !strings.Contains(said.String(), "loaded from the package") || strings.Contains(said.String(), "pulled") {
		t.Errorf("the images were not loaded from the package:\n%s", said.String())
	}
	id := func(ref string) string {
		out, err := exec.Command("podman", "image", "inspect", "--format", "{{.Id}}", ref).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	for unit, ref := range refs {
		held := id(ref)
		if held == "" {
			t.Errorf("the engine does not hold %s", ref)
			continue
		}
		// Each image keeps its own tag once the installation ends: a tag that moved on to the next image
		// would leave this one invisible to every reclaiming pass.
		if tagged := id("yoke/com.yoke.station/" + unit + ":2.1.0"); tagged != held {
			t.Errorf("the image of %s is not tagged yoke/com.yoke.station/%s:2.1.0: the tag names %q", unit, unit, tagged)
		}
	}
}
