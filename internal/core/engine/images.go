package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// What installing asks of the engine about images. The Core asks none of it: it never pulls, and an image
// it does not find is absent.

// HasImage says whether the engine holds an image under the reference given — a name and the digest
// pinned to it. The same digest under another name is not that image.
func (e *Engine) HasImage(ctx context.Context, ref string) (bool, error) {
	resp, err := e.do(ctx, http.MethodGet, "/images/"+ref+"/json", nil, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode == http.StatusOK:
		return true, nil
	case resp.StatusCode == http.StatusNotFound:
		return false, nil
	}
	return false, fmt.Errorf("the engine did not say whether it holds %s: %d", ref, resp.StatusCode)
}

// Pull asks the engine for the reference given, pinned by digest, handing on each line of its progress.
func (e *Engine) Pull(ctx context.Context, ref string, progress func(string)) error {
	resp, err := e.do(ctx, http.MethodPost, "/images/create", url.Values{"fromImage": {ref}}, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return refusal(resp, "pull "+ref)
	}
	if err := follow(resp.Body, progress); err != nil {
		return fmt.Errorf("the engine did not pull %s: %w", ref, err)
	}
	return nil
}

// Load sends the engine an image archive, handing on what it said.
func (e *Engine) Load(ctx context.Context, archive io.Reader, progress func(string)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://engine/v"+API+"/images/load", archive)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-tar")
	resp, err := e.client.Do(req)
	if err != nil {
		return &Unreachable{Address: e.address, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return refusal(resp, "load the archive")
	}
	if err := follow(resp.Body, progress); err != nil {
		return fmt.Errorf("the engine did not load the archive: %w", err)
	}
	return nil
}

// Tag places an image in the reserved namespace, `yoke/<id>:<version>`, which is what reclaiming reads.
func (e *Engine) Tag(ctx context.Context, ref, id, version string) error {
	return e.act(ctx, http.MethodPost, "/images/"+ref+"/tag", url.Values{"repo": {"yoke/" + id}, "tag": {version}},
		"tag "+ref+" as yoke/"+id+":"+version)
}

// follow reads an engine's progress, one JSON object at a time, handing on each line it carries; an error
// in the stream is the operation's failure.
func follow(r io.Reader, progress func(string)) error {
	dec := json.NewDecoder(r)
	for {
		var line struct {
			Status, Stream, Progress, Error string
			ErrorDetail                     struct{ Message string }
		}
		err := dec.Decode(&line)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if line.Error != "" || line.ErrorDetail.Message != "" {
			return errors.New(strings.TrimSpace(line.Error + " " + line.ErrorDetail.Message))
		}
		if text := strings.TrimSpace(strings.TrimSpace(line.Status+" "+line.Progress) + line.Stream); text != "" {
			progress(text)
		}
	}
}
