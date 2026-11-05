// Command yoke-compose is the composer: it makes a project, checks it with the one gate, and builds a
// package from it — signed where it is given a key, sealed where it is given a credential.
package main

import (
	"fmt"
	"os"

	"github.com/yoke-project/yoke/internal/compose"

	"github.com/yoke-project/yoke/internal/version"
)

func main() {
	// --version alone is answered before anything else, and nothing is started (prj_structure/95).
	if version.Asked(os.Args[1:]) {
		fmt.Println(version.Line("yoke-compose", version.BuildStated()))
		return
	}
	os.Exit(compose.Run(os.Args[1:], os.Stdout, os.Stderr))
}
