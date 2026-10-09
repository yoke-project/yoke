package engine

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Found is a container found under an instance's label: its identity, the unit and the incarnation it
// is, and whether it runs or the status it ended with.
type Found struct {
	ID          string
	Unit        string
	Incarnation int
	Running     bool
	ExitCode    int
}

// List asks the engine for every container under the instance's label, ended or not. What is not running
// is inspected for the status it ended with; one removed meanwhile is not found.
func (e *Engine) List(ctx context.Context, instance string) ([]Found, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {LabelInstance + "=" + instance}})
	resp, err := e.do(ctx, http.MethodGet, "/containers/json", url.Values{"all": {"1"}, "filters": {string(filters)}}, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, refusal(resp, "list the instance's containers")
	}
	var listed []struct {
		ID     string `json:"Id"`
		State  string
		Labels map[string]string
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		return nil, &Unreachable{Address: e.address, Err: fmt.Errorf("the list of containers is not the engine's API: %w", err)}
	}
	var found []Found
	for _, c := range listed {
		f := Found{ID: c.ID, Unit: c.Labels[LabelUnit], Running: c.State == "running"}
		f.Incarnation, _ = strconv.Atoi(c.Labels[LabelIncarnation])
		if !f.Running {
			running, status, held, err := e.inspect(ctx, c.ID)
			if err != nil {
				return nil, err
			}
			if !held {
				continue
			}
			f.Running, f.ExitCode = running, status
		}
		found = append(found, f)
	}
	return found, nil
}

// inspect asks the engine whether a container runs and, if not, the status it ended with; held is false
// for a container the engine no longer holds.
func (e *Engine) inspect(ctx context.Context, id string) (running bool, status int, held bool, err error) {
	resp, err := e.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/json", nil, nil)
	if err != nil {
		return false, 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, 0, false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, 0, false, refusal(resp, "inspect "+id)
	}
	var inspected struct {
		State struct {
			Running  bool
			ExitCode int
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&inspected); err != nil {
		return false, 0, false, &Unreachable{Address: e.address, Err: fmt.Errorf("the container %s is not described in the engine's API: %w", id, err)}
	}
	return inspected.State.Running, inspected.State.ExitCode, true, nil
}

// Returned notices each time the engine's socket is bound at its path again, which is how an engine that
// went away is learnt to be back without looking for it. What it returns carries one notice for however
// many arrived since it was last read, and is closed once ctx ends.
func (e *Engine) Returned(ctx context.Context) (<-chan struct{}, error) {
	fd, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil, os.NewSyscallError("inotify_init1", err)
	}
	if _, err := syscall.InotifyAddWatch(fd, filepath.Dir(e.path), syscall.IN_CREATE|syscall.IN_MOVED_TO|syscall.IN_ATTRIB); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("the engine's socket's directory cannot be watched: %w", os.NewSyscallError("inotify_add_watch", err))
	}
	// Non-blocking, the descriptor is the runtime's to wait on, and closing it ends a read in progress.
	watch := os.NewFile(uintptr(fd), "inotify")
	name := filepath.Base(e.path)
	out := make(chan struct{}, 1)
	go func() {
		<-ctx.Done()
		watch.Close()
	}()
	go func() {
		defer close(out)
		buf := make([]byte, 64*(syscall.SizeofInotifyEvent+syscall.NAME_MAX+1))
		for {
			n, err := watch.Read(buf)
			if err != nil {
				return
			}
			// Each event is its watch, mask and cookie, the length of its name, then the name padded with
			// zeroes.
			for off := 0; off+syscall.SizeofInotifyEvent <= n; {
				length := int(binary.NativeEndian.Uint32(buf[off+12:]))
				named := buf[off+syscall.SizeofInotifyEvent : off+syscall.SizeofInotifyEvent+length]
				off += syscall.SizeofInotifyEvent + length
				if strings.TrimRight(string(named), "\x00") != name {
					continue
				}
				select {
				case out <- struct{}{}:
				default:
				}
			}
		}
	}()
	return out, nil
}
