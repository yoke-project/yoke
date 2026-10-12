// Command yoke-guardian is the account's guardian, in the application form: it brings the declared
// instances to their desired state and keeps them there. An init unit starts it when a host comes up,
// with --boot; the manager starts it when nothing answers on its socket. Either way it is the same process
// reading the same records, and it exits when nothing is left to watch.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/yoke-project/yoke/internal/guardian"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/managerconf"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("yoke-guardian", flag.ContinueOnError)
	boot := flags.Bool("boot", false, "the pass a host coming up makes: autostart, and a desired state not left at stopped")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" || os.Getenv("XDG_STATE_HOME") == "" && os.Getenv("HOME") == "" {
		fmt.Fprintln(os.Stderr, "yoke-guardian: XDG_RUNTIME_DIR must be set, and XDG_STATE_HOME or HOME")
		return 2
	}
	cfg, err := managerconf.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "yoke-guardian:", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	err = guardian.Run(ctx, guardian.Options{Runtime: filepath.Join(runtime, "yoke"), Instances: instancerecord.Root(os.Getenv),
		Config: cfg, Boot: *boot, Stderr: os.Stderr})
	if err != nil {
		fmt.Fprintln(os.Stderr, "yoke-guardian:", err)
		return 1
	}
	return 0
}
