package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// The labels every container carries, so that what an instance launched can be found again: the instance,
// the unit and the incarnation.
const (
	LabelInstance    = "dev.yoke-project.instance"
	LabelUnit        = "dev.yoke-project.unit"
	LabelIncarnation = "dev.yoke-project.incarnation"
)

// Launch is what a container is created with: the image by digest, what the unit is handed, the
// instance's directory, which is bound at its own path, and who launches it.
type Launch struct {
	Image       string
	Args        []string
	Env         []string
	Directory   string
	Instance    string
	Unit        string
	Incarnation int
	UID, GID    int
	Devices     []string // device nodes, mapped at their own paths
	Mounts      []string // host paths bound at their own paths, beside the instance's directory
	Groups      []int    // the devices' owning groups, as numbers the host's lookup returned
	Network     bool     // the engine's default network, where none is given otherwise
}

// Absent is an image the engine does not hold. The Core never obtains one.
type Absent struct{ Image string }

func (a *Absent) Error() string {
	return fmt.Sprintf("the engine holds no image %s, and the Core obtains none", a.Image)
}

// Event is one of the engine's events about a container: its identity, what happened, and the status it
// exited with where it ended.
type Event struct {
	ID       string
	Action   string
	ExitCode int
}

// Create creates a container for l and returns its identity. It is created with the instance's directory
// at the identical path and nothing else shared, no network, the three labels, the instance's slice as
// its parent, and the launching identity held constant across the boundary as this engine needs it.
func (e *Engine) Create(ctx context.Context, l Launch) (string, error) {
	host := map[string]any{
		"NetworkMode":  "none",
		"Binds":        []string{l.Directory + ":" + l.Directory},
		"CgroupParent": Slice(l.Instance),
	}
	body := map[string]any{
		"Image":  l.Image,
		"Env":    l.Env,
		"Labels": map[string]string{LabelInstance: l.Instance, LabelUnit: l.Unit, LabelIncarnation: strconv.Itoa(l.Incarnation)},
	}
	if len(l.Args) > 0 {
		body["Cmd"] = l.Args
	}
	// The identity inside must be the one that owns the instance's sockets outside. A rootless Podman keeps
	// it when asked to; a rootless Docker maps its root to the launching account; an engine running as root
	// runs whom it is told to.
	switch {
	case e.Kind == Podman && e.Rootless:
		host["UsernsMode"] = "keep-id"
	case e.Kind == Docker && e.Rootless:
		body["User"] = "0:0"
	default:
		body["User"] = fmt.Sprintf("%d:%d", l.UID, l.GID)
	}
	body["HostConfig"] = host
	resp, err := e.do(ctx, http.MethodPost, "/containers/create", nil, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusCreated, http.StatusOK:
		var created struct {
			ID string `json:"Id"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&created); err != nil || created.ID == "" {
			return "", fmt.Errorf("the engine created a container and named none: %v", err)
		}
		return created.ID, nil
	case http.StatusNotFound:
		return "", &Absent{Image: l.Image}
	}
	return "", refusal(resp, "create a container for "+l.Image)
}

// Attach attaches to the container's output before it starts, writing what it writes on its output to
// stdout and on its error stream to stderr. What it returns is closed when the output ends.
func (e *Engine) Attach(ctx context.Context, id string, stdout, stderr io.Writer) (<-chan struct{}, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", e.path)
	if err != nil {
		return nil, &Unreachable{Address: e.address, Err: err}
	}
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}
	fmt.Fprintf(conn, "POST /v%s/containers/%s/attach?stream=1&stdout=1&stderr=1 HTTP/1.1\r\nHost: engine\r\n"+
		"Connection: Upgrade\r\nUpgrade: tcp\r\nContent-Length: 0\r\n\r\n", API, url.PathEscape(id))
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		conn.Close()
		return nil, &Unreachable{Address: e.address, Err: err}
	}
	if resp.StatusCode != http.StatusSwitchingProtocols && resp.StatusCode != http.StatusOK {
		defer conn.Close()
		return nil, refusal(resp, "attach to "+id)
	}
	// The attachment outlives the asking: the output runs for as long as the container does.
	conn.SetDeadline(time.Time{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		demultiplex(reader, stdout, stderr)
	}()
	return done, nil
}

// demultiplex reads the engine's framing of a container's two streams: one byte naming the stream, three
// of padding, the length, then that many bytes.
func demultiplex(r io.Reader, stdout, stderr io.Writer) {
	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			return
		}
		to := stdout
		if header[0] == 2 {
			to = stderr
		}
		if _, err := io.CopyN(to, r, int64(binary.BigEndian.Uint32(header[4:]))); err != nil {
			return
		}
	}
}

// Start starts a created container.
func (e *Engine) Start(ctx context.Context, id string) error {
	return e.act(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil, "start "+id)
}

// Signal delivers a signal, named as `SIGTERM` is, to the container's process.
func (e *Engine) Signal(ctx context.Context, id, signal string) error {
	return e.act(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/kill", url.Values{"signal": {signal}}, "signal "+id)
}

// Remove removes a container and what it holds. A container already gone is removed.
func (e *Engine) Remove(ctx context.Context, id string) error {
	return e.act(ctx, http.MethodDelete, "/containers/"+url.PathEscape(id), url.Values{"force": {"1"}, "v": {"1"}}, "remove "+id)
}

// Events follows the engine's events about this instance's containers, until ctx ends or the engine
// stops sending them; what it returns is closed then.
func (e *Engine) Events(ctx context.Context, instance string) (<-chan Event, error) {
	filters, _ := json.Marshal(map[string][]string{"label": {LabelInstance + "=" + instance}, "type": {"container"}})
	resp, err := e.do(ctx, http.MethodGet, "/events", url.Values{"filters": {string(filters)}}, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, refusal(resp, "follow the events")
	}
	out := make(chan Event)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		decoder := json.NewDecoder(resp.Body)
		for {
			var raw struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Action string
				Actor  struct {
					ID         string
					Attributes map[string]string
				}
			}
			if err := decoder.Decode(&raw); err != nil {
				return
			}
			ev := Event{ID: raw.Actor.ID, Action: raw.Action}
			if ev.ID == "" {
				ev.ID = raw.ID
			}
			if ev.Action == "" {
				ev.Action = raw.Status
			}
			if code, err := strconv.Atoi(raw.Actor.Attributes["exitCode"]); err == nil {
				ev.ExitCode = code
			}
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// Slice is the instance's control group: a systemd slice named after it, under yoke.slice, its identity
// escaped as a unit name escapes it so that a `-` in it does not nest.
func Slice(instance string) string {
	var b strings.Builder
	for i := 0; i < len(instance); i++ {
		c := instance[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == ':', c == '.' && i > 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, `\x%02x`, c)
		}
	}
	return "yoke-" + b.String() + ".slice"
}

// do asks the engine, on the version the Core speaks, sending body as JSON where there is one.
func (e *Engine) do(ctx context.Context, method, path string, query url.Values, body any) (*http.Response, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	target := "http://engine/v" + API + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, &Unreachable{Address: e.address, Err: err}
	}
	return resp, nil
}

// act asks for an act whose success is the answer's status and nothing more. A container already gone is
// an act already done.
func (e *Engine) act(ctx context.Context, method, path string, query url.Values, what string) error {
	resp, err := e.do(ctx, method, path, query, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode == http.StatusNotModified ||
		(method == http.MethodDelete && resp.StatusCode == http.StatusNotFound) {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return refusal(resp, what)
}

// refusal is the engine's answer to an act it did not perform, with the message it gave.
func refusal(resp *http.Response, what string) error {
	var answer struct{ Message string }
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if json.Unmarshal(raw, &answer) != nil || answer.Message == "" {
		answer.Message = strings.TrimSpace(string(raw))
	}
	return fmt.Errorf("the engine would not %s: %d %s", what, resp.StatusCode, answer.Message)
}
