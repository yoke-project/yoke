// Command yoke-release is `yoke`'s release verb: run at a commit a release's tag names, it publishes
// what the tags name and prints one manifest line per publication.
package main

import (
	"os"
	"time"

	"github.com/yoke-project/yoke/internal/release"
)

func main() {
	os.Exit(release.Run(release.Config{Root: ".", Proxy: release.FromProxy, Today: time.Now, Out: os.Stdout, Err: os.Stderr}))
}
