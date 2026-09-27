// Command yoke-release is `yoke`'s release verb: run at a commit a release's tag names, it publishes
// what the tags name and prints one manifest line per publication.
package main

import (
	"flag"
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
	flag.Parse()
	cfg := release.Config{Root: ".", Proxy: release.FromProxy, Today: time.Now, Out: os.Stdout, Err: os.Stderr}
	if !*modulesOnly {
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
