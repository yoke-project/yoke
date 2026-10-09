package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
)

// labelled is an engine holding containers under labels, answering the list and each container's
// inspection, and recording what it was asked.
type labelled struct {
	identity *fake
	held     []map[string]any // as the list answers each: Id, Labels, State
	ended    map[string]int   // the status each ended container exited with, by identity

	mu    sync.Mutex
	asked []request
}

func (l *labelled) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v"+engine.API)
	if strings.HasSuffix(path, "/version") || strings.HasSuffix(path, "/info") {
		l.identity.ServeHTTP(w, r)
		return
	}
	l.mu.Lock()
	l.asked = append(l.asked, request{method: r.Method, path: path, query: r.URL.Query()})
	l.mu.Unlock()
	switch {
	case path == "/containers/json":
		json.NewEncoder(w).Encode(l.held)
	case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
		status, ok := l.ended[id]
		if !ok {
			json.NewEncoder(w).Encode(map[string]any{"Id": id, "State": map[string]any{"Status": "running", "Running": true}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"Id": id, "State": map[string]any{"Status": "exited", "Running": false, "ExitCode": status}})
	default:
		http.NotFound(w, r)
	}
}

// std: yoke:the-engine-returning.01
func TestWhatRunsUnderTheLabelIsAskedOfTheEngine(t *testing.T) {
	l := &labelled{identity: podman(true), ended: map[string]int{"c2": 3}, held: []map[string]any{
		{"Id": "c1", "State": "running", "Labels": map[string]string{engine.LabelInstance: "bench", engine.LabelUnit: "panel", engine.LabelIncarnation: "2"}},
		{"Id": "c2", "State": "exited", "Labels": map[string]string{engine.LabelInstance: "bench", engine.LabelUnit: "calibrate", engine.LabelIncarnation: "1"}},
	}}
	e, err := reach(t, serve(t, l))
	if err != nil {
		t.Fatal(err)
	}
	found, err := e.List(ctx(t), "bench")
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(found, func(a, b engine.Found) int { return strings.Compare(a.ID, b.ID) })
	want := []engine.Found{{ID: "c1", Unit: "panel", Incarnation: 2, Running: true}, {ID: "c2", Unit: "calibrate", Incarnation: 1, ExitCode: 3}}
	if !slices.Equal(found, want) {
		t.Errorf("found %+v, want %+v", found, want)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	list := l.asked[0]
	var filters map[string][]string
	json.Unmarshal([]byte(list.query.Get("filters")), &filters)
	if list.path != "/containers/json" || list.query.Get("all") != "1" ||
		fmt.Sprint(filters["label"]) != "[dev.yoke-project.instance=bench]" || len(filters) != 1 {
		t.Errorf("the engine was asked %s %v", list.path, list.query)
	}
}

// std: yoke:the-engine-returning.02
func TestTheSocketAppearingIsANotice(t *testing.T) {
	path := filepath.Join(short(t), "engine.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: podman(true)}
	go server.Serve(listener)
	e, err := reach(t, "unix://"+path)
	if err != nil {
		t.Fatal(err)
	}
	server.Close()
	if _, err := os.Stat(path); err == nil {
		os.Remove(path)
	}

	waiting, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	notices, err := e.Returned(waiting)
	if err != nil {
		t.Fatal(err)
	}
	none := func(what string) {
		t.Helper()
		select {
		case _, open := <-notices:
			if open {
				t.Errorf("a notice arrived for %s", what)
			}
		case <-time.After(200 * time.Millisecond):
		}
	}
	one := func(what string) {
		t.Helper()
		select {
		case <-notices:
		case <-time.After(3 * time.Second):
			t.Fatalf("no notice arrived for %s", what)
		}
	}

	os.WriteFile(filepath.Join(filepath.Dir(path), "other"), nil, 0o600)
	none("another file")
	again, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	one("the socket bound again")
	again.Close()
	os.Remove(path)
	third, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	one("the socket bound a third time")
	none("nothing more")

	cancel()
	select {
	case _, open := <-notices:
		if open {
			t.Error("a notice arrived once the wait ended")
		}
	case <-time.After(3 * time.Second):
		t.Error("what carried the notices was not closed when the wait ended")
	}
}
