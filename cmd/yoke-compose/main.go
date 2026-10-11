// Command yoke-compose is the composer: it makes a project, checks it with the one gate, and builds a
// package from it — signed where it is given a key, sealed where it is given a credential.
package main

import (
	"os"

	"github.com/yoke-project/yoke/internal/compose"
)

func main() {
	os.Exit(compose.Run(os.Args[1:], os.Stdout, os.Stderr))
}
