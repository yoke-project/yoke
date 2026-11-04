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

// templated is a C family's checkout holding a template, tagged v0.5.0.
func templated(t *testing.T, withTemplate bool) (root, commit string) {
	t.Helper()
	root, _ = cFamily(t)
	if withTemplate {
		for path, content := range map[string]string{
			"template/yoke-plugin.yaml": "plugin: 1\nid: @YOKE_ID@\n",
			"template/plugin/main.c":    "int main(void) { return 0; }\n",
		} {
			os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755)
			os.WriteFile(filepath.Join(root, path), []byte(content), 0o644)
		}
		git(t, root, "add", "-A")
		git(t, root, "commit", "-q", "-m", "the template")
	}
	git(t, root, "tag", "v0.5.0")
	return root, git(t, root, "rev-parse", "HEAD")
}

// releaseTemplate runs the verb as the C family's source release with its template, keeping what it hands
// over.
func releaseTemplate(t *testing.T, root string) (int, []map[string]any, map[string][]byte, string) {
	t.Helper()
	files := map[string][]byte{}
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Proxy: (&proxy{}).serve, Out: &out, Err: &errs, SourceRelease: true,
		Source: "yoke-sdk-c", Template: "c", Releases: "https://github.com/yoke-project/yoke-sdk-c/releases/tag/",
		Upload: func(tag string, paths []string) error {
			for _, p := range paths {
				data, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				files[filepath.Base(p)] = data
			}
			return nil
		},
		Today: func() time.Time { return time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC) }})
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
	return code, lines, files, errs.String()
}

func templateEntries(t *testing.T, archive []byte) map[string]string {
	t.Helper()
	z, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	r := tar.NewReader(z)
	got := map[string]string{}
	for {
		h, err := r.Next()
		if err == io.EOF {
			return got
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			data, _ := io.ReadAll(r)
			got[h.Name] = string(data)
		}
	}
}

// std: yoke:the-plugin-template.01
func TestAFamilysReleaseTagHandsOverItsTemplate(t *testing.T) {
	root, commit := templated(t, true)
	code, lines, files, errs := releaseTemplate(t, root)
	if code != 0 {
		t.Fatalf("the verb failed: %s", errs)
	}
	archive, there := files["yoke-plugin-template-c-0.5.0.tar.gz"]
	if !there || files["yoke-sdk-c-0.5.0-source.tar.gz"] == nil {
		t.Fatalf("handed over: %v", handedNames(files))
	}
	want := map[string]string{"template/yoke-plugin.yaml": "plugin: 1\nid: @YOKE_ID@\n", "template/plugin/main.c": "int main(void) { return 0; }\n"}
	got := templateEntries(t, archive)
	if len(got) != len(want) {
		t.Errorf("the template holds %v", entryNames(got))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s holds %q", k, got[k])
		}
	}
	if len(lines) != 2 {
		t.Fatalf("lines: %v", lines)
	}
	sum := sha256.Sum256(archive)
	l := lines[1]
	if l["published"] != "yoke-plugin-template-c" || l["version"] != "v0.5.0" || l["commit"] != commit ||
		l["where"] != "https://github.com/yoke-project/yoke-sdk-c/releases/tag/v0.5.0" || l["authenticated"] != "https://github.com" {
		t.Errorf("the template's line: %v", l)
	}
	if d, _ := l["digests"].([]any); len(d) != 1 || d[0] != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Errorf("the template's digest: %v", l["digests"])
	}
}

// std: yoke:the-plugin-template.02
func TestATreeWithNoTemplateIsRefusedAndNothingIsHandedOver(t *testing.T) {
	root, _ := templated(t, false)
	code, lines, files, errs := releaseTemplate(t, root)
	if code == 0 || !strings.Contains(errs, "template/") {
		t.Errorf("a tree with no template: %d %s", code, errs)
	}
	if len(files) != 0 || len(lines) != 0 {
		t.Errorf("handed over %v, lines %v", handedNames(files), lines)
	}
}

// std: yoke:the-plugin-template.03
func TestMadeTwiceTheTemplatesArchiveIsTheSameBytes(t *testing.T) {
	root, _ := templated(t, true)
	_, _, first, _ := releaseTemplate(t, root)
	_, _, second, _ := releaseTemplate(t, root)
	name := "yoke-plugin-template-c-0.5.0.tar.gz"
	if first[name] == nil || !bytes.Equal(first[name], second[name]) {
		t.Errorf("the two archives differ")
	}
}

func entryNames(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
