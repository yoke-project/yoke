// Package verify reads the descriptions a repository carries, checks the form testing/30 fixes, and
// joins each case to the test that performs it.
package verify

import (
	"fmt"
	"io"
)

// Run performs one subcommand, writing what it produces to out and every finding to errOut. It
// returns the exit status: zero when every check the subcommand performs holds.
func Run(args []string, out, errOut io.Writer) int {
	fmt.Fprintln(errOut, "yoke-verify: not written yet")
	return 1
}
