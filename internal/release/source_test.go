package release_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/release"
)

// cFamily is a C family's checkout, tagged as given.
func cFamily(t *testing.T, tags ...string) (root, commit string) {
	t.Helper()
	root = t.TempDir()
	for path, content := range map[string]string{
		"CMakeLists.txt": "project(yoke-sdk-c VERSION 0.5.0 LANGUAGES C)\n", "LICENSE": "Apache License\n",
		"NOTICE": "Yk\n", "include/yoke/base.h": "#define YOKE_SDK_LINE \"yoke-sdk-c 0.5.0\"\n",
		"go.mod": "module example.com/c\n",
	} {
		os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
		os.WriteFile(filepath.Join(root, path), []byte(content), 0o644)
	}
	git(t, root, "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "one")
	for _, tag := range tags {
		git(t, root, "tag", tag)
	}
	return root, git(t, root, "rev-parse", "HEAD")
}

type handedOver struct {
	tag   string
	files map[string][]byte
}

// releaseSource runs the verb as the C family's source release, keeping what it hands over.
func releaseSource(t *testing.T, root string, p *proxy) (int, []map[string]any, *handedOver, string) {
	t.Helper()
	h := &handedOver{files: map[string][]byte{}}
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Proxy: p.serve, Out: &out, Err: &errs, SourceRelease: true,
		Source: "yoke-sdk-c", Releases: "https://github.com/yoke-project/yoke-sdk-c/releases/tag/",
		Upload: func(tag string, files []string) error {
			h.tag = tag
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					return err
				}
				h.files[filepath.Base(f)] = data
			}
			return nil
		},
		Today: func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }})
	var lines []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var l map[string]any
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatalf("a line is not one JSON object: %q", line)
		}
		lines = append(lines, l)
	}
	return code, lines, h, errs.String()
}

// std: yoke:the-source-release.01
func TestAFamilysReleaseTagHandsOverItsSourceArchive(t *testing.T) {
	root, commit := cFamily(t, "v0.5.0")
	code, lines, h, errs := releaseSource(t, root, &proxy{served: map[string]string{}})
	if code != 0 {
		t.Fatalf("the verb failed: %s", errs)
	}
	archive, there := h.files["yoke-sdk-c-0.5.0-source.tar.gz"]
	if !there || len(h.files) != 1 || h.tag != "v0.5.0" {
		t.Fatalf("handed over %v to %q", handedNames(h.files), h.tag)
	}
	z, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	r := tar.NewReader(z)
	var names []string
	for {
		hd, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if hd.Typeflag == tar.TypeXGlobalHeader {
			continue // git names the commit the archive was made from
		}
		names = append(names, hd.Name)
		if !strings.HasPrefix(hd.Name, "yoke-sdk-c-0.5.0/") {
			t.Errorf("%s is outside the archive's directory", hd.Name)
		}
	}
	if !strings.Contains(strings.Join(names, " "), "yoke-sdk-c-0.5.0/CMakeLists.txt") {
		t.Errorf("the archive holds %v", names)
	}
	if len(lines) != 1 {
		t.Fatalf("%d lines, want one", len(lines))
	}
	sum := sha256.Sum256(archive)
	l := lines[0]
	want := map[string]any{"published": "yoke-sdk-c-source", "version": "v0.5.0", "commit": commit,
		"where": "https://github.com/yoke-project/yoke-sdk-c/releases/tag/v0.5.0", "authenticated": "https://github.com",
		"licence": "Apache-2.0", "day": "2026-10-12"}
	for k, v := range want {
		if l[k] != v {
			t.Errorf("%s is %v, want %v", k, l[k], v)
		}
	}
	if d, _ := l["digests"].([]any); len(d) != 1 || d[0] != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Errorf("digests %v", l["digests"])
	}
	if n, _ := l["notices"].([]any); len(n) != 1 || n[0] != "NOTICE" {
		t.Errorf("notices %v", l["notices"])
	}
}

func handedNames(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// std: yoke:the-source-release.02
func TestASourceReleasePublishesNoModule(t *testing.T) {
	root, _ := cFamily(t, "v0.5.0", "proto/v0.5.0")
	p := &proxy{served: map[string]string{}}
	code, lines, h, errs := releaseSource(t, root, p)
	if code != 0 {
		t.Fatalf("the verb failed: %s", errs)
	}
	if len(p.asked) != 0 {
		t.Errorf("the proxy was asked for %v", p.asked)
	}
	if len(lines) != 1 || len(h.files) != 1 {
		t.Errorf("%d lines and %d files, want one each", len(lines), len(h.files))
	}
}

// std: yoke:the-source-release.03
func TestMadeTwiceTheSourceArchiveIsTheSameBytes(t *testing.T) {
	root, _ := cFamily(t, "v0.5.0")
	_, _, first, _ := releaseSource(t, root, &proxy{served: map[string]string{}})
	_, _, second, _ := releaseSource(t, root, &proxy{served: map[string]string{}})
	name := "yoke-sdk-c-0.5.0-source.tar.gz"
	if len(first.files[name]) == 0 || !bytes.Equal(first.files[name], second.files[name]) {
		t.Error("the two archives differ")
	}
}
