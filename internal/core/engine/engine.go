// Package engine reaches the container engine through the interface it serves, and no composition tool.
//
// Both engines are spoken to on one projection of the engine API, at one version, which the reference
// engine and the supported one both serve. Which engine answered, and whether it runs rootless, is asked
// of the engine and never configured; an engine that does not answer is unreachable, and nothing is
// concluded from it.
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// API is the one version of the engine's API the Core speaks.
const API = "1.41"

// The engines, as the Core names them.
const (
	Podman = "podman"
	Docker = "docker"
)

// Engine is an engine reached: which one, whether it runs rootless, and the version it states.
type Engine struct {
	Kind     string
	Rootless bool
	Version  string

	address string
	client  *http.Client
}

// Refused is an address the Core cannot speak to: anything but a local socket.
type Refused struct{ Address string }

func (r *Refused) Error() string {
	return fmt.Sprintf("the engine's address %q is not a local socket, written unix:///path", r.Address)
}

// Unreachable is an engine that did not answer.
type Unreachable struct {
	Address string
	Err     error
}

func (u *Unreachable) Error() string {
	return fmt.Sprintf("the engine at %s did not answer: %v", u.Address, u.Err)
}

func (u *Unreachable) Unwrap() error { return u.Err }

// Unsupported is an engine that does not serve the version the Core speaks.
type Unsupported struct{ Min, Max string }

func (u *Unsupported) Error() string {
	return fmt.Sprintf("the engine serves its API from %s to %s, and the Core speaks %s", u.Min, u.Max, API)
}

// version is what an engine's /version answers, in the part the Core reads.
type version struct {
	Version       string
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string
	Components    []struct{ Name string }
}

// info is what an engine's /info answers, in the part the Core reads.
type info struct {
	SecurityOptions []string
}

// Reach reaches the engine at address, which is unix:// and a path: it asks the engine which it is, the
// range of its API, and whether it runs rootless.
func Reach(ctx context.Context, address string) (*Engine, error) {
	path, ok := strings.CutPrefix(address, "unix://")
	if !ok || !strings.HasPrefix(path, "/") {
		return nil, &Refused{Address: address}
	}
	e := &Engine{address: address, client: &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}}}
	var v version
	status, err := e.get(ctx, "/v"+API+"/version", &v)
	if err != nil {
		return nil, err
	}
	if status == http.StatusBadRequest {
		// An engine whose oldest version is newer than the Core's refuses the request itself; its range
		// is read where no version is named.
		if _, err := e.get(ctx, "/version", &v); err != nil {
			return nil, err
		}
	}
	if !serves(v.MinAPIVersion, v.APIVersion) {
		return nil, &Unsupported{Min: v.MinAPIVersion, Max: v.APIVersion}
	}
	var i info
	if _, err := e.get(ctx, "/v"+API+"/info", &i); err != nil {
		return nil, err
	}
	e.Kind = Docker
	if slices.ContainsFunc(v.Components, func(c struct{ Name string }) bool { return c.Name == "Podman Engine" }) {
		e.Kind = Podman
	}
	e.Rootless = slices.Contains(i.SecurityOptions, "name=rootless")
	e.Version = v.Version
	return e, nil
}

// get asks the engine on path and decodes a successful answer into into, returning the status. An engine
// that cannot be reached, or answers with something that is not its API, is unreachable.
func (e *Engine) get(ctx context.Context, path string, into any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://engine"+path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return 0, &Unreachable{Address: e.address, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return 0, &Unreachable{Address: e.address, Err: fmt.Errorf("%s answered something that is not the engine's API: %w", path, err)}
	}
	return resp.StatusCode, nil
}

// serves says whether the range from min to max holds the version the Core speaks.
func serves(min, max string) bool {
	return compare(min, API) <= 0 && compare(API, max) <= 0
}

// compare orders two versions written major.minor; an unreadable one sorts last.
func compare(a, b string) int {
	pa, pb := parts(a), parts(b)
	for i := range 2 {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func parts(v string) [2]int {
	major, minor, ok := strings.Cut(v, ".")
	a, errA := strconv.Atoi(major)
	b, errB := strconv.Atoi(minor)
	if !ok || errA != nil || errB != nil {
		return [2]int{1 << 30, 0}
	}
	return [2]int{a, b}
}
