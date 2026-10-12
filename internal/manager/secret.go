package manager

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"

	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/instances"
)

func init() {
	register("instance", "set-secret", verb{"<name> <secret>", (*call).instanceSetSecret})
}

// instanceSetSecret reads a value without echoing it and writes it into the instance's state, reporting
// only that it is present. A value is read at launch and held for the incarnation, so where the instance
// runs, each unit that needs the secret is restarted over the operator projection; where it does not,
// there is nothing to restart and the verb says so.
func (c *call) instanceSetSecret() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(2, "an instance's name and a secret's"); err != nil {
		return err
	}
	name, secret := c.args[0], c.args[1]
	if err := instance.ValidateName(secret); err != nil {
		return refusef("the secret's name: %v", err)
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	if _, err := instancerecord.Read(o.Instances, name); err != nil {
		return refused(err)
	}
	value, err := readSecret(stdin)
	if err != nil {
		return err
	}
	if err := instances.SetSecret(o, name, secret, value); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "the secret %s of %s is present\n", secret, name)
	if up, _ := instances.Running(o, name); !up {
		fmt.Fprintf(c.out, "%s is not running, so nothing is restarted\n", name)
		return nil
	}
	declared, err := instances.Secrets(o, name)
	if err != nil {
		return err
	}
	units := declared[secret]
	if len(units) == 0 {
		fmt.Fprintf(c.out, "no unit of %s needs %s, so nothing is restarted\n", name, secret)
		return nil
	}
	op, err := c.dial(name, true)
	if err != nil {
		return err
	}
	defer op.Close()
	for _, unit := range units {
		if _, err := op.RestartUnit(context.Background(), unit); err != nil {
			return fmt.Errorf("%s needs %s and could not be restarted: %w", unit, secret, unreached(err))
		}
		fmt.Fprintf(c.out, "restarted %s, which needs %s\n", unit, secret)
	}
	return nil
}

// secretsOf reports, for `yoke instance show`, each secret the version declares and whether the instance's
// state holds it — presence and absence, never a value.
func (c *call) secretsOf(o instances.Options, name string) {
	declared, err := instances.Secrets(o, name)
	if err != nil || len(declared) == 0 {
		return
	}
	fmt.Fprintln(c.out, "secrets:")
	for _, secret := range slices.Sorted(maps.Keys(declared)) {
		state := "missing"
		if _, err := os.Stat(instances.SecretPath(o, name, secret)); err == nil {
			state = "present"
		}
		fmt.Fprintf(c.out, "  %s %s, needed by %v\n", secret, state, declared[secret])
	}
}
