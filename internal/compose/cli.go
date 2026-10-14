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
	"strings"

	"github.com/yoke-project/yoke/internal/gate"
)

const usage = `usage: yoke-compose <verb> [options]

  init <id> [--dir D] [--arch A] [--version V] [--core R]   make a project
  check [--dir D]                                           run the gate over it
  build [--dir D] [--out O] [--sign KEY] [--seal FILE]      make a package from it
`

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
		err = Init(*dir, id, InitOptions{Arch: *arch, Version: *ver, Core: *core})
	case "check":
		if fs.Parse(rest) != nil {
			return Refused
		}
		r := Check(*dir)
		show(stdout, r)
		if r.Refused() {
			return Refused
		}
		return Done
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
	default:
		fmt.Fprintf(stderr, "yoke-compose has no verb %q; the verbs are init, check and build\n", verb)
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
