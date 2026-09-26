// Package release is `yoke`'s release verb: it publishes what the tags on this commit name into this
// repository's own ecosystems, and emits one manifest line per publication.
package release

import (
	"io"
	"time"
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
}

// Run performs the verb, and returns its exit status.
func Run(cfg Config) int {
	return 0
}
