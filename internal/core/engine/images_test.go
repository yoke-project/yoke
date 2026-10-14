package engine_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
)

const held = "localhost/station@sha256:637da87b24f13ea5e010fdbd5e0bf474f14d8da34309a59b6d7c735faa637f04"

// images is a Podman engine that also answers about images, as each case scripts it.
type images struct {
	*fake
	mu     sync.Mutex
	held   map[string]bool
	pull   func(w http.ResponseWriter, from string)
	load   func(w http.ResponseWriter, body []byte)
	tagged []string
	pulled []string
}

func (f *images) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if i := strings.Index(path, "/images/"); i >= 0 {
		rest := path[i+len("/images/"):]
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(rest, "/json"):
			if !f.held[strings.TrimSuffix(rest, "/json")] {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(`{"Id":"sha256:9e9b"}`))
		case r.Method == http.MethodPost && rest == "create":
			from := r.URL.Query().Get("fromImage")
			f.pulled = append(f.pulled, from)
			f.pull(w, from)
		case r.Method == http.MethodPost && rest == "load":
			body, _ := io.ReadAll(r.Body)
			if r.Header.Get("Content-Type") != "application/x-tar" {
				http.Error(w, `{"message":"not a tar"}`, http.StatusBadRequest)
				return
			}
			f.load(w, body)
		case r.Method == http.MethodPost && strings.HasSuffix(rest, "/tag"):
			f.tagged = append(f.tagged, strings.TrimSuffix(rest, "/tag")+" "+r.URL.Query().Get("repo")+":"+r.URL.Query().Get("tag"))
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
		return
	}
	f.fake.ServeHTTP(w, r)
}

func withImages(t *testing.T, f *images) *engine.Engine {
	t.Helper()
	if f.held == nil {
		f.held = map[string]bool{}
	}
	e, err := reach(t, serve(t, f))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// std: yoke:the-engine-images.01
func TestAnImageIsPresentByItsReference(t *testing.T) {
	e := withImages(t, &images{fake: podman(true), held: map[string]bool{held: true}})
	if ok, err := e.HasImage(context.Background(), held); err != nil || !ok {
		t.Errorf("the image held: %v %v", ok, err)
	}
	other := strings.Replace(held, "localhost/station", "localhost/nothing", 1)
	if ok, err := e.HasImage(context.Background(), other); err != nil || ok {
		t.Errorf("the same digest under another name: %v %v", ok, err)
	}
}

// std: yoke:the-engine-images.02
func TestAPullAsksForTheReferenceAndReportsProgress(t *testing.T) {
	f := &images{fake: podman(true), pull: func(w http.ResponseWriter, from string) {
		for _, s := range []string{"Trying to pull", "Copying blob", "Writing manifest"} {
			json.NewEncoder(w).Encode(map[string]string{"status": s})
		}
	}}
	e := withImages(t, f)
	var lines []string
	if err := e.Pull(context.Background(), held, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if len(f.pulled) != 1 || f.pulled[0] != held || len(lines) != 3 || lines[2] != "Writing manifest" {
		t.Errorf("asked for %v, progress %v", f.pulled, lines)
	}
	failing := withImages(t, &images{fake: podman(true), pull: func(w http.ResponseWriter, from string) {
		json.NewEncoder(w).Encode(map[string]string{"status": "Trying to pull"})
		json.NewEncoder(w).Encode(map[string]string{"error": "manifest unknown"})
	}})
	err := failing.Pull(context.Background(), held, func(string) {})
	if err == nil || !strings.Contains(err.Error(), held) || !strings.Contains(err.Error(), "manifest unknown") {
		t.Errorf("a pull the engine failed: %v", err)
	}
}

// std: yoke:the-engine-images.03
func TestAnArchiveIsLoaded(t *testing.T) {
	var got []byte
	e := withImages(t, &images{fake: podman(true), load: func(w http.ResponseWriter, body []byte) {
		got = body
		json.NewEncoder(w).Encode(map[string]string{"stream": "Loaded image: " + held})
	}})
	var lines []string
	if err := e.Load(context.Background(), bytes.NewReader([]byte("an archive")), func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if string(got) != "an archive" || len(lines) != 1 || !strings.Contains(lines[0], held) {
		t.Errorf("sent %q, said %v", got, lines)
	}
	refusing := withImages(t, &images{fake: podman(true), load: func(w http.ResponseWriter, body []byte) {
		http.Error(w, `{"message":"payload does not match any of the supported image formats"}`, http.StatusInternalServerError)
	}})
	err := refusing.Load(context.Background(), bytes.NewReader([]byte("x")), func(string) {})
	if err == nil || !strings.Contains(err.Error(), "supported image formats") {
		t.Errorf("a load the engine refused: %v", err)
	}
}

// std: yoke:the-engine-images.04
func TestAnImageIsTaggedInTheReservedNamespace(t *testing.T) {
	f := &images{fake: podman(true), held: map[string]bool{held: true}}
	e := withImages(t, f)
	if err := e.Tag(context.Background(), held, "com.yoke.station", "2.1.0"); err != nil {
		t.Fatal(err)
	}
	if len(f.tagged) != 1 || f.tagged[0] != held+" yoke/com.yoke.station:2.1.0" {
		t.Errorf("tagged %v", f.tagged)
	}
}

// std: yoke:the-engine-images.05
func TestAnOCIArchiveSavedByDigestLoads(t *testing.T) {
	enginetest.Not(t, enginetest.Docker)
	ref, archive := enginetest.OCIArchive(t, "yoke-load")
	s := enginetest.Serve(t)
	e, err := engine.Reach(context.Background(), "unix://"+s.Socket)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := e.HasImage(context.Background(), ref); ok {
		t.Fatal("the image is still held before the load")
	}
	f, _ := os.Open(archive)
	defer f.Close()
	if err := e.Load(context.Background(), f, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if ok, err := e.HasImage(context.Background(), ref); err != nil || !ok {
		t.Errorf("after the load, %s is held: %v %v", ref, ok, err)
	}
}

// std: yoke:the-engine-images.06
func TestAReservedTagIsReadBackHoweverTheEngineQualifiesIt(t *testing.T) {
	for _, tag := range []string{"yoke/com.yoke.station:2.1.0", "localhost/yoke/com.yoke.station:2.1.0", "docker.io/yoke/com.yoke.station:2.1.0"} {
		if id, version, ok := engine.Reserved(tag); !ok || id != "com.yoke.station" || version != "2.1.0" {
			t.Errorf("%s reads as %q %q %v", tag, id, version, ok)
		}
	}
	for _, tag := range []string{"registry.example/yoke/com.yoke.station:2.1.0", "docker.io/library/busybox:1.36", "yoke/com.yoke.station", "docker.io/yoke/:2.1.0"} {
		if id, version, ok := engine.Reserved(tag); ok {
			t.Errorf("%s reads as %q %q", tag, id, version)
		}
	}
}
