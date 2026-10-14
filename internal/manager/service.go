package manager

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/yoke-project/yoke/internal/core/config"
	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/hostauth"
)

// The system regime: it reads the description the service form already has — core.yaml, the scanned
// Plugin directory and the composition in force — and asks the init system rather than starting
// anything itself.
func init() {
	register("service", "start", verb{"", func(c *call) error { return c.systemctl("start") }})
	register("service", "stop", verb{"", func(c *call) error { return c.systemctl("stop") }})
	register("service", "restart", verb{"", func(c *call) error { return c.systemctl("restart") }})
	register("service", "status", verb{"", (*call).serviceStatus})
	register("service", "check", verb{"[--composition <path>]", (*call).serviceCheck})
	register("service", "set-secret", verb{"<secret>", (*call).serviceSetSecret})
	register("service", "exec", verb{"[--composition <path>]", (*call).serviceExec})
}

// Unit is the service form's init unit: one deployment per host, named as its instance is.
const Unit = instance.ServiceName + ".service"

// stdin is where set-secret reads a value.
var stdin io.Reader = os.Stdin

// initSystem runs systemctl with the arguments given, answering what it printed and how it ended.
func (c *call) initSystem(args ...string) (string, int, error) {
	program, err := c.lookPath("systemctl")
	if err != nil {
		return "", 0, err
	}
	cmd := exec.Command(program, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return strings.TrimSpace(out.String()), exit.ExitCode(), nil
	}
	return strings.TrimSpace(out.String()), 0, err
}

// systemctl asks the init system to act on the unit, and starts nothing itself.
func (c *call) systemctl(act string) error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(0, "no subject"); err != nil {
		return err
	}
	said, code, err := c.initSystem(act, Unit)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the init system would not %s %s: %s", act, Unit, said)
	}
	fmt.Fprintf(c.out, "the init system is asked to %s %s\n", act, Unit)
	return nil
}

// serviceStatus reports what the init system says beside what the instance answers, and names a
// disagreement rather than choosing: the manager has no title to overrule the init system, and cannot
// claim an instance is fine when it has just failed to reach it.
func (c *call) serviceStatus() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(0, "no subject"); err != nil {
		return err
	}
	said, _, err := c.initSystem("is-active", Unit)
	if err != nil {
		return err
	}
	active := said == "active"
	fmt.Fprintf(c.out, "init system: %s is %s\n", Unit, said)

	answers, why := false, ""
	if o, err := c.dialService(); err != nil {
		why = err.Error()
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		records, err := o.Read(ctx, "instance", "")
		cancel()
		o.Close()
		switch {
		case err != nil:
			why = err.Error()
		case len(records) > 0:
			if records[0].GetInstance().GetReady() {
				answers = true
			} else {
				why = "it answers, and is not ready"
			}
		}
	}
	if answers {
		fmt.Fprintln(c.out, "instance: answers, and is ready")
	} else {
		fmt.Fprintf(c.out, "instance: does not answer: %s\n", why)
	}
	switch {
	case active && !answers:
		fmt.Fprintln(c.out, "disagreement: active according to the init system, not answering on its surface")
	case !active && answers:
		fmt.Fprintf(c.out, "disagreement: %s according to the init system, answering on its surface\n", said)
	}
	return nil
}

// composition is the composition in force — or the one named — passed through the gate as a start would
// pass it, reaching everything a start reaches.
func (c *call) composition(named string) (string, gate.Report, *gate.Deployment, error) {
	cfg, err := config.Load(c.getenv)
	if err != nil {
		return "", gate.Report{}, nil, refusal{err}
	}
	path := config.Composition(named, c.getenv)
	paths := instance.ServicePaths(cfg.RuntimeDir, cfg.StateDir)
	report, dep := gate.Check(gate.Input{
		Document:  gate.Read(path, gate.Composition),
		Moment:    gate.Starting,
		Manifests: cfg.Plugins.Manifests,
		Host: &gate.Host{Executables: cfg.Plugins.Executables, StateDir: paths.State, RuntimeRoot: paths.Root, Instance: paths.Name,
			// The Core this verb would execute is the one asked about.
			Authentication: func() error {
				core, err := c.lookPath("yoke-core")
				if err != nil {
					return err
				}
				return hostauth.Probe(core)
			},
			Engine: func() (gate.EngineFacts, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				e, err := engine.Reach(ctx, cfg.Engine)
				if err != nil {
					return gate.EngineFacts{}, err
				}
				return gate.EngineFacts{Rootless: e.Rootless}, nil
			}},
	})
	return path, report, dep, nil
}

// findings prints what the gate found, every failure rather than the first, and says whether it refused.
func (c *call) findings(path string, report gate.Report) bool {
	for _, f := range report.Findings {
		fmt.Fprintf(c.out, "%s %s at %s: %s\n", f.Class, f.Code, f.Location, f.Message)
	}
	return report.Refused()
}

func (c *call) serviceCheck() error {
	var named *string
	if _, _, _, err := c.flags(func(fs *flag.FlagSet) {
		named = fs.String("composition", "", "the composition to check, rather than the one in force")
	}); err != nil {
		return err
	}
	if err := c.positional(0, "no subject"); err != nil {
		return err
	}
	path, report, _, err := c.composition(*named)
	if err != nil {
		return err
	}
	if c.findings(path, report) {
		return refusef("the composition %s is refused", path)
	}
	fmt.Fprintf(c.out, "the composition %s passes\n", path)
	return nil
}

// serviceExec is the verb the unit names: it gates the composition and then executes the Core in its own
// place, so the party that executes the Core is the party that gated it, in one process.
func (c *call) serviceExec() error {
	var named *string
	if _, _, _, err := c.flags(func(fs *flag.FlagSet) {
		named = fs.String("composition", "", "the composition to execute, rather than the one in force")
	}); err != nil {
		return err
	}
	if err := c.positional(0, "no subject"); err != nil {
		return err
	}
	path, report, _, err := c.composition(*named)
	if err != nil {
		return err
	}
	if c.findings(path, report) {
		return refusef("the composition %s is refused, and no Core is executed", path)
	}
	core, err := c.lookPath("yoke-core")
	if err != nil {
		return err
	}
	return syscall.Exec(core, []string{"yoke-core", "--composition", path}, os.Environ())
}

// serviceSetSecret reads a value without echoing it and writes it into the deployment's own area,
// `<state_dir>/secrets/<secret>`, mode 0600 in a directory of mode 0700, and reports only its presence.
func (c *call) serviceSetSecret() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(1, "a secret's name"); err != nil {
		return err
	}
	name := c.args[0]
	if err := instance.ValidateName(name); err != nil {
		return refusef("the secret's name: %v", err)
	}
	cfg, err := config.Load(c.getenv)
	if err != nil {
		return refusal{err}
	}
	value, err := readSecret(stdin)
	if err != nil {
		return err
	}
	dir := filepath.Join(cfg.StateDir, "secrets")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+name+".")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "the secret %s is present\n", name)
	return nil
}

// readSecret reads one value: from a terminal with its echo off, and otherwise the whole input, a final
// line ending removed.
func readSecret(r io.Reader) (string, error) {
	if f, ok := r.(*os.File); ok {
		if t, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS); err == nil {
			quiet := *t
			quiet.Lflag &^= unix.ECHO
			if err := unix.IoctlSetTermios(int(f.Fd()), unix.TCSETS, &quiet); err == nil {
				defer unix.IoctlSetTermios(int(f.Fd()), unix.TCSETS, t)
			}
			fmt.Fprint(os.Stderr, "value: ")
			defer fmt.Fprintln(os.Stderr)
			line, err := bufio.NewReader(f).ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return "", err
			}
			r = strings.NewReader(line)
		}
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	value := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if value == "" {
		return "", refusef("no value was given")
	}
	return value, nil
}
