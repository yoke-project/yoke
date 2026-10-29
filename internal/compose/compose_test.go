package compose

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/pkgfile"
	"github.com/yoke-project/yoke/internal/version"
)

const head = "model: 1\nid: com.yoke.station\nversion: 2.1.0\narch: aarch64\ncore: \">=2.0 <3.0\"\n"

// project writes a project whose one unit runs components/acquire, with the files given beside it.
func project(t *testing.T, exec string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	body := "#!/bin/sh\nexit 0\n"
	descriptor := head + fmt.Sprintf("units:\n  acquire: { kind: oneshot, exec: %s, digest: \"sha256:%x\" }\n", exec, sha256.Sum256([]byte(body)))
	all := map[string]string{"yoke.yaml": descriptor, "components/acquire": body, "manifests/.keep": ""}
	for k, v := range files {
		all[k] = v
	}
	for name, content := range all {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if strings.HasPrefix(name, "components/") {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func build(t *testing.T, dir string, o BuildOptions) string {
	t.Helper()
	if o.Out == "" {
		o.Out = t.TempDir()
	}
	path, report, err := Build(dir, o)
	if err != nil {
		t.Fatalf("build: %v %v", err, report.Findings)
	}
	return path
}

func read(t *testing.T, path string) (*pkgfile.Footer, []byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := pkgfile.ReadFooter(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	return f, b
}

// std: yoke:the-composer.01
func TestInitWritesADescriptorAndTheDirectoriesBesideIt(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, "com.yoke.station", InitOptions{Arch: "aarch64"}); err != nil {
		t.Fatal(err)
	}
	r, _ := gate.Check(gate.Input{Document: gate.Read(filepath.Join(dir, "yoke.yaml"), gate.Descriptor)})
	for _, f := range r.Findings {
		if f.Code != "field.required" || f.Location != "units" {
			t.Errorf("the descriptor init wrote: %s %s %s", f.Code, f.Location, f.Message)
		}
	}
	written, _ := os.ReadFile(filepath.Join(dir, "yoke.yaml"))
	for _, key := range []string{"model: 1", "id: com.yoke.station", "version:", "arch: aarch64", "core:"} {
		if !strings.Contains(string(written), key) {
			t.Errorf("the descriptor does not state %q:\n%s", key, written)
		}
	}
	for _, sub := range []string{"manifests", "components"} {
		if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
			t.Errorf("%s/ is not there: %v", sub, err)
		}
	}
	if err := Init(dir, "com.yoke.other", InitOptions{Arch: "aarch64"}); err == nil || !strings.Contains(err.Error(), "yoke.yaml") {
		t.Errorf("init over a descriptor: %v", err)
	}
	again, _ := os.ReadFile(filepath.Join(dir, "yoke.yaml"))
	if !bytes.Equal(again, written) {
		t.Error("the descriptor was changed")
	}
	if err := Init(t.TempDir(), "Station", InitOptions{Arch: "aarch64"}); err == nil || !strings.Contains(err.Error(), "Station") {
		t.Errorf("init Station: %v", err)
	}
}

// std: yoke:the-composer.02
func TestCheckRunsTheGateComposing(t *testing.T) {
	bad := project(t, "components/acquire", map[string]string{
		"yoke.yaml": head + "colour: red\nunits:\n  acquire: { kind: oneshot }\n",
	})
	r := Check(bad)
	if !r.Refused() {
		t.Fatal("the project was not refused")
	}
	var got []string
	for _, f := range r.Findings {
		got = append(got, f.Code+"@"+f.Location)
	}
	slices.Sort(got)
	if want := []string{"key.unknown@colour", "unit.program.ambiguous@units.acquire"}; !slices.Equal(got, want) {
		t.Errorf("findings %v, want %v", got, want)
	}
	good := Check(project(t, "components/acquire", nil))
	if good.Refused() {
		t.Errorf("a good project was refused: %v", good.Findings)
	}
	if !slices.Contains(good.NotRun, gate.PhaseHostFacts) {
		t.Errorf("host facts ran while composing: not run %v", good.NotRun)
	}
}

// std: yoke:the-composer.03
func TestBuildRefusesOnAnyRefusalAndWritesNothing(t *testing.T) {
	dir := project(t, "components/acquire", map[string]string{"yoke.yaml": head + "units:\n  acquire: { kind: oneshot }\n"})
	out := t.TempDir()
	_, report, err := Build(dir, BuildOptions{Out: out})
	if !errors.Is(err, ErrRefused) || !report.Refused() {
		t.Fatalf("build of a refused project: %v %v", err, report.Findings)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Errorf("%d files were written", len(entries))
	}
}

// std: yoke:the-composer.04
func TestBuildRefusesAPathOutsideTheBundleAndAFileNotThere(t *testing.T) {
	for _, exec := range []string{"src/acquire", "../acquire", "/usr/bin/acquire", "components/absent"} {
		dir := project(t, exec, map[string]string{"src/acquire": "x"})
		out := t.TempDir()
		_, _, err := Build(dir, BuildOptions{Out: out})
		if err == nil || !strings.Contains(err.Error(), "acquire") || !strings.Contains(err.Error(), exec) {
			t.Errorf("%s: %v", exec, err)
		}
		if entries, _ := os.ReadDir(out); len(entries) != 0 {
			t.Errorf("%s: %d files were written", exec, len(entries))
		}
	}
}

// std: yoke:the-composer.05
func TestTwoBuildsOfOneTreeProduceOneDigest(t *testing.T) {
	dir := project(t, "components/acquire", nil)
	first, _ := read(t, build(t, dir, BuildOptions{}))
	later := time.Now().Add(48 * time.Hour)
	filepath.Walk(dir, func(p string, _ os.FileInfo, _ error) error { return os.Chtimes(p, later, later) })
	second, payload := read(t, build(t, dir, BuildOptions{}))
	if first.Meta.Digest != second.Meta.Digest {
		t.Error("two builds of one tree differ")
	}
	z, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range z.File {
		names = append(names, e.Name)
		if !e.Modified.Equal(z.File[0].Modified) {
			t.Errorf("%s is dated %v", e.Name, e.Modified)
		}
		want := os.FileMode(0o644)
		if e.Name == "components/acquire" {
			want = 0o755
		}
		if e.Mode() != want {
			t.Errorf("%s carries mode %v, want %v", e.Name, e.Mode(), want)
		}
	}
	if !slices.IsSorted(names) {
		t.Errorf("entries are not in a fixed order: %v", names)
	}
}

// std: yoke:the-composer.06
func TestThePackageIsTheBundleAndNothingElse(t *testing.T) {
	dir := project(t, "components/acquire", map[string]string{
		"advisories.yaml": "waived: []\n", "blocks.yaml": "blocks: []\n", "src/acquire.go": "package main\n",
	})
	path := build(t, dir, BuildOptions{})
	if filepath.Base(path) != "com.yoke.station-2.1.0.yoke" {
		t.Errorf("named %s", filepath.Base(path))
	}
	f, payload := read(t, path)
	m := f.Meta
	if m.Model != 1 || m.ID != "com.yoke.station" || m.Version != "2.1.0" || m.Arch != "aarch64" || m.Core != ">=2.0 <3.0" {
		t.Errorf("the footer carries %+v", m)
	}
	if m.SigID != 0 || m.SealID != 0 || m.KeyID != [32]byte{} {
		t.Errorf("signed or sealed: %d %d", m.SigID, m.SealID)
	}
	z, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range z.File {
		if e.Name != "yoke.yaml" && !strings.HasPrefix(e.Name, "manifests/") && !strings.HasPrefix(e.Name, "components/") {
			t.Errorf("%s is in the payload", e.Name)
		}
	}
}

// std: yoke:the-composer.07
func TestBuildSignsAndSealsWithWhatItIsGiven(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	dir := project(t, "components/acquire", nil)
	host := pkgfile.Host{Arch: "aarch64", Models: []uint16{1}, Core: version.MustParse("2.4.0")}
	signed, _ := read(t, build(t, dir, BuildOptions{Sign: priv}))
	if err := signed.Check(trusting{pub}, pkgfile.Policy{RequireSignature: true}, host); err != nil {
		t.Errorf("the signature: %v", err)
	}
	credential := bytes.Repeat([]byte{3}, 32)
	sealed, file := read(t, build(t, dir, BuildOptions{Sign: priv, Seal: credential}))
	if sealed.Meta.SealID != pkgfile.SealAES256GCM {
		t.Fatalf("seal_id %d", sealed.Meta.SealID)
	}
	if _, err := sealed.Open(bytes.NewReader(file), credential); err != nil {
		t.Errorf("opened with the credential: %v", err)
	}
	if sealed.Meta.Digest != signed.Meta.Digest {
		t.Error("sealing changed the payload digest")
	}
}

type trusting struct{ pub ed25519.PublicKey }

func (k trusting) Key(id [32]byte) (crypto.PublicKey, bool) {
	if pkgfile.KeyID(k.pub) == id {
		return k.pub, true
	}
	return nil, false
}

// std: yoke:the-composer.08
func TestBuildRunsNothingTheProjectContains(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	dir := project(t, "components/acquire", map[string]string{"components/acquire": "#!/bin/sh\ntouch " + marker + "\n"})
	os.WriteFile(filepath.Join(dir, "yoke.yaml"), []byte(head+fmt.Sprintf(
		"units:\n  acquire: { kind: oneshot, exec: components/acquire, digest: \"sha256:%x\" }\n",
		sha256.Sum256([]byte("#!/bin/sh\ntouch "+marker+"\n")))), 0o644)
	build(t, dir, BuildOptions{})
	if _, err := os.Stat(marker); err == nil {
		t.Error("the component was run")
	}
}

// std: yoke:the-composer.10
func TestAnImagesArchiveIsPackagedAndOneNoImagePinsIsRefused(t *testing.T) {
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("an image")))
	archive := "components/images/" + strings.Replace(digest, ":", "-", 1) + ".tar"
	dir := project(t, "components/acquire", map[string]string{archive: "an OCI archive"})
	descriptor, _ := os.ReadFile(filepath.Join(dir, "yoke.yaml"))
	os.WriteFile(filepath.Join(dir, "yoke.yaml"), append(descriptor, []byte("  panel: { kind: interface, image: \"localhost/panel@"+digest+"\" }\n")...), 0o644)
	_, payload := read(t, build(t, dir, BuildOptions{}))
	z, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range z.File {
		found = found || e.Name == archive
	}
	if !found {
		t.Errorf("the archive %s is not in the payload", archive)
	}
	stray := "components/images/sha256-" + strings.Repeat("0", 64) + ".tar"
	os.WriteFile(filepath.Join(dir, filepath.FromSlash(stray)), []byte("x"), 0o644)
	if _, _, err := Build(dir, BuildOptions{Out: t.TempDir()}); err == nil || !strings.Contains(err.Error(), stray) {
		t.Errorf("an archive no image pins: %v", err)
	}
}

// std: yoke:the-composer.12
func TestBuildNamesTheOutDirectoryThatIsNotThere(t *testing.T) {
	dir := project(t, "components/acquire", nil)
	missing := filepath.Join(t.TempDir(), "pkgs")
	_, report, err := Build(dir, BuildOptions{Out: missing})
	if !IsRefusal(err) || !strings.Contains(err.Error(), missing) {
		t.Errorf("a missing --out is refused naming it: %v", err)
	}
	if len(report.NotRun) != 0 {
		t.Errorf("the refusal came after the gate ran: %v", report.NotRun)
	}
	if _, statErr := os.Stat(missing); statErr == nil {
		t.Error("the directory was made")
	}
}

// std: yoke:the-composer.13
func TestAComponentThatChangedIsRefusedByCheckAndBuild(t *testing.T) {
	dir := project(t, "components/acquire", nil)
	if err := os.WriteFile(filepath.Join(dir, "components", "acquire"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	digest := func(r gate.Report) bool {
		for _, f := range r.Findings {
			if f.Code == "component.digest" && strings.Count(f.Message, "sha256:") == 2 {
				return true
			}
		}
		return false
	}
	if r := Check(dir); !r.Refused() || !digest(r) {
		t.Errorf("check does not refuse the changed component: %v", r.Findings)
	}
	out := t.TempDir()
	path, report, err := Build(dir, BuildOptions{Out: out})
	if err == nil || !digest(report) || path != "" {
		t.Errorf("build is not refused with component.digest: %q %v %v", path, err, report.Findings)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Errorf("build wrote %v", entries)
	}
}
