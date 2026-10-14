// Package hostauth is how the runtime asks the host's authentication service without opening its library:
// it executes the program beside the Core's own executable, once per question, and reads the exit status.
// The program is the only part of the runtime that opens a library; whatever imports this stays static.
package hostauth

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Program is the name of the program that asks, beside the Core's executable.
const Program = "yoke-auth"

// Wait is how long an answer is waited for before the program is killed and the question counted as one
// that could not be asked.
const Wait = 30 * time.Second

// Verdict is what the service answered.
type Verdict int

const (
	Accepted Verdict = iota // the secret is the account's
	Refused                 // it is not, or the service will not answer for the account
	Unasked                 // the service could not be asked
)

func (v Verdict) String() string {
	return [...]string{"accepted", "refused", "could not be asked"}[v]
}

// Beside is the program that asks for the Core at core.
func Beside(core string) string { return filepath.Join(filepath.Dir(core), Program) }

// Asker asks for the Core at Core, waiting at most Wait — the package's Wait where it is zero.
type Asker struct {
	Core string
	Wait time.Duration
}

// Ask asks whether secret is account's. An error says why the service could not be asked, and never
// carries the secret.
func (a Asker) Ask(ctx context.Context, account, secret string) (Verdict, error) {
	wait := a.Wait
	if wait == 0 {
		wait = Wait
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	cmd := exec.CommandContext(ctx, Beside(a.Core))
	cmd.Stdin = bytes.NewReader(append(append([]byte(account), 0), secret...))
	cmd.Env = os.Environ()
	var said bytes.Buffer
	cmd.Stderr = &said
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return Accepted, nil
	case ctx.Err() != nil:
		return Unasked, fmt.Errorf("%s did not answer within %v", Beside(a.Core), wait)
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return Refused, nil
	case errors.As(err, &exit):
		return Unasked, fmt.Errorf("%s could not ask the service: exit %d %s", Beside(a.Core), exit.ExitCode(), bytes.TrimSpace(said.Bytes()))
	}
	return Unasked, fmt.Errorf("%s cannot be executed: %w", Beside(a.Core), err)
}

// Probe says whether the service can be asked for the Core at core: an error names why not.
func Probe(core string) error {
	ctx, cancel := context.WithTimeout(context.Background(), Wait)
	defer cancel()
	out, err := exec.CommandContext(ctx, Beside(core), "--probe").CombinedOutput()
	if err != nil {
		return fmt.Errorf("the host's authentication service cannot be asked through %s: %v %s", Beside(core), err, bytes.TrimSpace(out))
	}
	return nil
}
