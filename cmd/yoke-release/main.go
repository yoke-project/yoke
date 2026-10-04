// Command yoke-release is `yoke`'s release verb: run at a commit a release's tag names, it publishes
// what the tags name and prints one manifest line per publication.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/yoke-project/yoke/internal/packages"
	"github.com/yoke-project/yoke/internal/release"
)

// What a programs tag hands over as files: the developer axis's two artifacts, and the source archive.
// The host axis's tarball waits for the programs it holds.
var artifacts = []release.Artifact{
	{Name: "yoke-conformance", Packages: []string{"./cmd/yoke-conformance", "./cmd/yoke-core"}},
	{Name: "yoke-verify", Packages: []string{"./cmd/yoke-verify"}},
}

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
	flag.Parse()
	cfg := release.Config{Root: ".", Proxy: release.FromProxy, Today: time.Now, Out: os.Stdout, Err: os.Stderr}
	switch {
	case *crate != "" && *wheel != "":
		fmt.Fprintln(os.Stderr, "release: a family publishes one package: -crate or -wheel, not both")
		os.Exit(2)
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
		cfg.Artifacts, cfg.Source, cfg.Upload = artifacts, "yoke", release.ToForge
		cfg.Releases = "https://github.com/yoke-project/yoke/releases/tag/"
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
