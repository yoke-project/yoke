// The greeter: a Plugin written against the Go family's library, as an author would write it, kept as a
// Plugin project that the scenario of taking a Plugin takes into an application.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yoke-project/yoke-sdk-go/plugin"
)

var greeter = plugin.Declaration{ID: "com.example.greeter"}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--manifest" {
		os.Stdout.Write(greeter.Manifest())
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	u, err := plugin.Start(ctx, greeter)
	if err != nil {
		fmt.Fprintln(os.Stderr, "greeter:", err)
		os.Exit(1)
	}
	// A unit's heartbeat repeats its author's last report, so it reports once to keep its Session.
	u.Health(0, "ready")
	<-ctx.Done()
	u.Close()
	<-u.Done()
}
