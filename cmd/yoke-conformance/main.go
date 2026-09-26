// Command yoke-conformance is the conformance suite: it drives a real Core and a harness of the library
// under test through every case of the contract that harness implements, prints the table and what ran,
// and exits zero only when every cell passes.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yoke-project/yoke/internal/conformance"
)

func main() {
	flags := flag.NewFlagSet("yoke-conformance", flag.ExitOnError)
	core := flags.String("core", "yoke-core", "the yoke-core binary to drive")
	harness := flags.String("harness", "", "the harness of the library under test")
	render := flags.String("render", "", "print a contract's cases as its description, and run nothing")
	flags.Parse(os.Args[1:])
	if *render != "" {
		contract, known := conformance.Contracts()[*render]
		if !known {
			fmt.Fprintf(os.Stderr, "yoke-conformance: no contract %s\n", *render)
			os.Exit(2)
		}
		os.Stdout.Write(conformance.Render(contract))
		return
	}
	if *harness == "" {
		fmt.Fprintln(os.Stderr, "yoke-conformance: --harness names the harness of the library under test")
		os.Exit(2)
	}
	os.Exit(conformance.Main(conformance.Config{Core: *core, Harness: *harness, Cases: conformance.Cases(), Out: os.Stdout}))
}
