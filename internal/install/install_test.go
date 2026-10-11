package install

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/pkgfile"
	"github.com/yoke-project/yoke/internal/version"
)

const program = "#!/bin/sh\nexit 0\n"

const manifest = "manifest: 1\nid: com.yoke.station.acquire\nprotocol: 1\n"

// project writes a project of com.yoke.station at the version given, for the architecture given, with one
// Plugin unit and one unit that runs to completion. digest, where given, is stated for the executable in
// place of its own.
func project(t *testing.T, ver, arch, digest string) string {
	t.Helper()
	dir := t.TempDir()
	if digest == "" {
		digest = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(program)))
	}
	descriptor := fmt.Sprintf("model: 1\nid: com.yoke.station\nversion: %s\narch: %s\ncore: \">=0.1\"\nunits:\n"+
		"  acquire: { kind: plugin, plugin: com.yoke.station.acquire, exec: components/plugins/acquire, digest: \"%s\" }\n"+
		"  setup: { kind: oneshot, exec: components/setup, digest: \"%s\" }\n",
		ver, arch, digest, fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(program))))
	for name, body := range map[string]string{
		"yoke.yaml": descriptor, "manifests/com.yoke.station.acquire.yaml": manifest,
		"components/plugins/acquire": program, "components/setup": program,
	} {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(body), 0o755)
	}
	return dir
}

func build(t *testing.T, dir string, o compose.BuildOptions) string {
	t.Helper()
	o.Out = t.TempDir()
	path, report, err := compose.Build(dir, o)
	if err != nil {
		t.Fatalf("build: %v %v", err, report.Findings)
	}
	return path
}

func root(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "apps")
	os.Mkdir(dir, 0o755)
	os.Chmod(dir, 0o775|os.ModeSetgid)
	return dir
}

type trusted []crypto.PublicKey

func (k trusted) Key(id [32]byte) (crypto.PublicKey, bool) {
	for _, p := range k {
		if pkgfile.KeyID(p) == id {
			return p, true
		}
	}
	return nil, false
}

var pub, priv, _ = ed25519.GenerateKey(rand.Reader)

func options(apps, pkg string) Options {
	return Options{Package: pkg, Root: apps, Keys: trusted{pub},
		Host: pkgfile.Host{Arch: "aarch64", Models: []uint16{1}, Core: version.MustParse("0.5.0")},
		Now:  func() time.Time { return time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC) }}
}

func refused(t *testing.T, err error, names ...string) {
	t.Helper()
	var r *Refusal
	if !errors.As(err, &r) {
		t.Fatalf("not a refusal: %v", err)
	}
	for _, n := range names {
		if !strings.Contains(err.Error(), n) {
			t.Errorf("the refusal does not name %q: %v", n, err)
		}
	}
}

// std: yoke:the-installation.01
func TestAPackageIsInstalled(t *testing.T) {
	apps := root(t)
	pkg := build(t, project(t, "2.1.0", "aarch64", ""), compose.BuildOptions{Sign: priv})
	res, err := Install(options(apps, pkg))
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(apps, "com.yoke.station", "2.1.0", "bundle")
	if res.Dir != filepath.Dir(bundle) {
		t.Errorf("installed at %s", res.Dir)
	}
	for name, mode := range map[string]os.FileMode{"yoke.yaml": 0o644, "manifests/com.yoke.station.acquire.yaml": 0o644,
		"components/plugins/acquire": 0o755, "components/setup": 0o755} {
		info, err := os.Stat(filepath.Join(bundle, name))
		if err != nil || info.Mode().Perm() != mode {
			t.Errorf("%s: %v, mode %v, want %v", name, err, info, mode)
		}
	}
	rec, err := installroot.ReadRecord(res.Dir)
	if err != nil {
		t.Fatal(err)
	}
	id := pkgfile.KeyID(pub)
	data, _ := os.ReadFile(pkg)
	f, _ := pkgfile.ReadFooter(bytes.NewReader(data), int64(len(data)))
	if rec.State != installroot.Complete || rec.VerifiedBy != hex.EncodeToString(id[:]) || rec.Sealed ||
		rec.Digest != "sha256:"+hex.EncodeToString(f.Meta.Digest[:]) || !rec.InstalledAt.Equal(time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("the record is %+v", rec)
	}
}

// std: yoke:the-installation.02
func TestARefusalBeforeTheRootIsTouchedLeavesNothing(t *testing.T) {
	_, stranger, _ := ed25519.GenerateKey(rand.Reader)
	apps := root(t)
	for _, c := range []struct {
		pkg   string
		names string
	}{
		{build(t, project(t, "2.1.0", "aarch64", ""), compose.BuildOptions{Sign: stranger}), "trust"},
		{build(t, project(t, "2.1.0", "x86_64", ""), compose.BuildOptions{Sign: priv}), "x86_64"},
	} {
		_, err := Install(options(apps, c.pkg))
		refused(t, err, c.names)
	}
	o := options(apps, build(t, project(t, "2.1.0", "aarch64", ""), compose.BuildOptions{Sign: priv}))
	o.Host.Core = version.MustParse("0.0.9")
	_, err := Install(o)
	refused(t, err, ">=0.1")
	if entries, _ := os.ReadDir(apps); len(entries) != 0 {
		t.Errorf("the root holds %d entries", len(entries))
	}
}

// std: yoke:the-installation.03
func TestAFailureAfterTheDirectoryLeavesTheVersionIncomplete(t *testing.T) {
	apps := root(t)
	if _, err := Install(options(apps, build(t, project(t, "2.0.0", "aarch64", ""), compose.BuildOptions{Sign: priv}))); err != nil {
		t.Fatal(err)
	}
	altered := build(t, project(t, "2.1.0", "aarch64", ""), compose.BuildOptions{Sign: priv})
	data, _ := os.ReadFile(altered)
	data[40] ^= 0xff // inside the payload, which the footer's digest covers
	os.WriteFile(altered, data, 0o644)
	_, err := Install(options(apps, altered))
	refused(t, err, "payload")
	assertIncomplete(t, apps, "2.1.0")
	os.RemoveAll(filepath.Join(apps, "com.yoke.station", "2.1.0"))
	refusedByGate := project(t, "2.1.0", "aarch64", "")
	descriptor, _ := os.ReadFile(filepath.Join(refusedByGate, "yoke.yaml"))
	pkg := packWith(t, map[string]string{"yoke.yaml": strings.Replace(string(descriptor), "units:", "colour: red\nunits:", 1)}, "2.1.0")
	_, err = Install(options(apps, pkg))
	refused(t, err, "key.unknown")
	assertIncomplete(t, apps, "2.1.0")
	if rec, _ := installroot.ReadRecord(filepath.Join(apps, "com.yoke.station", "2.0.0")); rec.State != installroot.Complete {
		t.Errorf("2.0.0 is now %s", rec.State)
	}
}

func assertIncomplete(t *testing.T, apps, ver string) {
	t.Helper()
	rec, err := installroot.ReadRecord(filepath.Join(apps, "com.yoke.station", ver))
	if err != nil || rec.State != installroot.Incomplete {
		t.Errorf("%s: %+v %v, want incomplete", ver, rec, err)
	}
}

// packWith signs a package whose payload holds exactly the entries given, written raw.
func packWith(t *testing.T, entries map[string]string, ver string) string {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, body := range entries {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	z.Close()
	return packPayload(t, b.Bytes(), ver)
}

func packPayload(t *testing.T, payload []byte, ver string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), pkgfile.Name("com.yoke.station", ver))
	f, _ := os.Create(path)
	defer f.Close()
	if err := pkgfile.Write(f, payload, pkgfile.Options{Model: 1, ID: "com.yoke.station", Version: ver, Arch: "aarch64",
		Core: ">=0.1", BuiltAt: time.Now(), Sign: priv}); err != nil {
		t.Fatal(err)
	}
	return path
}

// std: yoke:the-installation.04
func TestAnEntryThatWouldLeaveTheBundleIsRefused(t *testing.T) {
	apps := root(t)
	escapes := map[string][]byte{}
	for _, name := range []string{"../escape", "/etc/escape"} {
		var b bytes.Buffer
		z := zip.NewWriter(&b)
		w, _ := z.CreateHeader(&zip.FileHeader{Name: name})
		w.Write([]byte("x"))
		z.Close()
		escapes[name] = b.Bytes()
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	h := &zip.FileHeader{Name: "components/link"}
	h.SetMode(os.ModeSymlink | 0o777)
	w, _ := z.CreateHeader(h)
	w.Write([]byte("/etc/passwd"))
	z.Close()
	escapes["components/link"] = b.Bytes()
	for name, payload := range escapes {
		os.RemoveAll(filepath.Join(apps, "com.yoke.station"))
		_, err := Install(options(apps, packPayload(t, payload, "2.1.0")))
		refused(t, err, name)
		if _, err := os.Lstat(filepath.Join(apps, "com.yoke.station", "2.1.0", "escape")); err == nil {
			t.Errorf("%s was written", name)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(apps), "escape")); err == nil {
		t.Error("an entry was written outside the root")
	}
}

// std: yoke:the-installation.05
func TestAComponentWhoseDigestDisagreesIsRefused(t *testing.T) {
	apps := root(t)
	pkg := build(t, project(t, "2.1.0", "aarch64", "sha256:"+strings.Repeat("ab", 32)), compose.BuildOptions{Sign: priv})
	_, err := Install(options(apps, pkg))
	refused(t, err, "component.digest", "components/plugins/acquire")
	assertIncomplete(t, apps, "2.1.0")
}

// std: yoke:the-installation.06
func TestAVersionAlreadyInstalledIsReported(t *testing.T) {
	apps := root(t)
	pkg := build(t, project(t, "2.1.0", "aarch64", ""), compose.BuildOptions{Sign: priv})
	if _, err := Install(options(apps, pkg)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(apps, "com.yoke.station", "2.1.0", installroot.RecordFile))
	res, err := Install(options(apps, pkg))
	if err != nil || !res.Present {
		t.Errorf("a second install: %+v %v", res, err)
	}
	after, _ := os.ReadFile(filepath.Join(apps, "com.yoke.station", "2.1.0", installroot.RecordFile))
	if !bytes.Equal(before, after) {
		t.Error("the record changed")
	}
}

// std: yoke:the-installation.07
func TestASealedPackageIsOpenedWithTheCredential(t *testing.T) {
	apps := root(t)
	credential := bytes.Repeat([]byte{4}, 32)
	pkg := build(t, project(t, "2.1.0", "aarch64", ""), compose.BuildOptions{Sign: priv, Seal: credential})
	_, err := Install(options(apps, pkg))
	refused(t, err, "credential")
	assertIncomplete(t, apps, "2.1.0")
	os.RemoveAll(filepath.Join(apps, "com.yoke.station", "2.1.0"))
	o := options(apps, pkg)
	o.Credential = credential
	res, err := Install(o)
	if err != nil {
		t.Fatal(err)
	}
	if rec, _ := installroot.ReadRecord(res.Dir); !rec.Sealed || rec.State != installroot.Complete {
		t.Errorf("the record is %+v", rec)
	}
}

// std: yoke:the-installation.08
func TestTheCoresVersionIsReadFromItsBinary(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "yoke-core")
	if said, err := exec.Command("go", "build", "-o", binary, "github.com/yoke-project/yoke/cmd/yoke-core").CombinedOutput(); err != nil {
		t.Fatalf("yoke-core does not build: %v\n%s", err, said)
	}
	v, err := CoreVersion(binary)
	if err != nil {
		t.Fatalf("the version of a Core built here: %v", err)
	}
	if _, err := version.Parse(v.String()); err != nil {
		t.Errorf("the version %q is not a semantic version", v)
	}
	other := filepath.Join(t.TempDir(), "not-a-core")
	os.WriteFile(other, []byte("#!/bin/sh\necho 9.9.9\n"), 0o755)
	if _, err := CoreVersion(other); err == nil || !strings.Contains(err.Error(), other) {
		t.Errorf("a file that is not a Go program: %v", err)
	}
}
