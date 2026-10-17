package manager

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/yoke-project/yoke-sdk-go/admin"
	administrativev1 "github.com/yoke-project/yoke/proto/yoke/administrative/v1"

	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/instancerecord"
)

// The client verbs: one per operation of the administrative contract, on the operator projection, or on
// the connection a shell holds. Each reaches the instance the way any other client would; the manager
// holds no path of its own inwards.
func init() {
	register("core", "status", verb{"", func(c *call) error { return c.read("instance", 0) }})
	register("core", "read", verb{"<kind> [<identity>]", func(c *call) error { return c.readAny() }})
	register("core", "subscribe", verb{"[--kind K] [--identity I] [--floor N] [--type T]", (*call).subscribe})
	register("plugin", "list", verb{"", func(c *call) error { return c.read("plugin", 0) }})
	register("plugin", "show", verb{"<plugin>", func(c *call) error { return c.read("plugin", 1) }})
	register("plugin", "enable", verb{"<plugin>", change(1, "one subject", "enabled", func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.EnablePlugin(ctx, a[0])
	})})
	register("plugin", "disable", verb{"<plugin>", change(1, "one subject", "disabled", func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.DisablePlugin(ctx, a[0])
	})})
	register("plugin", "grant", verb{"<plugin> <capability>", change(2, "two subjects", "granted", func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.Grant(ctx, a[0], a[1])
	})})
	register("plugin", "withdraw", verb{"<plugin> <capability>", change(2, "two subjects", "not granted", func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.Withdraw(ctx, a[0], a[1])
	})})
	register("unit", "list", verb{"", func(c *call) error { return c.read("unit", 0) }})
	register("unit", "show", verb{"<unit>", func(c *call) error { return c.read("unit", 1) }})
	register("unit", "start", verb{"<unit>", c1(func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.StartUnit(ctx, a[0])
	})})
	register("unit", "stop", verb{"<unit>", c1(func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.StopUnit(ctx, a[0])
	})})
	register("unit", "restart", verb{"<unit>", c1(func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.RestartUnit(ctx, a[0])
	})})
	register("unit", "ask", verb{"<unit> <type> [--input <file>|-]", (*call).ask})
	register("unit", "retention", verb{"<unit> [--age D] [--bytes S] [--entries N] | --clear", (*call).retention})
	register("stream", "start", verb{"<unit> <stream>", c2(func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.StartStream(ctx, a[0], a[1])
	})})
	register("stream", "stop", verb{"<unit> <stream>", c2(func(o admin.Operations, ctx context.Context, a []string) (admin.Changed, error) {
		return o.StopStream(ctx, a[0], a[1])
	})})
	register("log", "query", verb{"[--unit U] [--incarnation N] [--floor N]", func(c *call) error { return c.log(false) }})
	register("log", "follow", verb{"[--unit U] [--incarnation N] [--floor N]", func(c *call) error { return c.log(true) }})
}

// operatorAddress is where the operator projection of the selected instance is.
func operatorAddress(instance string, selected bool, getenv func(string) string) (string, error) {
	a, err := addresses(instance, selected, getenv)
	return a.Operator, err
}

// addresses are the selected instance's two projections. With a selector they are derived from the
// instance's name, which chooses nothing else. With none, the account's one instance is selected;
// several are refused, since choosing among them is the selector's; and an account with none addresses
// the service form, under the runtime directory its `core.yaml` states.
func addresses(instance string, selected bool, getenv func(string) string) (admin.Addresses, error) {
	if !selected {
		listed, err := instancerecord.List(instancerecord.Root(getenv))
		if err != nil {
			return admin.Addresses{}, err
		}
		switch len(listed) {
		case 0:
		case 1:
			instance, selected = listed[0].Name, true
		default:
			var names []string
			for _, l := range listed {
				names = append(names, l.Name)
			}
			return admin.Addresses{}, refusef("this account has %d instances, %s: name one with --instance", len(names), strings.Join(names, ", "))
		}
	}
	if selected {
		a, err := admin.Application(instance, getenv)
		if err != nil {
			return admin.Addresses{}, refusal{err}
		}
		return a, nil
	}
	c, err := config.Load(getenv)
	if err != nil {
		return admin.Addresses{}, refusal{err}
	}
	return admin.Service(c.RuntimeDir), nil
}

// serviceAddress is the service form's operator projection, under the runtime directory its `core.yaml`
// states, whatever instances the account holds.
func serviceAddress(getenv func(string) string) (string, error) {
	c, err := config.Load(getenv)
	if err != nil {
		return "", refusal{err}
	}
	return admin.Service(c.RuntimeDir).Operator, nil
}

// flags parses the call's arguments: the selector and the verb's own options, the rest positional.
func (c *call) flags(define func(*flag.FlagSet)) (*flag.FlagSet, string, bool, error) {
	fs := flag.NewFlagSet("yoke", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	instance := fs.String("instance", "", "the instance a verb addresses")
	if define != nil {
		define(fs)
	}
	var positional, flags []string
	for i := 0; i < len(c.args); i++ {
		a := c.args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(c.args) && !isBool(fs, a) {
				flags = append(flags, c.args[i+1])
				i++
			}
			continue
		}
		positional = append(positional, a)
	}
	if err := fs.Parse(flags); err != nil {
		return nil, "", false, refusal{err}
	}
	c.args = positional
	selected := false
	fs.Visit(func(f *flag.Flag) { selected = selected || f.Name == "instance" })
	return fs, *instance, selected, nil
}

func isBool(fs *flag.FlagSet, arg string) bool {
	f := fs.Lookup(strings.TrimLeft(arg, "-"))
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// dial reaches the selected instance's operator projection. An instance whose socket is not there is not
// running, which is a fault: starting it changes the answer.
func (c *call) dial(instance string, selected bool) (*admin.Operator, error) {
	path, err := operatorAddress(instance, selected, c.getenv)
	if err != nil {
		return nil, err
	}
	return dialAt(path)
}

// dialService reaches the service form's operator projection.
func (c *call) dialService() (*admin.Operator, error) {
	path, err := serviceAddress(c.getenv)
	if err != nil {
		return nil, err
	}
	return dialAt(path)
}

// link is what a verb performs its operation on: in a shell, the connection it holds, which already names
// its instance; otherwise the selected instance's operator projection, let go when the verb is done.
func (c *call) link(instance string, selected bool) (admin.Operations, func(), error) {
	if c.held != nil {
		if selected {
			return admin.Operations{}, nil, refusef("--instance %s names an instance, and this shell's connection already names its own", instance)
		}
		return c.held.Operations, func() {}, nil
	}
	o, err := c.dial(instance, selected)
	if err != nil {
		return admin.Operations{}, nil, err
	}
	return o.Operations, func() { o.Close() }, nil
}

// context is what a verb's operation runs under: in a shell, the line's, which an interrupt ends.
func (c *call) context() context.Context {
	if c.line != nil {
		return c.line
	}
	return context.Background()
}

// interruptible is what a following verb runs under, ended by an interrupt: in a shell, the line's,
// which leaves the connection; otherwise the process's.
func (c *call) interruptible() (context.Context, func()) {
	if c.line != nil {
		return c.line, func() {}
	}
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func dialAt(path string) (*admin.Operator, error) {
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("the instance is not running: nothing answers at %s", path)
	}
	return admin.DialOperator(path)
}

func (c *call) positional(n int, usage string) error {
	if len(c.args) != n {
		if c.verb != "" {
			return refusef("%s: expected %s", strings.TrimSpace("yoke "+c.verb+" "+c.takes), usage)
		}
		return refusef("expected %s", usage)
	}
	return nil
}

// change is a verb whose operation changes something, printed as the contract answers it.
func change(n int, usage, asked string, op func(admin.Operations, context.Context, []string) (admin.Changed, error)) func(*call) error {
	return func(c *call) error {
		_, instance, selected, err := c.flags(nil)
		if err != nil {
			return err
		}
		if err := c.positional(n, usage); err != nil {
			return err
		}
		o, done, err := c.link(instance, selected)
		if err != nil {
			return err
		}
		defer done()
		ch, err := op(o, c.context(), c.args)
		if err != nil {
			return unreached(err)
		}
		printChanged(c.out, ch, asked)
		return nil
	}
}

func c1(op func(admin.Operations, context.Context, []string) (admin.Changed, error)) func(*call) error {
	return change(1, "one subject", "", op)
}

func c2(op func(admin.Operations, context.Context, []string) (admin.Changed, error)) func(*call) error {
	return change(2, "two subjects", "", op)
}

// unreached says that an instance did not answer where its transport could not reach it.
func unreached(err error) error {
	if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "Unavailable") {
		return fmt.Errorf("the instance is not running: %v", err)
	}
	return err
}

// printChanged prints what a change answered. Where the caller asked for a state and that is what the
// operation replaced, the effect was already true: it succeeded and changed nothing.
func printChanged(w io.Writer, ch admin.Changed, asked string) {
	if asked != "" && ch.Previously != nil && previously(ch.Previously) == asked {
		fmt.Fprintf(w, "nothing changed: it was already %s\n", asked)
		return
	}
	if ch.Previously != nil {
		fmt.Fprintf(w, "previously   %s\n", previously(ch.Previously))
	}
	switch ch.Effective {
	case admin.Immediately:
		fmt.Fprintln(w, "effective    immediately")
	case admin.AtNextAdmission:
		fmt.Fprintln(w, "effective    at the unit's next admission")
	}
	if len(ch.Consequences) == 0 {
		fmt.Fprintln(w, "consequences none")
	}
	for _, q := range ch.Consequences {
		fmt.Fprintf(w, "consequence  %s incarnation %d: %s\n", q.Unit, q.Incarnation, q.What)
	}
}

func previously(p *administrativev1.Previously) string {
	switch v := p.GetValue().(type) {
	case *administrativev1.Previously_Enabled:
		if v.Enabled {
			return "enabled"
		}
		return "disabled"
	case *administrativev1.Previously_Granted:
		if v.Granted {
			return "granted"
		}
		return "not granted"
	case *administrativev1.Previously_State:
		return v.State
	case *administrativev1.Previously_Retention:
		return protojson.Format(v.Retention)
	}
	return "nothing"
}

// read prints the records a read of a kind returns: every subject of it, or the one named.
func (c *call) read(kind string, n int) error {
	_, instance, selected, err := c.flags(nil)
	if err != nil {
		return err
	}
	if err := c.positional(n, map[int]string{0: "no subject", 1: "the subject's identity"}[n]); err != nil {
		return err
	}
	identity := ""
	if n == 1 {
		identity = c.args[0]
	}
	return c.records(instance, selected, kind, identity)
}

func (c *call) readAny() error {
	_, instance, selected, err := c.flags(nil)
	if err != nil {
		return err
	}
	if len(c.args) < 1 || len(c.args) > 2 {
		return refusef("expected a kind, and an identity where one is named")
	}
	identity := ""
	if len(c.args) == 2 {
		identity = c.args[1]
	}
	return c.records(instance, selected, c.args[0], identity)
}

func (c *call) records(instance string, selected bool, kind, identity string) error {
	o, done, err := c.link(instance, selected)
	if err != nil {
		return err
	}
	defer done()
	records, err := o.Read(c.context(), kind, identity)
	if err != nil {
		return unreached(err)
	}
	for _, r := range records {
		show(c.out, r)
	}
	return nil
}

var shown = protojson.MarshalOptions{Multiline: true, UseProtoNames: true}

func show(w io.Writer, m proto.Message) {
	b, err := shown.Marshal(m)
	if err != nil {
		fmt.Fprintln(w, err)
		return
	}
	fmt.Fprintf(w, "%s\n", b)
}

func (c *call) ask() error {
	var input *string
	_, instance, selected, err := c.flags(func(fs *flag.FlagSet) {
		input = fs.String("input", "", "a file holding the question, or - for standard input")
	})
	if err != nil {
		return err
	}
	if err := c.positional(2, "a unit and a question's type"); err != nil {
		return err
	}
	var question []byte
	switch *input {
	case "":
	case "-":
		if c.held != nil {
			return refusef("--input - reads the standard input, and in a shell that is where the lines come from")
		}
		question, err = io.ReadAll(os.Stdin)
	default:
		question, err = os.ReadFile(*input)
	}
	if err != nil {
		return err
	}
	o, done, err := c.link(instance, selected)
	if err != nil {
		return err
	}
	defer done()
	answer, err := o.Ask(c.context(), c.args[0], c.args[1], question)
	if err != nil {
		return unreached(err)
	}
	_, err = c.out.Write(answer)
	return err
}

func (c *call) retention() error {
	var age, size, entries *string
	var clear *bool
	_, instance, selected, err := c.flags(func(fs *flag.FlagSet) {
		age = fs.String("age", "", "the age limit")
		size = fs.String("bytes", "", "the size limit")
		entries = fs.String("entries", "", "the count limit")
		clear = fs.Bool("clear", false, "remove the override")
	})
	if err != nil {
		return err
	}
	if err := c.positional(1, "a unit"); err != nil {
		return err
	}
	var r admin.Retention
	if *age != "" {
		d, ok := gate.ParseDuration(*age)
		if !ok {
			return refusef("--age %q is not an integer followed by s, m, h or d", *age)
		}
		r.Age = d
	}
	if *size != "" {
		b, ok := gate.ParseSize(*size)
		if !ok {
			return refusef("--bytes %q is not an integer followed by KB, MB or GB", *size)
		}
		v := uint64(b)
		r.Bytes = &v
	}
	if *entries != "" {
		n, err := strconv.ParseUint(*entries, 10, 64)
		if err != nil {
			return refusef("--entries %q is not a count", *entries)
		}
		r.Entries = &n
	}
	o, done, err := c.link(instance, selected)
	if err != nil {
		return err
	}
	defer done()
	var ch admin.Changed
	if *clear {
		ch, err = o.ClearRetention(c.context(), c.args[0])
	} else {
		ch, err = o.SetRetention(c.context(), c.args[0], r)
	}
	if err != nil {
		return unreached(err)
	}
	printChanged(c.out, ch, "")
	return nil
}

func (c *call) logFlags() (string, bool, *string, *uint64, *uint, error) {
	var unit *string
	var incarnation *uint64
	var floor *uint
	_, instance, selected, err := c.flags(func(fs *flag.FlagSet) {
		unit = fs.String("unit", "", "one unit's entries")
		incarnation = fs.Uint64("incarnation", 0, "one incarnation's entries")
		floor = fs.Uint("floor", 0, "entries at or above this severity")
	})
	return instance, selected, unit, incarnation, floor, err
}

func (c *call) log(follow bool) error {
	instance, selected, unit, incarnation, floor, err := c.logFlags()
	if err != nil {
		return err
	}
	if err := c.positional(0, "no subject"); err != nil {
		return err
	}
	o, done, err := c.link(instance, selected)
	if err != nil {
		return err
	}
	defer done()
	ctx := c.context()
	if !follow {
		q := &administrativev1.LogQuery{Unit: *unit, Incarnation: *incarnation, Floor: uint32(*floor)}
		for {
			page, err := o.QueryLog(ctx, q)
			if err != nil {
				return unreached(err)
			}
			for _, e := range page.GetEntries() {
				line(c.out, e)
			}
			if page.GetNext() == 0 || len(page.GetEntries()) == 0 {
				return nil
			}
			q.Cursor = page.GetNext()
		}
	}
	ctx, stop := c.interruptible()
	defer stop()
	f, err := o.FollowLog(ctx, &administrativev1.LogFollow{Unit: *unit, Incarnation: *incarnation, Floor: uint32(*floor)})
	if err != nil {
		return unreached(err)
	}
	for {
		next, err := f.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return unreached(err)
		}
		show(c.out, next)
	}
}

func line(w io.Writer, e *administrativev1.LogEntry) {
	at := e.GetAt().AsTime().UTC().Format(time.RFC3339Nano)
	fmt.Fprintf(w, "%s %s/%d [%d] %s", at, e.GetUnit(), e.GetIncarnation(), e.GetSeverity(), strings.TrimRight(e.GetMessage(), "\n"))
	if d := e.GetDetail(); len(d) > 0 {
		fmt.Fprintf(w, " %s", d)
	}
	fmt.Fprintln(w)
}

func (c *call) subscribe() error {
	var kind, identity, typ *string
	var floor *uint
	_, instance, selected, err := c.flags(func(fs *flag.FlagSet) {
		kind = fs.String("kind", "", "a subject kind")
		identity = fs.String("identity", "", "a subject's identity")
		floor = fs.Uint("floor", 0, "events at or above this severity")
		typ = fs.String("type", "", "an event type, or a prefix of segments ending in a dot")
	})
	if err != nil {
		return err
	}
	o, done, err := c.link(instance, selected)
	if err != nil {
		return err
	}
	defer done()
	filter := &administrativev1.Filter{SubjectKind: *kind, SubjectIdentity: *identity, Floor: uint32(*floor),
		Type: strings.TrimSuffix(*typ, "."), TypePrefix: strings.HasSuffix(*typ, ".")}
	ctx, stop := c.interruptible()
	defer stop()
	s, err := o.Subscribe(ctx, filter)
	if err != nil {
		return unreached(err)
	}
	for {
		next, err := s.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return unreached(err)
		}
		show(c.out, next)
	}
}
