// Command yoke-verify checks this project's own test descriptions and the markers that join a case
// to the test performing it, and writes the record of a run.
//
// Its commands, and the forms they check, are described at
// https://docs.yoke-project.dev/contributing/descriptions-and-records/#yoke-verify.
package main

import (
	"os"

	"github.com/yoke-project/yoke/internal/verify"
)

func main() {
	os.Exit(verify.Run(os.Args[1:], os.Stdout, os.Stderr))
}
