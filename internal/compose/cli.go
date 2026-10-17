package compose

import (
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/yoke-project/yoke/internal/gate"
)

const usage = `usage: yoke-compose <verb> [options]

  init <id> [--dir D] [--arch A] [--version V] [--core R]   make a project
  check [--dir D]                                           run the gate over it, then the advice
  build [--dir D] [--out O] [--sign KEY] [--seal FILE]      make a package from it

  add unit <name> --kind K [--exec P | --image R | --plugin ID] [--arg A]... [--depends-on U]... [--bind N=V]...
  add channel <name> --transport T --clients C [--unit U] [--class C [--host H] [--port N]] [--on-suspend G]
  add parameter <group>.<name> --type T [--range R] [--pattern P] [--values A,B] [--required] [--default V] [--label L]
  set <path> <value>                                        write any field, by the path a finding carries
  remove <path>                                             take away a unit, a channel, a parameter or any field, by its path
  arbitrate <channel> over <channel>...                     write one precedence relation
  block add <source> --as <name> [--dir D]                  place a block, recorded in blocks.yaml
  block save <unit> <destination> [--dir D]                 write a unit as a block
  waive <code> --at <location> --reason <text> [--dir D]    silence one advisory item, recorded in advisories.yaml
  waivers [--dir D]                                         every waiver, its reason, and whether it is stale
  migrate [--to M] [--dir D]                                carry the descriptor to model M, by default the newest

  An authoring verb writes the project's descriptor in --dir, or the composition document --file names,
  checked against the Manifests in --manifests.
`

// authoring are the verbs that write the descriptor or a composition document.
var authoring = []string{"add", "set", "remove", "arbitrate"}

// synopsis is a verb's own lines of the usage, and what an authoring verb writes; nothing for a verb
// the usage does not name.
func synopsis(verb string) string {
	var b strings.Builder
	for _, line := range strings.Split(usage, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), verb+" ") {
			b.WriteString(line + "\n")
		}
	}
	if b.Len() == 0 {
		return ""
	}
	said := "usage: yoke-compose\n" + b.String()
	if slices.Contains(authoring, verb) {
		said += "\n  It writes the project's descriptor in --dir, or the composition document --file names, checked\n" +
			"  against the Manifests in --manifests.\n"
	}
	return said
}

// asksHelp says whether a verb's arguments ask for its synopsis rather than for the verb.
func asksHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "-help" || a == "--help" {
			return true
		}
	}
	return false
}

// The exit statuses: a refusal will not pass on the next attempt, a fault may.
const (
	Done    = 0
	Refused = 1
	Fault   = 2
)

// Run is the command line: one verb per act. It returns the exit status: Done, Refused or Fault.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return Refused
	}
	verb, rest := args[0], args[1:]
	if asksHelp(rest) {
		if said := synopsis(verb); said != "" {
			fmt.Fprint(stdout, said)
			return Done
		}
	}
	fs := flag.NewFlagSet("yoke-compose "+verb, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "the project's directory")
	var err error
	switch verb {
	case "init":
		arch := fs.String("arch", "", "the architecture, by default this host's")
		ver := fs.String("version", "", "the version, by default 0.1.0")
		core := fs.String("core", "", "the range of Core versions, by default >=0.5")
		id, flags := split(rest)
		if fs.Parse(flags) != nil {
			return Refused
		}
		if id == "" {
			fmt.Fprint(stderr, "init takes the application's identifier\n"+usage)
			return Refused
		}
		if err = Init(*dir, id, InitOptions{Arch: *arch, Version: *ver, Core: *core}); err == nil {
			fmt.Fprintf(stdout, "%s made: %s, with %s/ and %s/ beside it\n", id, filepath.Join(*dir, Descriptor), Manifests, Components)
		}
	case "check":
		if fs.Parse(rest) != nil {
			return Refused
		}
		r, advised := Advise(*dir)
		show(stdout, r)
		shown, waived, werr := Unwaived(*dir, advised)
		if werr != nil {
			err = werr
			break
		}
		show(stdout, gate.Report{Findings: shown})
		if waived > 0 {
			noun := "advisory items"
			if waived == 1 {
				noun = "advisory item"
			}
			fmt.Fprintf(stdout, "waived: %d %s, listed by waivers\n", waived, noun)
		}
		if r.Refused() {
			return Refused
		}
		return Done
	case "waive":
		at := fs.String("at", "", "the location the finding points at")
		reason := fs.String("reason", "", "why the author keeps the combination")
		code, flags := split(rest)
		if fs.Parse(flags) != nil {
			return Refused
		}
		if code == "" || *at == "" {
			fmt.Fprint(stderr, "waive takes a code, --at and --reason\n"+usage)
			return Refused
		}
		if err = Waive(*dir, code, *at, *reason, time.Now()); err == nil {
			fmt.Fprintf(stdout, "waived %s at %s, recorded in %s\n", code, *at, Advisories)
		}
	case "waivers":
		if fs.Parse(rest) != nil {
			return Refused
		}
		var listed []Waiver
		if listed, err = Waivers(*dir); err == nil {
			for _, w := range listed {
				mark := ""
				if w.Stale {
					mark = "  stale: it matches nothing advised now"
				}
				fmt.Fprintf(stdout, "%s at %s, waived %s: %s%s\n", w.Code, w.At, w.WaivedAt, w.Reason, mark)
			}
		}
	case "migrate":
		to := fs.Int("to", ToNewest, "the model to carry the descriptor to, by default the newest this composer implements")
		if fs.Parse(rest) != nil {
			return Refused
		}
		var m Migration
		m, err = Migrate(*dir, *to)
		m.report(stdout)
		if err == nil {
			if m.From == m.To {
				fmt.Fprintf(stdout, "the project is already at model %d; nothing changed\n", m.To)
			} else {
				fmt.Fprintf(stdout, "carried the project from model %d to %d\n", m.From, m.To)
			}
		}
	case "build":
		out := fs.String("out", ".", "where the package is written")
		key := fs.String("sign", "", "a private key in PEM to sign with; none builds an unsigned package")
		seal := fs.String("seal", "", "a file holding the seal credential; none leaves the payload in clear")
		if fs.Parse(rest) != nil {
			return Refused
		}
		o := BuildOptions{Out: *out}
		if *key != "" {
			if o.Sign, err = signer(*key); err != nil {
				break
			}
		}
		if *seal != "" {
			if o.Seal, err = credential(*seal); err != nil {
				break
			}
		}
		var path string
		var r gate.Report
		path, r, err = Build(*dir, o)
		show(stdout, r)
		if err == nil {
			fmt.Fprintln(stdout, path)
		}
	case "add", "set", "remove", "arbitrate":
		return author(verb, rest, stdout, stderr)
	case "block":
		as := fs.String("as", "", "the name the block's unit is placed as")
		positional, flags := positionals(rest, fs)
		if fs.Parse(flags) != nil {
			return Refused
		}
		switch {
		case len(positional) == 2 && positional[0] == "add" && *as != "":
			if err = AddBlock(*dir, positional[1], *as, time.Now()); err == nil {
				fmt.Fprintf(stdout, "placed %s as %s, recorded in %s\n", positional[1], *as, Blocks)
			}
		case len(positional) == 3 && positional[0] == "save" && *as == "":
			if err = SaveBlock(*dir, positional[1], positional[2]); err == nil {
				fmt.Fprintf(stdout, "wrote %s as a block in %s\n", positional[1], positional[2])
			}
		default:
			fmt.Fprint(stderr, "yoke-compose block takes add <source> --as <name>, or save <unit> <destination>\n"+usage)
			return Refused
		}
	default:
		fmt.Fprintf(stderr, "yoke-compose has no verb %q; the verbs are init, check, build, add, set, remove, arbitrate, block, waive, waivers and migrate\n", verb)
		return Refused
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		if IsRefusal(err) {
			return Refused
		}
		return Fault
	}
	return Done
}

// split takes the one positional argument out of args, wherever it stands among the flags.
func split(args []string) (string, []string) {
	var positional string
	var flags []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case strings.HasPrefix(a, "-"):
			flags = append(flags, a)
			if !strings.Contains(a, "=") && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
		case positional == "":
			positional = a
		default:
			flags = append(flags, a)
		}
	}
	return positional, flags
}

func show(w io.Writer, r gate.Report) {
	for _, f := range r.Findings {
		fmt.Fprintf(w, "%-8s %s  %s  %s\n", f.Class, f.Code, f.Location, f.Message)
	}
	if len(r.NotRun) > 0 {
		fmt.Fprintf(w, "not run: %s\n", strings.Join(r.NotRun, ", "))
	}
}

func signer(path string) (crypto.Signer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, refusef("%s holds no PEM block", path)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, refusef("%s: %w", path, err)
	}
	s, ok := key.(crypto.Signer)
	if !ok {
		return nil, refusef("%s holds a key that cannot sign", path)
	}
	return s, nil
}

func credential(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return nil, refusef("%s is not one line of base64: %w", path, err)
	}
	if len(key) != 32 {
		return nil, refusef("the seal credential in %s is %d bytes; it must be 32", path, len(key))
	}
	return key, nil
}

// author runs an authoring verb: it writes one declaration, or one field, and says what it wrote.
func author(verb string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("yoke-compose "+verb, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", ".", "the project's directory")
	file := fs.String("file", "", "a composition document to write, in place of the project's descriptor")
	manifests := fs.String("manifests", "", "the Manifests a composition document is checked against")
	var u UnitOptions
	var c ChannelOptions
	var p ParameterOptions
	var values string
	fs.StringVar(&u.Kind, "kind", "", "the unit's kind")
	fs.StringVar(&u.Exec, "exec", "", "the unit's program")
	fs.StringVar(&u.Image, "image", "", "the unit's image, pinned by digest")
	fs.StringVar(&u.Plugin, "plugin", "", "the Plugin a unit of kind plugin runs")
	fs.Func("arg", "an argument, in order; repeat it", func(s string) error { u.Args = append(u.Args, s); return nil })
	fs.Func("depends-on", "a unit this one depends on; repeat it", func(s string) error { u.DependsOn = append(u.DependsOn, s); return nil })
	fs.Func("bind", "a binding its needs take, need=value; repeat it", func(s string) error {
		need, value, ok := strings.Cut(s, "=")
		if !ok || need == "" || value == "" {
			return fmt.Errorf("--bind %q is not need=value", s)
		}
		u.Bind = append(u.Bind, [2]string{need, value})
		return nil
	})
	fs.StringVar(&c.Transport, "transport", "", "the channel's transport")
	fs.StringVar(&c.Clients, "clients", "", "single or multiple")
	fs.StringVar(&c.Unit, "unit", "", "the managed interface the channel serves")
	fs.StringVar(&c.OnSuspend, "on-suspend", "", "the grade a suspension leaves")
	fs.StringVar(&c.Class, "class", "", "the address's class: local, loopback or routable")
	fs.StringVar(&c.Host, "host", "", "the address to bind")
	fs.StringVar(&c.Port, "port", "", "the port")
	fs.StringVar(&p.Type, "type", "", "the parameter's type")
	fs.StringVar(&p.Range, "range", "", "its range, [min, max]")
	fs.StringVar(&p.Pattern, "pattern", "", "its pattern")
	fs.StringVar(&values, "values", "", "its permitted values, separated by commas")
	fs.BoolVar(&p.Required, "required", false, "whether it is required")
	fs.StringVar(&p.Default, "default", "", "its default")
	fs.StringVar(&p.Label, "label", "", "its label")
	positional, flags := positionals(args, fs)
	if fs.Parse(flags) != nil {
		return Refused
	}
	if values != "" {
		p.Values = strings.Split(values, ",")
	}
	d := ProjectDocument(*dir)
	if *file != "" {
		d = CompositionDocument(*file, *manifests)
	}
	var err error
	var wrote string
	switch {
	case verb == "add" && len(positional) == 2 && positional[0] == "unit":
		err, wrote = AddUnit(d, positional[1], u), "units."+positional[1]
	case verb == "add" && len(positional) == 2 && positional[0] == "channel":
		err, wrote = AddChannel(d, positional[1], c), "channels."+positional[1]
	case verb == "add" && len(positional) == 2 && positional[0] == "parameter":
		err, wrote = AddParameter(d, positional[1], p), "parameters."+positional[1]
	case verb == "set" && len(positional) == 2:
		err, wrote = Set(d, positional[0], positional[1]), positional[0]
	case verb == "remove" && len(positional) == 1:
		if err = Remove(d, positional[0]); err == nil {
			fmt.Fprintf(stdout, "removed %s from %s\n", positional[0], d.Path)
			return Done
		}
	case verb == "arbitrate" && len(positional) >= 3 && positional[1] == "over":
		err, wrote = Arbitrate(d, positional[0], positional[2:]), "arbitration"
	default:
		fmt.Fprint(stderr, "yoke-compose "+verb+" takes other arguments\n"+usage)
		return Refused
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		if IsRefusal(err) {
			return Refused
		}
		return Fault
	}
	fmt.Fprintf(stdout, "wrote %s in %s\n", wrote, d.Path)
	return Done
}

// positionals takes the positional arguments out of args, wherever they stand among the flags; a flag
// that is not boolean takes the argument after it unless it is written --flag=value.
func positionals(args []string, fs *flag.FlagSet) ([]string, []string) {
	var positional, flags []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(name, "=") {
			continue
		}
		if f := fs.Lookup(name); f != nil {
			if b, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && b.IsBoolFlag() {
				continue
			}
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return positional, flags
}
