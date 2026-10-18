// Command yoke-release is `yoke`'s release verb: run at a commit a release's tag names, it publishes
// what the tags name and prints one manifest line per publication.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yoke-project/yoke/internal/packages"
	"github.com/yoke-project/yoke/internal/release"
)

func main() {
	// A repository that publishes only its Go modules — a Go family — runs this verb with -modules-only.
	modulesOnly := flag.Bool("modules-only", false, "publish the modules the tags name, and hand over no file")
	// The maintainer publishes the definitions packages alone by hand once, for the first crate: crates.io
	// trusts a workflow only for a crate that exists.
	packagesOnly := flag.Bool("packages-only", false, "publish the definitions packages a definitions tag names, and nothing else")
	// A family that publishes a package — Rust's crate, Python's wheel — names it, and the verb publishes
	// that package alone, made and published by the family's ci/package.sh.
	crate := flag.String("crate", "", "publish the family's crate of this name at its release tag, and nothing else")
	wheel := flag.String("wheel", "", "publish the family's wheel of this name at its release tag, and nothing else")
	// A family consumed as its source — C, C++ — names the archive, and the verb hands that over alone.
	source := flag.String("source", "", "hand over the family's source archive of this name at its release tag, and nothing else")
	// The tag whose placing invoked the run: a commit carrying two release tags runs the verb once for
	// each, and each run publishes what its own tag names.
	invoked := flag.String("tag", "", "the tag that invoked this run; none publishes every release tag on the commit")
	// A programs release runs the suite against each family's newest release tag: -measure names them,
	// from the tags the families' repositories carry, and -conformance hands over the report joined from
	// what was measured.
	measure := flag.String("measure", "", "print, from this file of the tags each family's repository carries, the family releases a programs release measures, and nothing else")
	conformance := flag.String("conformance", "", "the directory the release's measuring left: the joined report is handed over with the programs")
	flag.Parse()
	if *measure != "" {
		data, err := os.ReadFile(*measure)
		if err == nil {
			var measured []release.Measure
			if measured, err = release.Measured(data); err == nil {
				if measured == nil {
					measured = []release.Measure{}
				}
				err = json.NewEncoder(os.Stdout).Encode(measured)
			}
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "release:", err)
			os.Exit(1)
		}
		return
	}
	cfg := release.Config{Root: ".", Invoked: *invoked, Proxy: release.FromProxy, Today: time.Now, Out: os.Stdout, Err: os.Stderr}
	switch {
	case *crate != "" && *wheel != "":
		fmt.Fprintln(os.Stderr, "release: a family publishes one package: -crate or -wheel, not both")
		os.Exit(2)
	case *source != "" && (*crate != "" || *wheel != ""):
		fmt.Fprintln(os.Stderr, "release: a family is published one way: -source, -crate or -wheel")
		os.Exit(2)
	case *source != "":
		cfg.SourceRelease, cfg.Source, cfg.Upload = true, *source, release.ToForge
		cfg.Releases = "https://github.com/" + os.Getenv("GITHUB_REPOSITORY") + "/releases/tag/"
	case *crate != "":
		cfg.Family, cfg.Settle, cfg.Pause = true, 30, 10*time.Second
		cfg.Packages = release.Scripted(".", packages.Rust, *crate)
		cfg.Registries = []release.Registry{release.Crates{API: "https://crates.io", Static: "https://static.crates.io", Root: ".",
			Name: *crate, Script: "ci/package.sh"}}
	case *wheel != "":
		cfg.Family, cfg.Settle, cfg.Pause = true, 30, 10*time.Second
		cfg.Packages = release.Scripted(".", packages.Python, *wheel)
		cfg.Registries = []release.Registry{release.PyPI{Index: "https://pypi.org", Upload: "https://upload.pypi.org/legacy/", Name: *wheel}}
	case !*modulesOnly:
		cfg.Artifacts, cfg.Source, cfg.Upload = release.Yoke, "yoke", release.ToForge
		cfg.Modules = true
		cfg.Conformance = *conformance
		cfg.Releases = "https://github.com/yoke-project/yoke/releases/tag/"
		cfg.Repository = "yoke-project/yoke"
		cfg.Forge = release.GitHub{Repository: cfg.Repository, Root: "."}
		cfg.Packages = func(version, dir string) (string, string, error) { return packages.Package(".", version, dir) }
		cfg.Registries = []release.Registry{
			release.Crates{API: "https://crates.io", Static: "https://static.crates.io", Root: "."},
			release.PyPI{Index: "https://pypi.org", Upload: "https://upload.pypi.org/legacy/"},
		}
		// A registry may take minutes to serve what it was just given.
		cfg.Settle, cfg.Pause = 30, 10*time.Second
		cfg.PackagesOnly = *packagesOnly
	}
	os.Exit(release.Run(cfg))
}
