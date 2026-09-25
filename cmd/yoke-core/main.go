// Command yoke-core is the Core: one process per instance, which supervises, routes and persists and
// never interprets a payload.
//
// In the service form it is executed with the composition in force and reads everything else from
// `core.yaml`. It validates its configuration, derives its paths, takes the claim on its instance, and
// holds it until it is told to stop.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/core/instance"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("yoke-core", flag.ContinueOnError)
	composition := flags.String("composition", "", "the composition document in force, overriding YOKE_COMPOSITION")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	// Identity and paths, and the parameters: nothing is created before both pass.
	settings, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "yoke-core:", err)
		return 1
	}
	// The composition in force is chosen now, and read when the deployment is.
	_ = config.Composition(*composition, os.Getenv)
	paths := instance.ServicePaths(settings.RuntimeDir, settings.StateDir)

	// The claim: either this process serves the instance, or the refusal says which one does.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	claim, err := instance.Claim(paths.Root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "yoke-core:", err)
		return 1
	}
	<-stop
	if err := claim.Release(); err != nil {
		fmt.Fprintln(os.Stderr, "yoke-core:", err)
		return 1
	}
	return 0
}
