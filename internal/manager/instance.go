package manager

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/instances"
	"github.com/yoke-project/yoke/internal/version"
)

// The lifecycle verbs on records: they act on the instance tree and reach no process.
func init() {
	register("instance", "create", verb{"<id> [--name <n>] [--version <v>] [--params <file>]", (*call).instanceCreate})
	register("instance", "list", verb{"", (*call).instanceList})
	register("instance", "show", verb{"<name>", (*call).instanceShow})
	register("instance", "set-version", verb{"<name> <version>", (*call).instanceSetVersion})
	register("instance", "set-param", verb{"<name> <group>.<key> <value>", (*call).instanceSetParam})
	register("instance", "copy", verb{"<name> <path>", (*call).instanceCopy})
	register("instance", "restore", verb{"<path> <name>", (*call).instanceRestore})
}

func (c *call) instances() (instances.Options, error) {
	cfg, err := LoadConfig(c.getenv)
	if err != nil {
		return instances.Options{}, refusal{err}
	}
	runtime := c.getenv("XDG_RUNTIME_DIR")
	o := instances.Options{InstallRoot: cfg.InstallRoot, Instances: instancerecord.Root(c.getenv),
		Runtime: filepath.Join(runtime, "yoke"), Arch: compose.HostArch(), RuntimeDir: &runtime}
	// The Core this host will execute, where it is there to be read; a range is checked against it.
	core := cfg.Core
	if !filepath.IsAbs(core) {
		core, _ = c.lookPath(core)
	}
	if v, err := version.OfBinary(core); core != "" && err == nil {
		o.Core = &v
	}
	return o, nil
}

// refused is a refusal where the answer will not change until something is: everything these verbs
// refuse is a declaration that does not hold.
func refused(err error) error {
	if err == nil {
		return nil
	}
	return refusal{err}
}

func (c *call) instanceCreate() error {
	var name, ver, params *string
	if _, _, _, err := c.flags(func(fs *flag.FlagSet) {
		name = fs.String("name", "", "the instance's name; one is generated where none is given")
		ver = fs.String("version", "", "the version to pin; the newest complete release where none is given")
		params = fs.String("params", "", "a values file, in TOML")
	}); err != nil {
		return err
	}
	if err := c.positional(1, "an application's identifier"); err != nil {
		return err
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	created, err := instances.Create(o, c.args[0], *name, *ver, *params)
	if err != nil {
		return refused(err)
	}
	r, err := instancerecord.Read(o.Instances, created)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "created %s, pinned to %s %s\n", created, r.ID, r.Version)
	return nil
}

func (c *call) instanceList() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(0, "no subject"); err != nil {
		return err
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	listed, err := instancerecord.List(o.Instances)
	if err != nil {
		return err
	}
	var names []string
	for _, l := range listed {
		names = append(names, l.Name)
	}
	derived := c.derived(o, names)
	for _, l := range listed {
		if l.Record == nil {
			fmt.Fprintf(c.out, "%s holds no valid record: %v\n", l.Name, l.Err)
			continue
		}
		r, w := l.Record, derived[l.Name]
		state := "stopped"
		if w != nil {
			state = w.GetState()
		}
		if state == "stopped" && r.Outcome == instancerecord.Failed {
			state = "failed"
		}
		line := fmt.Sprintf("%s %s %s %s", r.Name, r.ID, r.Version, state)
		if up := state != "stopped" && state != "refused" && state != "failed"; up != (r.DesiredState == instancerecord.Running) {
			line += ", desired " + r.DesiredState
		}
		if r.Autostart {
			line += " autostart"
		}
		if r.Ephemeral {
			line += " ephemeral"
		}
		fmt.Fprintln(c.out, line)
		// The why, which only the guardian knows.
		for _, f := range w.GetFindings() {
			fmt.Fprintf(c.out, "  refused: %s\n", f)
		}
		if w.GetState() == "waiting" {
			fmt.Fprintf(c.out, "  waiting: attempt %d, tried again in %v\n", w.GetAttempt(), w.GetWait().AsDuration())
		}
	}
	return nil
}

func (c *call) instanceShow() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(1, "an instance's name"); err != nil {
		return err
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	if _, err := instancerecord.Read(o.Instances, c.args[0]); err != nil {
		return refused(err)
	}
	data, err := os.ReadFile(instancerecord.File(o.Instances, c.args[0]))
	if err != nil {
		return err
	}
	fmt.Fprint(c.out, strings.TrimRight(string(data), "\n")+"\n")
	occupies, err := instances.Occupies(o, c.args[0])
	if err != nil {
		return err
	}
	fmt.Fprintln(c.out, "occupies:")
	for _, what := range slices.Sorted(maps.Keys(occupies)) {
		fmt.Fprintf(c.out, "  %s %d\n", what, occupies[what])
	}
	c.secretsOf(o, c.args[0])
	return nil
}

func (c *call) instanceCopy() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(2, "an instance's name and a path"); err != nil {
		return err
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	if err := instances.Copy(o, c.args[0], c.args[1]); err != nil {
		return refused(err)
	}
	fmt.Fprintf(c.out, "copied the state of %s to %s\n", c.args[0], c.args[1])
	return nil
}

func (c *call) instanceRestore() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(2, "a path and an instance's name"); err != nil {
		return err
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	if err := instances.Restore(o, c.args[0], c.args[1]); err != nil {
		return refused(err)
	}
	fmt.Fprintf(c.out, "restored %s from %s\n", c.args[1], c.args[0])
	return nil
}

func (c *call) instanceSetVersion() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(2, "an instance's name and a version"); err != nil {
		return err
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	if err := instances.SetVersion(o, c.args[0], c.args[1]); err != nil {
		return refused(err)
	}
	fmt.Fprintf(c.out, "%s is pinned to %s; it runs it from its next start\n", c.args[0], c.args[1])
	return nil
}

func (c *call) instanceSetParam() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(3, "an instance's name, a parameter and a value"); err != nil {
		return err
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	return refused(instances.SetParam(o, c.args[0], c.args[1], c.args[2]))
}
