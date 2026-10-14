// Command yoke-auth asks the host's authentication service, once, whether a secret is an account's, for
// the Core that executes it: the account name, a zero byte and the secret on standard input; the answer
// is the exit status alone — 0 accepted, 1 refused, 2 the service could not be asked. `--probe` says
// whether the service can be asked at all.
package main

import (
	"os"

	"github.com/yoke-project/yoke/internal/pam"
)

func main() {
	os.Exit(pam.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}
