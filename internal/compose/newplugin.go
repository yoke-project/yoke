package compose

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/version"
)

// manifestURL is the release manifest on yoke's default branch, where a cut line names the family
// releases current with a programs version; composerVersion is the version this composer states.
var (
	manifestURL     = "https://raw.githubusercontent.com/yoke-project/yoke/main/releases/manifest.jsonl"
	composerVersion = version.BuildStated
	fetchClient     = &http.Client{Timeout: 2 * time.Minute}
)

// The two values a template is written with, and the directory its archive holds the tree in.
const (
	templateID      = "@YOKE_ID@"
	templateProgram = "@YOKE_PROGRAM@"
	templateRoot    = "template/"
)

// NewOptions are what `new plugin` is asked for besides the identity.
type NewOptions struct {
	// Language is the family whose template the tree starts from.
	Language string
	// Dir is the destination, which must not exist; empty is the program's name.
	Dir string
	// Program is the program's name; empty is the identity's last segment.
	Program string
	// Release names a published release whose cut to read, for a composer that is none; Template is an
	// archive already obtained, read with no network. At most one is given.
	Release, Template string
	// Out receives what is said and the take's output.
	Out io.Writer
}

// NewPlugin writes a Plugin project from the family's template, as arch/20-composer/07 §The template
// states, and returns its destination. Any refusal before the take leaves nothing; a failed take leaves
// the tree and says the command that repeats it.
func NewPlugin(id string, o NewOptions) (string, error) {
	out := o.Out
	if out == nil {
		out = io.Discard
	}
	// 1. What was given.
	if !gate.IsIdentifier(id) {
		return "", refusef("the id %q is not a namespaced identifier of dot-separated lowercase labels", id)
	}
	switch {
	case o.Language == "":
		return "", refusef("new plugin takes --language, one of %s", strings.Join(withTemplate(), ", "))
	case o.Language == "python":
		return "", refusef("a Python Plugin's template is carried later, with its image; the families with one are %s", strings.Join(withTemplate(), ", "))
	case !slices.Contains(gate.Families, o.Language):
		return "", refusef("the language %q is not a family; the families with a template are %s", o.Language, strings.Join(withTemplate(), ", "))
	}
	if o.Release != "" && o.Template != "" {
		return "", refusef("--release and --template are two ways to name the template: give one")
	}
	program := o.Program
	if program == "" {
		program = id[strings.LastIndex(id, ".")+1:]
	}
	if program == "." || program == ".." || strings.ContainsAny(program, `/\`) {
		return "", refusef("the program %q is not a file name: the build writes it under $YOKE_OUT, with no directory", program)
	}
	dest := o.Dir
	if dest == "" {
		dest = program
	}
	if _, err := os.Lstat(dest); err == nil {
		return "", refusef("%s is already there: a Plugin project is made in a directory that does not exist", dest)
	}

	// 2 and 3. The archive, resolved and checked.
	var body []byte
	core := ""
	if o.Template != "" {
		data, err := os.ReadFile(o.Template)
		if err != nil {
			return "", refusef("the template %s cannot be read: %v", o.Template, err)
		}
		body = data
		fmt.Fprintf(out, "the template %s is sha256:%x, checked against no record: it is the archive you obtained\n", o.Template, sha256.Sum256(body))
		if v := composerVersion(); version.IsRelease(v) {
			core = ">=" + strings.TrimPrefix(v, "v")
		}
	} else {
		release := o.Release
		if release == "" {
			v := composerVersion()
			if !version.IsRelease(v) {
				if v == "" {
					v = "no version"
				}
				return "", refusef("this composer is no release's build (it states %s), so no cut names its templates: "+
					"name a published release with --release <v>, or an archive you obtained with --template <archive>", v)
			}
			release = v
		}
		release = "v" + strings.TrimPrefix(release, "v")
		data, err := fetchTemplate(release, o.Language, out)
		if err != nil {
			return "", err
		}
		body = data
		core = ">=" + strings.TrimPrefix(release, "v")
	}

	// 4. Unpacked, substituted. From here a refusal removes the destination.
	if err := unpack(body, dest, id, program); err != nil {
		os.RemoveAll(dest)
		return "", err
	}
	// 5. The Plugin's file.
	if report, p := CheckPlugin(dest); p == nil {
		os.RemoveAll(dest)
		return "", refusef("%s", refusalLines(report))
	}
	// 6. try/, made an application project here.
	try := filepath.Join(dest, "try")
	if exists(filepath.Join(try, Descriptor)) {
		os.RemoveAll(dest)
		return "", refusef("the template's try/%s is there: try/ is made an application project by new plugin, on this machine", Descriptor)
	}
	if err := Init(try, id+"-try", InitOptions{Core: core}); err != nil {
		os.RemoveAll(dest)
		return "", err
	}
	// 7. The Plugin, taken into try/ from .., by the one who asked for it.
	abs, err := filepath.Abs(dest)
	if err != nil {
		return dest, err
	}
	t := &taking{source: ".."}
	t.commit, t.clean, t.tracked = revision(abs)
	if _, err := take(try, abs, t, TakeOptions{Yes: true, Out: out}); err != nil {
		return dest, refusef("%s holds the Plugin project, and taking it into try/ failed: %v\n"+
			"take it again with: cd %s && yoke-compose add plugin ..", dest, err, try)
	}
	return dest, nil
}

// withTemplate are the families that hand over a template.
func withTemplate() []string {
	return slices.DeleteFunc(slices.Clone(gate.Families), func(f string) bool { return f == "python" })
}

// manifestLine is the part of a release manifest's line new plugin reads.
type manifestLine struct {
	Line         string   `json:"line"`
	Published    string   `json:"published"`
	Version      string   `json:"version"`
	Digests      []string `json:"digests"`
	Where        string   `json:"where"`
	Publications []struct {
		Published string `json:"published"`
		Version   string `json:"version"`
	} `json:"publications"`
}

// fetchTemplate reads the manifest's cut line for the release, the family's template it names and that
// template's line, obtains the archive from the release the line names and checks its digest.
func fetchTemplate(release, family string, out io.Writer) ([]byte, error) {
	manifest, err := fetch(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("the release manifest cannot be read: %v", err)
	}
	var lines []manifestLine
	sc := bufio.NewScanner(bytes.NewReader(manifest))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		if text := strings.TrimSpace(sc.Text()); text != "" {
			var l manifestLine
			if json.Unmarshal([]byte(text), &l) == nil {
				lines = append(lines, l)
			}
		}
	}
	published := "yoke-plugin-template-" + family
	cut := slices.IndexFunc(lines, func(l manifestLine) bool { return l.Line == "cut" && l.Version == release })
	if cut < 0 {
		return nil, refusef("the release manifest holds no cut for %s: it is not a release whose templates can be found "+
			"(templates are published from 0.12.0); name another with --release, or an archive with --template", release)
	}
	at := slices.IndexFunc(lines[cut].Publications, func(p struct {
		Published string `json:"published"`
		Version   string `json:"version"`
	}) bool {
		return p.Published == published
	})
	if at < 0 {
		return nil, refusef("the cut of %s names no template for %s", release, family)
	}
	version := lines[cut].Publications[at].Version
	t := slices.IndexFunc(lines, func(l manifestLine) bool {
		return l.Line == "publication" && l.Published == published && l.Version == version
	})
	if t < 0 || len(lines[t].Digests) == 0 {
		return nil, refusef("the release manifest names %s at %s and holds no line stating its digest", published, version)
	}
	line := lines[t]
	if !strings.Contains(line.Where, "/releases/tag/") {
		return nil, refusef("the line of %s at %s names no release to obtain it from: %s", published, version, line.Where)
	}
	name := fmt.Sprintf("%s-%s.tar.gz", published, strings.TrimPrefix(version, "v"))
	url := strings.Replace(line.Where, "/releases/tag/", "/releases/download/", 1) + "/" + name
	fmt.Fprintf(out, "the template of %s for %s: %s\n", family, release, url)
	body, err := fetch(url)
	if err != nil {
		return nil, fmt.Errorf("the template cannot be obtained: %v", err)
	}
	if got := fmt.Sprintf("sha256:%x", sha256.Sum256(body)); got != line.Digests[0] {
		return nil, refusef("the template obtained has digest %s, and the release manifest states %s: it was not used", got, line.Digests[0])
	}
	return body, nil
}

func fetch(url string) ([]byte, error) {
	resp, err := fetchClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// unpack writes the archive's template/ into dest, the two values substituted in contents and names.
// Nothing else of the archive is read or changed.
func unpack(body []byte, dest, id, program string) error {
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return refusef("the template is not a gzip-compressed archive: %v", err)
	}
	substitute := strings.NewReplacer(templateID, id, templateProgram, program)
	tr := tar.NewReader(zr)
	found := false
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return refusef("the template is not a tar archive: %v", err)
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		name := h.Name
		if !strings.HasPrefix(name, templateRoot) {
			continue
		}
		rel := substitute.Replace(strings.TrimPrefix(name, templateRoot))
		if path.IsAbs(rel) || slices.Contains(strings.Split(rel, "/"), "..") {
			return refusef("the template's entry %s leaves the destination", name)
		}
		found = true
		target := filepath.Join(dest, filepath.FromSlash(rel))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			data, err := io.ReadAll(tr)
			if err != nil {
				return refusef("the template's entry %s cannot be read: %v", name, err)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			if err := os.WriteFile(target, []byte(substitute.Replace(string(data))), mode); err != nil {
				return err
			}
		default:
			return refusef("the template's entry %s is neither a file nor a directory", name)
		}
	}
	if !found {
		return refusef("the template holds no %s: a family's template is one directory, template/", templateRoot)
	}
	return nil
}
