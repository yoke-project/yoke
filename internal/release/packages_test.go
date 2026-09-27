package release_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/packages"
	"github.com/yoke-project/yoke/internal/release"
)

// crateOf is a crate at version carrying the sources given, as cargo lays one out.
func crateOf(t *testing.T, version string, sources map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zipped := gzip.NewWriter(&buf)
	archive := tar.NewWriter(zipped)
	files := map[string]string{"Cargo.toml.orig": "[package]\nname = \"yoke-proto\"\nversion = \"" + version + "\"\n",
		"Cargo.toml": "# normalized by cargo\n", "LICENSE": "Apache License\n"}
	for name, content := range sources {
		files[name] = content
	}
	for name, content := range files {
		archive.WriteHeader(&tar.Header{Name: "yoke-proto-" + version + "/" + name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg})
		archive.Write([]byte(content))
	}
	archive.Close()
	zipped.Close()
	return buf.Bytes()
}

// wheelOf is a wheel at version carrying the sources given.
func wheelOf(t *testing.T, version string, sources map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)
	info := "yoke_proto-" + version + ".dist-info/"
	files := map[string]string{info + "METADATA": "Metadata-Version: 2.4\nName: yoke-proto\nVersion: " + version + "\n",
		info + "RECORD": "differs from build to build\n", info + "licenses/LICENSE": "Apache License\n"}
	for name, content := range sources {
		files[name] = content
	}
	for name, content := range files {
		w, _ := archive.Create(name)
		w.Write([]byte(content))
	}
	archive.Close()
	return buf.Bytes()
}

// The sources the tree's packages carry, in these tests.
var (
	crateSources = map[string]string{"src/lib.rs": "pub mod plugin {}\n"}
	wheelSources = map[string]string{"yoke/plugin/v1/register_pb2.py": "# generated\n"}
)

// packager packages the tree's sources at a version, and records the versions it was asked for.
type packager struct{ asked []string }

func (p *packager) make(t *testing.T) func(version, dir string) (string, string, error) {
	return func(version, dir string) (string, string, error) {
		p.asked = append(p.asked, version)
		crate := filepath.Join(dir, "yoke-proto-"+version+".crate")
		wheel := filepath.Join(dir, "yoke_proto-"+version+"-py3-none-any.whl")
		os.WriteFile(crate, crateOf(t, version, crateSources), 0o644)
		os.WriteFile(wheel, wheelOf(t, version, wheelSources), 0o644)
		return crate, wheel, nil
	}
}

// registry serves what it holds, publishes what it is given, and records both.
type registry struct {
	kind          packages.Language
	name, host    string
	held          map[string][]byte
	asked         int
	published     []string
	refuse        error
	heldOnRefusal []byte // what it begins to serve when it refuses: another run published first
}

func (r *registry) Published() string     { return r.name }
func (r *registry) Authenticated() string { return "https://" + r.host }
func (r *registry) Where(version string) string {
	return "https://" + r.host + "/yoke-proto/" + version
}
func (r *registry) Package() packages.Language { return r.kind }
func (r *registry) Served(version string) ([]byte, bool, error) {
	r.asked++
	data, ok := r.held[version]
	return data, ok, nil
}
func (r *registry) Publish(version, path string) error {
	if r.refuse != nil {
		if r.heldOnRefusal != nil {
			r.held[version] = r.heldOnRefusal
		}
		return r.refuse
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	r.published = append(r.published, version)
	r.held[version] = data
	return nil
}

func registries() (*registry, *registry) {
	return &registry{kind: packages.Rust, name: "crates.io/yoke-proto", host: "crates.io", held: map[string][]byte{}},
		&registry{kind: packages.Python, name: "pypi.org/yoke-proto", host: "pypi.org", held: map[string][]byte{}}
}

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// publishing runs the verb with registries, the definitions module served as its tree is.
func publishing(t *testing.T, root string, pkg *packager, only bool, regs ...release.Registry) (int, []map[string]any, string, string, *proxy) {
	t.Helper()
	p := &proxy{served: map[string]string{}}
	for _, tag := range strings.Fields(git(t, root, "tag", "--points-at", "HEAD")) {
		if v, ok := strings.CutPrefix(tag, "proto/"); ok {
			p.served["example.com/yk/proto@"+v] = treeHash(t, root, "example.com/yk/proto", v, tag, "proto")
		} else {
			p.served["example.com/yk@"+tag] = treeHash(t, root, "example.com/yk", tag, tag, "")
		}
	}
	var out, errs bytes.Buffer
	code := release.Run(release.Config{Root: root, Proxy: p.serve, Out: &out, Err: &errs, PackagesOnly: only,
		Packages: pkg.make(t), Registries: regs, Settle: 1,
		Today: func() time.Time { return time.Date(2026, 9, 26, 23, 30, 0, 0, time.UTC) }})
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
	return code, lines, out.String(), errs.String(), p
}

// named finds the line that names what was published.
func named(lines []map[string]any, published string) map[string]any {
	for _, l := range lines {
		if l["published"] == published {
			return l
		}
	}
	return nil
}

// std: yoke:the-packages-publication.01
func TestADefinitionsTagPublishesTheCrateAndTheWheel(t *testing.T) {
	root, commit := repositoryAnd(t, map[string]string{"NOTICE": "Yk\n"}, "proto/v0.2.0")
	crates, pypi := registries()
	pkg := &packager{}
	code, lines, _, errs, _ := publishing(t, root, pkg, false, crates, pypi)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errs)
	}
	if len(crates.published) != 1 || len(pypi.published) != 1 || len(pkg.asked) != 1 || pkg.asked[0] != "0.2.0" {
		t.Fatalf("published the crate %v and the wheel %v, packaged at %v", crates.published, pypi.published, pkg.asked)
	}
	if named(lines, "example.com/yk/proto") == nil || len(lines) != 3 {
		t.Errorf("the verb emitted %d lines, want the module's and two more: %v", len(lines), lines)
	}
	for _, r := range []*registry{crates, pypi} {
		l := named(lines, r.name)
		if l == nil {
			t.Errorf("no line names %s", r.name)
			continue
		}
		want := map[string]any{"line": "publication", "version": "v0.2.0", "commit": commit, "where": r.Where("0.2.0"),
			"authenticated": "https://" + r.host, "licence": "Apache-2.0", "day": "2026-09-26"}
		for field, value := range want {
			if l[field] != value {
				t.Errorf("%s's line says %s %v, want %v", r.name, field, l[field], value)
			}
		}
		if d, _ := l["digests"].([]any); len(d) != 1 || d[0] != digestOf(r.held["0.2.0"]) {
			t.Errorf("%s's line carries the digests %v, want the served file's", r.name, l["digests"])
		}
		if n, _ := l["notices"].([]any); len(n) != 1 || n[0] != "NOTICE" {
			t.Errorf("%s's line names the notices %v", r.name, l["notices"])
		}
	}
}

// std: yoke:the-packages-publication.02
func TestAVersionAlreadyServedIsRecordedAndNotPublishedAgain(t *testing.T) {
	root, _ := repository(t, "proto/v0.2.0")
	crates, pypi := registries()
	crates.held["0.2.0"] = crateOf(t, "0.2.0", crateSources)
	pypi.held["0.2.0"] = wheelOf(t, "0.2.0", wheelSources)
	code, lines, _, errs, _ := publishing(t, root, &packager{}, false, crates, pypi)
	if code != 0 || len(crates.published)+len(pypi.published) != 0 {
		t.Fatalf("exit %d, published %v and %v: %s", code, crates.published, pypi.published, errs)
	}
	if named(lines, crates.name) == nil || named(lines, pypi.name) == nil {
		t.Errorf("the lines are %v", lines)
	}
}

// std: yoke:the-packages-publication.03
func TestAServedPackageWhoseSourcesDifferIsRefused(t *testing.T) {
	root, _ := repository(t, "proto/v0.2.0")
	crates, pypi := registries()
	crates.held["0.2.0"] = crateOf(t, "0.2.0", map[string]string{"src/lib.rs": "pub mod other {}\n"})
	code, _, out, errs, _ := publishing(t, root, &packager{}, false, crates, pypi)
	if code == 0 || out != "" || !strings.Contains(errs, crates.name) || !strings.Contains(errs, "src/lib.rs") {
		t.Errorf("exit %d, emitted %q, said %q", code, out, errs)
	}
}

// std: yoke:the-packages-publication.04
func TestAFailedPublicationFailsTheVerbAfterTheOtherWasTried(t *testing.T) {
	root, _ := repository(t, "proto/v0.2.0")
	crates, pypi := registries()
	crates.refuse = errors.New("no crates.io credential")
	code, _, out, errs, _ := publishing(t, root, &packager{}, false, crates, pypi)
	if code == 0 || out != "" || !strings.Contains(errs, crates.name) || !strings.Contains(errs, "no crates.io credential") {
		t.Errorf("exit %d, emitted %q, said %q", code, out, errs)
	}
	if len(pypi.published) != 1 {
		t.Errorf("the wheel was published %v, want once all the same", pypi.published)
	}
}

// std: yoke:the-packages-publication.05
func TestAPublicationRefusedBecauseTheVersionIsServedIsRecorded(t *testing.T) {
	root, _ := repository(t, "proto/v0.2.0")
	crates, pypi := registries()
	crates.refuse = errors.New("crate version 0.2.0 is already uploaded")
	crates.heldOnRefusal = crateOf(t, "0.2.0", crateSources)
	code, lines, _, errs, _ := publishing(t, root, &packager{}, false, crates, pypi)
	if code != 0 || named(lines, crates.name) == nil {
		t.Errorf("exit %d, lines %v: %s", code, lines, errs)
	}
}

// std: yoke:the-packages-publication.06
func TestAProgramsTagAlonePublishesNoPackage(t *testing.T) {
	root, _ := repository(t, "v0.1.0")
	crates, pypi := registries()
	pkg := &packager{}
	code, lines, _, errs, _ := publishing(t, root, pkg, false, crates, pypi)
	if code != 0 || crates.asked+pypi.asked != 0 || len(pkg.asked) != 0 {
		t.Fatalf("exit %d, asked %d and %d, packaged %v: %s", code, crates.asked, pypi.asked, pkg.asked, errs)
	}
	if named(lines, crates.name) != nil || named(lines, pypi.name) != nil {
		t.Errorf("a line names a package: %v", lines)
	}
}

// index is a package index offering a workflow identity, minting credentials and taking uploads.
type index struct {
	requested, minted string
	upload            struct{ user, password, name, version, digest, filename string }
}

func (x *index) serve(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/identity":
			if r.Header.Get("Authorization") != "bearer request-token" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			x.requested = r.URL.Query().Get("audience")
			fmt.Fprint(w, `{"value":"identity-of-the-run"}`)
		case r.URL.Path == "/_/oidc/audience":
			fmt.Fprint(w, `{"audience":"the-index"}`)
		case r.URL.Path == "/_/oidc/mint-token" && r.Method == http.MethodPost:
			var body struct{ Token string }
			json.NewDecoder(r.Body).Decode(&body)
			x.minted = body.Token
			fmt.Fprint(w, `{"success":true,"token":"pypi-minted"}`)
		case r.URL.Path == "/legacy/" && r.Method == http.MethodPost:
			x.upload.user, x.upload.password, _ = r.BasicAuth()
			_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			form := multipart.NewReader(r.Body, params["boundary"])
			for {
				part, err := form.NextPart()
				if err != nil {
					break
				}
				value, _ := io.ReadAll(part)
				switch part.FormName() {
				case "name":
					x.upload.name = string(value)
				case "version":
					x.upload.version = string(value)
				case "sha256_digest":
					x.upload.digest = string(value)
				case "content":
					x.upload.filename = part.FileName()
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

// std: yoke:the-packages-publication.07
func TestTheWheelIsPublishedUnderACredentialMintedFromTheRunsIdentity(t *testing.T) {
	x := &index{}
	server := x.serve(t)
	defer server.Close()
	env := map[string]string{
		"ACTIONS_ID_TOKEN_REQUEST_URL":   server.URL + "/identity?api-version=2.0",
		"ACTIONS_ID_TOKEN_REQUEST_TOKEN": "request-token",
		"PYPI_TOKEN":                     "",
	}
	pypi := release.PyPI{Index: server.URL, Upload: server.URL + "/legacy/", Getenv: func(name string) string { return env[name] }}
	wheel := filepath.Join(t.TempDir(), "yoke_proto-0.2.0-py3-none-any.whl")
	data := wheelOf(t, "0.2.0", wheelSources)
	os.WriteFile(wheel, data, 0o644)
	if err := pypi.Publish("0.2.0", wheel); err != nil {
		t.Fatal(err)
	}
	if x.requested != "the-index" || x.minted != "identity-of-the-run" {
		t.Errorf("the identity was requested for %q and %q was minted from", x.requested, x.minted)
	}
	sum := sha256.Sum256(data)
	u := x.upload
	if u.user != "__token__" || u.password != "pypi-minted" || u.name != "yoke-proto" || u.version != "0.2.0" ||
		u.digest != hex.EncodeToString(sum[:]) || u.filename != filepath.Base(wheel) {
		t.Errorf("the upload was %+v", u)
	}
}

// std: yoke:the-packages-publication.08
func TestTheReleaseRunOffersWhatTrustedPublishingNeeds(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Permissions map[string]string
		Jobs        map[string]struct {
			Steps []struct {
				ID              string `yaml:"id"`
				Uses            string
				Run             string
				ContinueOnError bool `yaml:"continue-on-error"`
				Env             map[string]string
			}
		}
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	if workflow.Permissions["id-token"] != "write" {
		t.Errorf("the run may not request an identity token: %v", workflow.Permissions)
	}
	exchange, verb := -1, -1
	var id string
	steps := workflow.Jobs["release"].Steps
	for i, s := range steps {
		if strings.HasPrefix(s.Uses, "rust-lang/crates-io-auth-action@") {
			exchange, id = i, s.ID
			if !s.ContinueOnError {
				t.Errorf("a refused exchange fails the run")
			}
		}
		if strings.Contains(s.Run, "just release") {
			verb = i
		}
	}
	if exchange < 0 || verb < 0 || exchange > verb || id == "" {
		t.Fatalf("the exchange is step %d, with id %q, and the verb step %d", exchange, id, verb)
	}
	if got := steps[verb].Env["CARGO_REGISTRY_TOKEN"]; got != "${{ steps."+id+".outputs.token }}" {
		t.Errorf("the verb is given %q as its crates.io credential", got)
	}
}

// std: yoke:the-packages-publication.09
func TestThePackagesAloneCanBePublishedByHand(t *testing.T) {
	root, _ := repository(t, "v0.1.0", "proto/v0.2.0")
	crates, pypi := registries()
	code, lines, _, errs, p := publishing(t, root, &packager{}, true, crates, pypi)
	if code != 0 || len(p.asked) != 0 {
		t.Fatalf("exit %d, the proxy was asked %v: %s", code, p.asked, errs)
	}
	if len(lines) != 2 || named(lines, crates.name) == nil || named(lines, pypi.name) == nil {
		t.Errorf("the lines are %v", lines)
	}
}
