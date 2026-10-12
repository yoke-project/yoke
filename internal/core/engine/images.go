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

// Image is one image in the engine's store and every tag it carries, as the engine names them.
type Image struct {
	ID   string
	Tags []string
}

// Images lists the engine's store.
func (e *Engine) Images(ctx context.Context) ([]Image, error) {
	resp, err := e.do(ctx, http.MethodGet, "/images/json", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, refusal(resp, "list its images")
	}
	var listed []struct {
		ID       string `json:"Id"`
		RepoTags []string
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		return nil, fmt.Errorf("the engine's list of images cannot be read: %w", err)
	}
	images := make([]Image, 0, len(listed))
	for _, l := range listed {
		images = append(images, Image{ID: l.ID, Tags: l.RepoTags})
	}
	return images, nil
}

// Reserved reads a tag in the reserved namespace back into the application and version it names. Podman
// qualifies an unqualified name with `localhost/`, and Docker does not; both are the same tag.
func Reserved(tag string) (id, version string, ok bool) {
	name := strings.TrimPrefix(tag, "localhost/")
	rest, found := strings.CutPrefix(name, "yoke/")
	if !found {
		return "", "", false
	}
	id, version, ok = strings.Cut(rest, ":")
	return id, version, ok && id != "" && version != ""
}

// RemoveImage removes a reference: a tag goes, and an image goes once nothing refers to it. An image the
// engine declines to remove because something else still refers to it, and one already gone, are not
// failures.
func (e *Engine) RemoveImage(ctx context.Context, ref string) error {
	resp, err := e.do(ctx, http.MethodDelete, "/images/"+ref, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusConflict {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return refusal(resp, "remove "+ref)
}
