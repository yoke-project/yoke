package core_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pluginv1 "github.com/yoke-project/yoke/proto/yoke/plugin/v1"
)

// std: yoke:the-centre.01
func TestNoPackageOfTheCoreImportsAUnitOrNamesOne(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}} {{.Standard}}", "../../cmd/yoke-core").Output()
	if err != nil {
		t.Fatalf("the packages yoke-core is built from cannot be listed: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		path, standard, _ := strings.Cut(line, " ")
		if standard == "true" || strings.HasPrefix(path, "github.com/yoke-project/yoke/") || path == "github.com/yoke-project/yoke" {
			continue
		}
		if strings.Contains(path, "yoke-reference") || strings.Contains(path, "yoke-sdk") || strings.Contains(path, "station") {
			t.Errorf("yoke-core is built from %s, which is a unit's", path)
		}
	}
	err = filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, id := range []string{"com.yoke.station", "com.yoke.example"} {
			if strings.Contains(string(data), id) {
				t.Errorf("%s names %s, a Plugin the reference publishes", path, id)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// std: yoke:the-centre.02
func TestWhatAUnitSendsNamesNoDestination(t *testing.T) {
	fields := (&pluginv1.Envelope{}).ProtoReflect().Descriptor().Fields()
	header := map[string]bool{"message_id": true, "session_id": true, "sent_at_unix_nano": true, "correlation_id": true}
	families := 0
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		switch {
		case f.ContainingOneof() != nil:
			families++
		case !header[string(f.Name())]:
			t.Errorf("the envelope carries %s, which is not one of its four header fields", f.Name())
		}
	}
	if families != 8 {
		t.Errorf("the envelope's payload is one of %d families, want eight", families)
	}
}
