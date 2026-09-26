// Command yoke-release is `yoke`'s release verb: run at a commit a release's tag names, it publishes
// what the tags name and prints one manifest line per publication.
package main

import (
	"os"
	"time"

	"github.com/yoke-project/yoke/internal/release"
)

// What a programs tag hands over as files: the developer axis's two artifacts, and the source archive.
// The host axis's tarball waits for the programs it holds.
var artifacts = []release.Artifact{
	{Name: "yoke-conformance", Packages: []string{"./cmd/yoke-conformance", "./cmd/yoke-core"}},
	{Name: "yoke-verify", Packages: []string{"./cmd/yoke-verify"}},
}

func main() {
	os.Exit(release.Run(release.Config{Root: ".", Proxy: release.FromProxy, Today: time.Now, Out: os.Stdout, Err: os.Stderr,
		Artifacts: artifacts, Source: "yoke", Releases: "https://github.com/yoke-project/yoke/releases/tag/", Upload: release.ToForge}))
}
