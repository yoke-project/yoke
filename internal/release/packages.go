package release

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yoke-project/yoke/internal/packages"
)

// A Registry is where one of the definitions packages is published, and what authenticates it there.
type Registry interface {
	Published() string           // what the manifest names the publication
	Authenticated() string       // the mechanism a reader verifies it by
	Where(version string) string // the registry's page for the version
	Package() packages.Language  // which of the two packages it takes
	// Served returns the file the registry serves at version, and whether it serves one.
	Served(version string) ([]byte, bool, error)
	Publish(version, path string) error
}

// publishPackages publishes the definitions packages at the version a definitions tag carries, and
// returns a line for each. A version a registry already serves is not published again; whatever is
// served is compared with the tree's package before a line names it. Every registry is tried, and a
// failure at one fails the whole with no line.
func (cfg Config) publishPackages(version, commit string, notices []string) ([]Line, error) {
	bare := strings.TrimPrefix(version, "v")
	dir, err := os.MkdirTemp("", "yoke-release-packages-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	crate, wheel, err := cfg.Packages(bare, dir)
	if err != nil {
		return nil, fmt.Errorf("the packages cannot be made at %s: %v", bare, err)
	}

	var lines []Line
	var failures []string
	for _, r := range cfg.Registries {
		file := crate
		if r.Package() == packages.Python {
			file = wheel
		}
		served, err := cfg.served(r, bare, file)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", r.Published(), err))
			continue
		}
		if differ, err := differing(r.Package(), file, served); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", r.Published(), err))
			continue
		} else if len(differ) > 0 {
			failures = append(failures, fmt.Sprintf("%s serves %s with sources other than the tree's: %s",
				r.Published(), bare, strings.Join(differ, ", ")))
			continue
		}
		sum := sha256.Sum256(served)
		lines = append(lines, Line{Line: "publication", Published: r.Published(), Version: version, Commit: commit,
			Digests: []string{"sha256:" + hex.EncodeToString(sum[:])}, Where: r.Where(bare),
			Authenticated: r.Authenticated(), Licence: licence, Notices: notices,
			Day: cfg.Today().UTC().Format(time.DateOnly)})
	}
	if len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "\n"))
	}
	return lines, nil
}

// served returns what the registry serves at version, publishing file first when it serves nothing. A
// publication the registry refuses is still taken when the version is served afterwards: the other run
// of a commit carrying two tags may have published it in between.
func (cfg Config) served(r Registry, version, file string) ([]byte, error) {
	data, found, err := r.Served(version)
	if err != nil || found {
		return data, err
	}
	published := r.Publish(version, file)
	for try := 0; try < max(cfg.Settle, 1); try++ {
		if try > 0 {
			time.Sleep(cfg.Pause)
		}
		if data, found, err = r.Served(version); err == nil && found {
			return data, nil
		}
	}
	if published != nil {
		return nil, fmt.Errorf("%s was not published: %v", version, published)
	}
	if err != nil {
		return nil, fmt.Errorf("%s was published, and asking for it failed: %v", version, err)
	}
	return nil, fmt.Errorf("%s was published, and is not served", version)
}

// differing names the files where what is served and the tree's package differ, both ways.
func differing(lang packages.Language, built string, served []byte) ([]string, error) {
	want, err := packages.Carried(lang, built)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "yoke-release-served-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(served); err != nil {
		file.Close()
		return nil, err
	}
	file.Close()
	got, err := packages.Carried(lang, file.Name())
	if err != nil {
		return nil, fmt.Errorf("what is served is not a package: %v", err)
	}
	var names []string
	for name, data := range want {
		if other, ok := got[name]; !ok || !bytes.Equal(data, other) {
			names = append(names, name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// The name both packages carry, and what a registry is told of who asks.
const (
	packageName = "yoke-proto"
	userAgent   = "yoke-release (https://github.com/yoke-project/yoke)"
)

// fetch reads what url answers, and whether it answered at all: a 404 is an answer of nothing.
func fetch(client *http.Client, address string) ([]byte, bool, error) {
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		return nil, false, err
	}
	request.Header.Set("User-Agent", userAgent)
	response, err := client.Do(request)
	if err != nil {
		return nil, false, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, false, err
	}
	switch {
	case response.StatusCode == http.StatusNotFound:
		return nil, false, nil
	case response.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("%s answered %s", address, response.Status)
	}
	return body, true, nil
}

func getenv(f func(string) string) func(string) string {
	if f == nil {
		return os.Getenv
	}
	return f
}

// Crates is crates.io. A crate is published by cargo, in the pinned Rust image, under the credential
// CARGO_REGISTRY_TOKEN holds — the one the release run's identity is exchanged for, or the maintainer's.
type Crates struct {
	API, Static string // https://crates.io, https://static.crates.io
	Root        string // the checkout the crate is published from
	Getenv      func(string) string
	Client      *http.Client
}

func (c Crates) Published() string          { return "crates.io/" + packageName }
func (c Crates) Authenticated() string      { return c.API }
func (c Crates) Package() packages.Language { return packages.Rust }
func (c Crates) Where(version string) string {
	return c.API + "/crates/" + packageName + "/" + version
}

func (c Crates) Served(version string) ([]byte, bool, error) {
	if _, found, err := fetch(c.Client, c.API+"/api/v1/crates/"+packageName+"/"+version); err != nil || !found {
		return nil, false, err
	}
	return fetch(c.Client, fmt.Sprintf("%s/crates/%s/%s-%s.crate", c.Static, packageName, packageName, version))
}

func (c Crates) Publish(version, _ string) error {
	token := getenv(c.Getenv)("CARGO_REGISTRY_TOKEN")
	if token == "" {
		return errors.New("no crates.io credential: CARGO_REGISTRY_TOKEN is empty — the run's identity was not exchanged, " +
			"which crates.io refuses until the crate exists and trusts this repository's release workflow")
	}
	command := exec.Command("bash", "ci/definitions.sh", "publish-crate", version)
	command.Dir = c.Root
	command.Env = append(os.Environ(), "CARGO_REGISTRY_TOKEN="+token)
	if said, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%v\n%s", err, said)
	}
	return nil
}

// PyPI is the Python package index. A wheel is uploaded under the credential PYPI_TOKEN holds, or else
// one the index mints for the release run's identity, which lives as long as the run.
type PyPI struct {
	Index  string // https://pypi.org
	Upload string // https://upload.pypi.org/legacy/
	Getenv func(string) string
	Client *http.Client
}

func (p PyPI) Published() string          { return "pypi.org/" + packageName }
func (p PyPI) Authenticated() string      { return p.Index }
func (p PyPI) Package() packages.Language { return packages.Python }
func (p PyPI) Where(version string) string {
	return p.Index + "/project/" + packageName + "/" + version + "/"
}

func wheelName(version string) string { return "yoke_proto-" + version + "-py3-none-any.whl" }

func (p PyPI) Served(version string) ([]byte, bool, error) {
	body, found, err := fetch(p.Client, p.Index+"/pypi/"+packageName+"/"+version+"/json")
	if err != nil || !found {
		return nil, false, err
	}
	var release struct {
		URLs []struct{ Filename, URL string } `json:"urls"`
	}
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, false, fmt.Errorf("the index's answer for %s is not JSON: %v", version, err)
	}
	for _, f := range release.URLs {
		if f.Filename == wheelName(version) {
			return fetch(p.Client, f.URL)
		}
	}
	return nil, false, nil
}

func (p PyPI) Publish(version, path string) error {
	token := getenv(p.Getenv)("PYPI_TOKEN")
	if token == "" {
		minted, err := p.mint()
		if err != nil {
			return err
		}
		token = minted
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	metadata, err := wheelMetadata(path, version)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	fields := [][2]string{{":action", "file_upload"}, {"protocol_version", "1"}, {"filetype", "bdist_wheel"},
		{"pyversion", "py3"}, {"sha256_digest", hex.EncodeToString(sum[:])}}
	fields = append(fields, metadata...)
	for _, f := range fields {
		form.WriteField(f[0], f[1])
	}
	content, _ := form.CreateFormFile("content", filepath.Base(path))
	content.Write(data)
	form.Close()
	request, err := http.NewRequest(http.MethodPost, p.Upload, &body)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("User-Agent", userAgent)
	request.SetBasicAuth("__token__", token)
	return answered(p.Client, request, nil)
}

// mint exchanges the release run's identity for a credential the index issues for it.
func (p PyPI) mint() (string, error) {
	env := getenv(p.Getenv)
	requestURL, requestToken := env("ACTIONS_ID_TOKEN_REQUEST_URL"), env("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if requestURL == "" || requestToken == "" {
		return "", errors.New("no PyPI credential: PYPI_TOKEN is empty and the run offers no identity to exchange")
	}
	var audience struct{ Audience string }
	body, found, err := fetch(p.Client, p.Index+"/_/oidc/audience")
	if err != nil || !found || json.Unmarshal(body, &audience) != nil || audience.Audience == "" {
		return "", fmt.Errorf("the index names no audience for an identity: %v", err)
	}
	identityURL, err := url.Parse(requestURL)
	if err != nil {
		return "", err
	}
	query := identityURL.Query()
	query.Set("audience", audience.Audience)
	identityURL.RawQuery = query.Encode()
	request, _ := http.NewRequest(http.MethodGet, identityURL.String(), nil)
	request.Header.Set("Authorization", "bearer "+requestToken)
	var identity struct{ Value string }
	if err := answered(p.Client, request, &identity); err != nil || identity.Value == "" {
		return "", fmt.Errorf("the run's identity could not be read: %v", err)
	}
	exchange, _ := json.Marshal(map[string]string{"token": identity.Value})
	request, _ = http.NewRequest(http.MethodPost, p.Index+"/_/oidc/mint-token", bytes.NewReader(exchange))
	request.Header.Set("Content-Type", "application/json")
	var minted struct{ Token string }
	if err := answered(p.Client, request, &minted); err != nil || minted.Token == "" {
		return "", fmt.Errorf("the index minted no credential for the run's identity — is this workflow its trusted publisher? %v", err)
	}
	return minted.Token, nil
}

// answered performs a request, fails on anything but success, and decodes a JSON answer into into.
func answered(client *http.Client, request *http.Request, into any) error {
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("%s answered %s: %s", request.URL.Redacted(), response.Status, strings.TrimSpace(string(body)))
	}
	if into != nil {
		return json.Unmarshal(body, into)
	}
	return nil
}

// wheelMetadata reads the fields of a wheel's metadata an upload states, in the form's names.
func wheelMetadata(path, version string) ([][2]string, error) {
	files, err := packages.WheelSources(path)
	if err != nil {
		return nil, err
	}
	metadata, ok := files["yoke_proto-"+version+".dist-info/METADATA"]
	if !ok {
		return nil, fmt.Errorf("%s carries no metadata for %s", filepath.Base(path), version)
	}
	names := map[string]string{"Metadata-Version": "metadata_version", "Name": "name", "Version": "version",
		"Summary": "summary", "License-Expression": "license_expression", "License-File": "license_file",
		"Requires-Python": "requires_python", "Requires-Dist": "requires_dist", "Project-URL": "project_urls"}
	var fields [][2]string
	scanner := bufio.NewScanner(bytes.NewReader(metadata))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break // the headers end where the description begins
		}
		key, value, ok := strings.Cut(line, ": ")
		if form, known := names[key]; ok && known {
			fields = append(fields, [2]string{form, value})
		}
	}
	return fields, nil
}
