// Package release is `yoke`'s release verb: it publishes what the tags on this commit name into this
// repository's own ecosystems, and emits one manifest line per publication.
//
// It runs where a release's tag is seen. It reads its own tree and nothing else — no sibling, and never
// the manifest, which the release command reads and appends to.
package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/mod/sumdb/dirhash"
	"golang.org/x/mod/zip"
)

// Config is what the verb runs with.
type Config struct {
	Root string // the checkout, at the commit being released
	// Proxy asks the module proxy to serve a module at a version, which is what publishes it, and
	// returns the digest the proxy serves.
	Proxy func(module, version string) (string, error)
	Today func() time.Time
	Out   io.Writer // the lines, and nothing else
	Err   io.Writer

	// What a programs tag hands over as files: each artifact built for every architecture, and the
	// source archive, named for Source, when Source is not empty. Upload hands them to the tag's
	// release, which Releases, followed by the tag, locates.
	Artifacts []Artifact
	Source    string
	Releases  string
	Upload    func(tag string, files []string) error
}

// Artifact is one archive of programs, built from the packages named.
type Artifact struct {
	Name     string
	Packages []string
}

// Line is one publication, in the manifest's fixed shape and order.
type Line struct {
	Line          string   `json:"line"` // "publication"
	Published     string   `json:"published"`
	Version       string   `json:"version"`
	Commit        string   `json:"commit"`
	Digests       []string `json:"digests"`
	Where         string   `json:"where"`
	Authenticated string   `json:"authenticated"` // the mechanism a reader verifies it by
	Licence       string   `json:"licence"`
	Notices       []string `json:"notices"`
	Day           string   `json:"day"`
}

// The licence `yoke`'s role assigns, which every artifact it publishes carries.
const licence = "Apache-2.0"

// A Go module is published by the proxy serving it, and authenticated by the checksum database.
const (
	goProxy = "https://proxy.golang.org"
	goSumDB = "https://sum.golang.org"
)

// A module a tag names: `vX.Y.Z` the programs, the root module; `proto/vX.Y.Z` the definitions.
type tagged struct {
	tag, subdir, version string
}

// Run performs the verb, and returns its exit status. Every publication is checked before any line is
// written, so a refusal leaves no line behind.
func Run(cfg Config) int {
	fail := func(format string, a ...any) int {
		fmt.Fprintf(cfg.Err, "release: "+format+"\n", a...)
		return 1
	}
	listed, err := gitIn(cfg.Root, "tag", "--points-at", "HEAD")
	if err != nil {
		return fail("the tags on this commit cannot be read: %v", err)
	}
	commit, err := gitIn(cfg.Root, "rev-parse", "HEAD")
	if err != nil {
		return fail("the commit cannot be read: %v", err)
	}
	var modules []tagged
	for _, tag := range strings.Fields(listed) {
		version, subdir := tag, ""
		if rest, isDefinitions := strings.CutPrefix(tag, "proto/"); isDefinitions {
			version, subdir = rest, "proto"
		}
		if semver.IsValid(version) && semver.Canonical(version) == version {
			modules = append(modules, tagged{tag: tag, subdir: subdir, version: version})
		}
	}
	if len(modules) == 0 {
		fmt.Fprintln(cfg.Err, "release: nothing is published from this commit — no release tag names it")
		return 0
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].subdir < modules[j].subdir })

	var lines []Line
	for _, m := range modules {
		path, err := modulePath(filepath.Join(cfg.Root, m.subdir, "go.mod"))
		if err != nil {
			return fail("the module %s names cannot be read: %v", m.tag, err)
		}
		digest, err := treeDigest(cfg.Root, module.Version{Path: path, Version: m.version}, m.tag, m.subdir)
		if err != nil {
			return fail("the digest of %s@%s cannot be computed: %v", path, m.version, err)
		}
		served, err := cfg.Proxy(path, m.version)
		if err != nil {
			return fail("the proxy did not serve %s@%s: %v", path, m.version, err)
		}
		if served != digest {
			return fail("the proxy serves %s@%s as %s, which differs from the tree's %s", path, m.version, served, digest)
		}
		lines = append(lines, Line{Line: "publication", Published: path, Version: m.version, Commit: commit,
			Digests: []string{digest}, Where: goProxy, Authenticated: goSumDB, Licence: licence,
			Notices: []string{}, Day: cfg.Today().UTC().Format(time.DateOnly)})
	}

	// A programs tag also hands over files: the artifacts and the source archive, uploaded to its release.
	for _, m := range modules {
		if m.subdir != "" || (len(cfg.Artifacts) == 0 && cfg.Source == "") {
			continue
		}
		handed, err := handOver(cfg, m.tag, strings.TrimPrefix(m.version, "v"))
		if err != nil {
			return fail("the files of %s could not be handed over: %v", m.tag, err)
		}
		for _, f := range handed {
			lines = append(lines, Line{Line: "publication", Published: f.published, Version: m.version, Commit: commit,
				Digests: []string{f.digest}, Where: cfg.Releases + m.tag, Authenticated: origin(cfg.Releases),
				Licence: licence, Notices: []string{}, Day: cfg.Today().UTC().Format(time.DateOnly)})
		}
	}
	out := json.NewEncoder(cfg.Out)
	out.SetEscapeHTML(false)
	for _, l := range lines {
		if err := out.Encode(l); err != nil {
			return fail("a line could not be written: %v", err)
		}
	}
	return 0
}

func gitIn(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func modulePath(goMod string) (string, error) {
	data, err := os.ReadFile(goMod)
	if err != nil {
		return "", err
	}
	if path := modfile.ModulePath(data); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("%s declares no module", goMod)
}

// treeDigest is the digest Go's checksum database records for a module: its zip, made from the tagged
// tree as the proxy makes it, hashed.
func treeDigest(root string, m module.Version, revision, subdir string) (string, error) {
	file, err := os.CreateTemp("", "yoke-release-*.zip")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if err := zip.CreateFromVCS(file, m, root, revision, subdir); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	return dirhash.HashZip(file.Name(), dirhash.Hash1)
}

// FromProxy asks the public module proxy for a module at a version, with a module cache of its own and
// nothing exempted from the checksum database, and returns the digest it served.
func FromProxy(path, version string) (string, error) {
	cache, err := os.MkdirTemp("", "yoke-release-cache-")
	if err != nil {
		return "", err
	}
	defer func() {
		filepath.WalkDir(cache, func(p string, d os.DirEntry, _ error) error { os.Chmod(p, 0o700); return nil })
		os.RemoveAll(cache)
	}()
	command := exec.Command("go", "mod", "download", "-json", path+"@"+version)
	command.Dir = cache
	command.Env = append(environmentWithout("GOPRIVATE", "GONOPROXY", "GONOSUMDB", "GONOSUMCHECK", "GOINSECURE", "GOFLAGS"),
		"GOPROXY="+goProxy, "GOSUMDB=sum.golang.org", "GOMODCACHE="+cache, "GOWORK=off")
	out, err := command.Output()
	var answer struct{ Sum, Error string }
	if json.Unmarshal(out, &answer) != nil && err != nil {
		return "", err
	}
	if answer.Error != "" {
		return "", fmt.Errorf("%s", answer.Error)
	}
	return answer.Sum, nil
}

func environmentWithout(names ...string) []string {
	var env []string
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		keep := true
		for _, n := range names {
			keep = keep && name != n
		}
		if keep {
			env = append(env, variable)
		}
	}
	return env
}

// The architectures every artifact is built for.
var architectures = []string{"amd64", "arm64"}

// A file handed over: what it publishes, and the digest of its bytes.
type handed struct {
	published, path, digest string
}

// handOver builds every artifact for every architecture and the source archive, from the tagged tree,
// and uploads them to the tag's release. Every archive is written the same way from the same tree —
// its entries' times the tag's commit's, their owners nobody, the compression's header empty — so two
// builds give the same bytes.
func handOver(cfg Config, tag, version string) ([]handed, error) {
	dir, err := os.MkdirTemp("", "yoke-release-files-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	stamp, err := gitIn(cfg.Root, "log", "-1", "--format=%ct", tag)
	if err != nil {
		return nil, err
	}
	seconds, err := strconv.ParseInt(stamp, 10, 64)
	if err != nil {
		return nil, err
	}
	when := time.Unix(seconds, 0).UTC()
	notice, err := os.ReadFile(filepath.Join(cfg.Root, "LICENSE"))
	if err != nil {
		return nil, err
	}

	var files []handed
	for _, a := range cfg.Artifacts {
		for _, arch := range architectures {
			var entries []entry
			for _, pkg := range a.Packages {
				program := filepath.Join(dir, arch, filepath.Base(pkg))
				build := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w -buildid=", "-o", program, pkg)
				build.Dir = cfg.Root
				build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
				if said, err := build.CombinedOutput(); err != nil {
					return nil, fmt.Errorf("%s for %s does not build: %v\n%s", pkg, arch, err, said)
				}
				data, err := os.ReadFile(program)
				if err != nil {
					return nil, err
				}
				entries = append(entries, entry{name: filepath.Base(pkg), mode: 0o755, data: data})
			}
			entries = append(entries, entry{name: "LICENSE", mode: 0o644, data: notice})
			name := fmt.Sprintf("%s-%s-linux-%s.tar.gz", a.Name, version, arch)
			if err := writeArchive(filepath.Join(dir, name), entries, when); err != nil {
				return nil, err
			}
			files = append(files, handed{published: fmt.Sprintf("%s-linux-%s", a.Name, arch), path: filepath.Join(dir, name)})
		}
	}
	if cfg.Source != "" {
		// The archive of the tagged tree git makes, compressed as every other archive here is.
		archive := exec.Command("git", "-C", cfg.Root, "archive", "--format=tar", "--prefix="+cfg.Source+"-"+version+"/", tag)
		tarred, err := archive.Output()
		if err != nil {
			return nil, fmt.Errorf("the source archive cannot be made: %v", err)
		}
		name := fmt.Sprintf("%s-%s-source.tar.gz", cfg.Source, version)
		if err := writeCompressed(filepath.Join(dir, name), tarred); err != nil {
			return nil, err
		}
		files = append(files, handed{published: cfg.Source + "-source", path: filepath.Join(dir, name)})
	}

	var paths []string
	for i, f := range files {
		data, err := os.ReadFile(f.path)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		files[i].digest = "sha256:" + hex.EncodeToString(sum[:])
		paths = append(paths, f.path)
	}
	if err := cfg.Upload(tag, paths); err != nil {
		return nil, err
	}
	return files, nil
}

type entry struct {
	name string
	mode int64
	data []byte
}

// writeArchive writes the entries as a gzipped tar, every header fixed but the name, the mode and the size.
func writeArchive(path string, entries []entry, when time.Time) error {
	var tarred bytes.Buffer
	w := tar.NewWriter(&tarred)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: e.mode, Size: int64(len(e.data)), ModTime: when, Typeflag: tar.TypeReg, Format: tar.FormatPAX}
		if err := w.WriteHeader(h); err != nil {
			return err
		}
		if _, err := w.Write(e.data); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return writeCompressed(path, tarred.Bytes())
}

// writeCompressed gzips data with an empty header, so the bytes depend on the data alone.
func writeCompressed(path string, data []byte) error {
	var out bytes.Buffer
	z, err := gzip.NewWriterLevel(&out, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := z.Write(data); err != nil {
		return err
	}
	if err := z.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// origin is the scheme and host of where releases live: what authenticates a file served there.
func origin(releases string) string {
	u, err := url.Parse(releases)
	if err != nil || u.Host == "" {
		return releases
	}
	return u.Scheme + "://" + u.Host
}

// ToForge uploads the files to the tag's release on the forge, making the release when it does not yet
// exist; a file already there is replaced, which changes nothing, since a build gives the same bytes.
func ToForge(tag string, files []string) error {
	var repository []string
	if r := os.Getenv("GITHUB_REPOSITORY"); r != "" {
		repository = []string{"-R", r}
	}
	gh := func(args ...string) error {
		said, err := exec.Command("gh", append(args, repository...)...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("gh %s: %v\n%s", args[0], err, said)
		}
		return nil
	}
	if gh("release", "view", tag) != nil {
		return gh(append([]string{"release", "create", tag, "--verify-tag", "--title", tag, "--notes", ""}, files...)...)
	}
	return gh(append([]string{"release", "upload", tag, "--clobber"}, files...)...)
}
