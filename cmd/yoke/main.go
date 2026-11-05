// Command yoke is the manager: it installs, validates, starts, stops and observes deployments, and
// addresses a running instance through its published administrative contract.
package main

import (
	"fmt"
	"os"

	"github.com/yoke-project/yoke/internal/manager"

	"github.com/yoke-project/yoke/internal/version"
)

func main() {
	// --version alone is answered before anything else, and nothing is started (prj_structure/95).
	if version.Asked(os.Args[1:]) {
		fmt.Println(version.Line("yoke", version.BuildStated()))
		return
	}
	os.Exit(manager.Run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
