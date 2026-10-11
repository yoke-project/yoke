// Command yoke is the manager: it installs, validates, starts, stops and observes deployments, and
// addresses a running instance through its published administrative contract.
package main

import (
	"os"

	"github.com/yoke-project/yoke/internal/manager"
)

func main() {
	os.Exit(manager.Run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}
