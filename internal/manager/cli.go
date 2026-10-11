package manager

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/yoke-project/yoke-sdk-go/base"
)

// The exit statuses: a refusal will not pass on the next attempt, a fault may.
const (
	Done    = 0
	Fault   = 1
	Refused = 2
)

// refusal is an answer that will not change until something is changed.
type refusal struct{ err error }

func (r refusal) Error() string { return r.err.Error() }
func (r refusal) Unwrap() error { return r.err }

func refusef(format string, args ...any) error { return refusal{fmt.Errorf(format, args...)} }

// verb is one act of the command line.
type verb struct {
	usage string
	run   func(c *call) error
}

// call is one invocation: the arguments after the verb, the environment and where the output goes.
type call struct {
	args   []string
	getenv func(string) string
	out    io.Writer
}

// verbs are the command line's acts, by subject and operation. Each lifecycle namespace adds its own.
var verbs = map[string]map[string]verb{}

func register(subject, operation string, v verb) {
	if verbs[subject] == nil {
		verbs[subject] = map[string]verb{}
	}
	verbs[subject][operation] = v
}

// Run is `yoke`: it performs one verb and returns its exit status.
func Run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, Usage())
		return Done
	}
	subject, ops := args[0], verbs[args[0]]
	if ops == nil {
		fmt.Fprintf(stderr, "yoke has no verb %q; the subjects are %s\n", subject, strings.Join(subjects(), ", "))
		return Refused
	}
	if len(args) < 2 || ops[args[1]].run == nil {
		named := ""
		if len(args) > 1 {
			named = fmt.Sprintf(" %q", args[1])
		}
		fmt.Fprintf(stderr, "yoke %s has no operation%s; it has %s\n", subject, named, strings.Join(operations(subject), ", "))
		return Refused
	}
	err := ops[args[1]].run(&call{args: args[2:], getenv: getenv, out: stdout})
	if err == nil {
		return Done
	}
	fmt.Fprintln(stderr, "yoke:", err)
	var r refusal
	var b *base.Refusal
	if errors.As(err, &r) || errors.As(err, &b) {
		return Refused
	}
	return Fault
}

// Usage is every verb, by subject.
func Usage() string {
	var b strings.Builder
	b.WriteString("usage: yoke <subject> <operation> [arguments] [--instance <name>]\n\n")
	for _, s := range subjects() {
		for _, o := range operations(s) {
			fmt.Fprintf(&b, "  yoke %s %s %s\n", s, o, verbs[s][o].usage)
		}
	}
	return b.String()
}

func subjects() []string {
	var out []string
	for s := range verbs {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func operations(subject string) []string {
	var out []string
	for o := range verbs[subject] {
		out = append(out, o)
	}
	sort.Strings(out)
	return out
}
