package compose

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ociArchive writes an OCI archive whose one image is named by a manifest holding says, and returns where
// it is and the manifest's digest.
func ociArchive(t *testing.T, says string) (string, string) {
	t.Helper()
	manifest := []byte(`{"schemaVersion":2,"says":"` + says + `"}`)
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(manifest))
	index, _ := json.Marshal(map[string]any{"schemaVersion": 2, "manifests": []map[string]any{{"digest": digest, "size": len(manifest)}}})
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for name, data := range map[string][]byte{"index.json": index, "oci-layout": []byte(`{"imageLayoutVersion":"1.0.0"}`), "blobs/sha256/" + digest[7:]: manifest} {
		w.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data))})
		w.Write(data)
	}
	w.Close()
	path := filepath.Join(t.TempDir(), "image.tar")
	os.WriteFile(path, b.Bytes(), 0o644)
	return path, digest
}

// pythonSource is a Python Plugin project whose build leaves the file at from as its program.
func pythonSource(t *testing.T, from string) string {
	t.Helper()
	file := strings.Replace(withRun(`cp "`+from+`" "$YOKE_OUT/greeter"`), "architectures: [x86_64, aarch64]", "architectures: ["+HostArch()+"]", 1)
	dir := pluginProject(t, strings.Replace(file, "language: go", "language: python", 1))
	git(t, dir, "init", "-q")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "the greeter")
	return dir
}

// std: yoke:a-python-plugin.01
func TestAPythonPluginIsTakenAsAnImagePinnedByItsManifestsDigest(t *testing.T) {
	app := authored(t)
	archive, digest := ociArchive(t, "one")
	source := pythonSource(t, archive)
	if code, out := takePlugin(t, "", source, "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	placed := filepath.Join(app, Components, "images", strings.Replace(digest, ":", "-", 1)+".tar")
	if raw(t, placed) != raw(t, archive) {
		t.Errorf("the archive is not at %s", placed)
	}
	if exists(filepath.Join(app, Components, "plugins", "greeter")) {
		t.Errorf("an executable was placed")
	}
	file := filepath.Join(app, Descriptor)
	if got, _ := written(t, file, "units.greeter.image").(string); !strings.HasSuffix(got, "@"+digest) {
		t.Errorf("the unit's image is %q, want it pinned by %s", got, digest)
	}
	if got := written(t, file, "units.greeter.exec"); got != nil {
		t.Errorf("the unit names exec %v", got)
	}
	entries, _ := readPlugins(app)
	if len(entries) != 1 || entries[0].Digest != digest {
		t.Errorf("the record is %+v", entries)
	}
	if !exists(filepath.Join(app, Manifests, "com.example.greeter.yaml")) {
		t.Errorf("no Manifest was placed")
	}
	if _, _, err := Build(app, BuildOptions{Out: t.TempDir()}); err != nil {
		t.Errorf("the application does not build: %v", err)
	}
}

// std: yoke:a-python-plugin.02
func TestABuildThatLeavesNoImageArchiveIsRefusedAndNothingIsPlaced(t *testing.T) {
	app := authored(t)
	notOne := filepath.Join(t.TempDir(), "not-one")
	os.WriteFile(notOne, []byte("not an archive"), 0o644)
	source := pythonSource(t, notOne)
	code, out := takePlugin(t, "", source, "--yes", "--dir", app)
	if code == Done || !strings.Contains(out, "OCI") {
		t.Errorf("add plugin: %d %s", code, out)
	}
	if exists(filepath.Join(app, Components, "images")) || exists(filepath.Join(app, Plugins)) {
		t.Errorf("something was placed")
	}
}

// std: yoke:a-python-plugin.03
func TestUpdateReplacesTheImageTheFirstTakingPlaced(t *testing.T) {
	app := authored(t)
	first, digest := ociArchive(t, "one")
	source := pythonSource(t, first)
	if code, out := takePlugin(t, "", source, "--yes", "--dir", app); code != Done {
		t.Fatalf("add plugin: %d %s", code, out)
	}
	second, newDigest := ociArchive(t, "two")
	data, _ := os.ReadFile(second)
	os.WriteFile(first, data, 0o644)
	if code, out := updating(t, time.Now(), "com.example.greeter", "--yes", "--dir", app); code != Done {
		t.Fatalf("update plugin: %d %s", code, out)
	}
	if exists(filepath.Join(app, Components, "images", strings.Replace(digest, ":", "-", 1)+".tar")) {
		t.Errorf("the first archive is still there")
	}
	if !exists(filepath.Join(app, Components, "images", strings.Replace(newDigest, ":", "-", 1)+".tar")) {
		t.Errorf("the second archive is not there")
	}
	if got, _ := written(t, filepath.Join(app, Descriptor), "units.greeter.image").(string); !strings.HasSuffix(got, "@"+newDigest) {
		t.Errorf("the unit's image is %q", got)
	}
	if _, _, err := Build(app, BuildOptions{Out: t.TempDir()}); err != nil {
		t.Errorf("the application does not build: %v", err)
	}
}
