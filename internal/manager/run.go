package manager

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/instances"
)

// A run of an application with no record: the record it creates lasts as long as the run, and the
// guardian executes it like every other start. What the manager holds is the record and the following.
func init() {
	register("app", "run", verb{"<id> [--version <v>] | --from <dir> [--admit-unlaunched]", (*call).appRun})
}

func (c *call) appRun() error {
	var ver, from *string
	var unlaunched *bool
	if _, _, _, err := c.flags(func(fs *flag.FlagSet) {
		ver = fs.String("version", "", "the installed version to run; the newest complete release where none is given")
		from = fs.String("from", "", "a working directory holding a bundle being written, run unverified")
		unlaunched = fs.Bool("admit-unlaunched", false, "a process the Core did not launch may register")
	}); err != nil {
		return err
	}
	run := instances.Run{From: *from, Version: *ver, AdmitUnlaunched: *unlaunched}
	switch {
	case *from != "" && (len(c.args) != 0 || *ver != ""):
		return refusef("a run from a working directory names no application and no version: the descriptor there does")
	case *from == "":
		if err := c.positional(1, "an application's identifier, or --from and a working directory"); err != nil {
			return err
		}
		run.ID = c.args[0]
	}
	o, err := c.instances()
	if err != nil {
		return err
	}
	name, err := instances.Ephemeral(o, run)
	if err != nil {
		return refused(err)
	}
	r, err := instancerecord.Read(o.Instances, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s runs %s %s, an ephemeral instance; an interrupt stops it\n", name, r.ID, r.Version)
	if r.Verification == instancerecord.Waived {
		fmt.Fprintf(c.out, "  from %s, whose integrity is not verified\n", r.Source)
	}
	if r.Admission == instancerecord.UnlaunchedPermitted {
		fmt.Fprintln(c.out, "  a process the Core did not launch may register")
	}
	return c.follow(o, name)
}

// follow follows an ephemeral run until it ends: until whoever typed the command interrupts it, which
// writes the desired state and waits for the instance to come down, or until it fails, which leaves the
// record marked failed. The guardian is subscribed to before it is told, so no change is missed.
func (c *call) follow(o instances.Options, name string) error {
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupted)

	client, done, err := c.summon()
	if err != nil {
		return err
	}
	defer done()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub, err := client.Subscribe(ctx, &channel.SubscribeRequest{})
	if err != nil {
		return err
	}
	changes := make(chan *channel.Watched)
	ended := make(chan error, 1)
	go func() {
		for {
			w, err := sub.Recv()
			if err != nil {
				ended <- err
				return
			}
			if w.GetName() != name {
				continue
			}
			select {
			case changes <- w:
			case <-ctx.Done():
				return
			}
		}
	}()
	if _, err := client.Reconcile(ctx, &channel.ReconcileRequest{Name: name}); err != nil {
		return err
	}

	stopping, said := false, ""
	check := time.NewTicker(500 * time.Millisecond)
	defer check.Stop()
	for {
		select {
		case <-interrupted:
			if stopping {
				continue
			}
			stopping = true
			fmt.Fprintf(c.out, "%s is asked to stop\n", name)
			if err := instances.SetDesired(o, name, instancerecord.Stopped); err != nil {
				return err
			}
			if _, err := client.Reconcile(ctx, &channel.ReconcileRequest{Name: name}); err != nil {
				return err
			}
		case w := <-changes:
			if w.GetState() != said {
				said = w.GetState()
				fmt.Fprintf(c.out, "%s is %s\n", name, said)
				if said == "observable" {
					c.saysOfItself(name)
				}
			}
			switch w.GetState() {
			case "refused":
				for _, f := range w.GetFindings() {
					fmt.Fprintf(c.out, "  refused: %s\n", f)
				}
				return c.unrun(o, client, name, refusef("%s was refused at its start, and nothing ran", name))
			case "failed":
				return c.failed(name)
			}
		case <-check.C:
			if over, err := c.over(o, client, name, stopping); over || err != nil {
				return err
			}
		case err := <-ended:
			// The guardian has gone: what the record says is the end of the run.
			if over, err := c.over(o, client, name, true); over || err != nil {
				return err
			}
			return fmt.Errorf("the guardian ended while %s was running: %v", name, err)
		}
	}
}

// over says whether a run has ended, and how: its record gone is an orderly stop, marked failed is a
// failure. A run asked to stop that the guardian is not running — never started, or already down — is
// ended by the manager removing the record, which nothing else would.
func (c *call) over(o instances.Options, client channel.GuardianClient, name string, stopping bool) (bool, error) {
	r, err := instancerecord.Read(o.Instances, name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		fmt.Fprintf(c.out, "%s has stopped, and its record is gone with it\n", name)
		return true, nil
	case err != nil:
		return true, err
	case r.Outcome == instancerecord.Failed:
		return true, c.failed(name)
	case !stopping:
		return false, nil
	}
	if up, _ := instances.Running(o, name); up {
		return false, nil
	}
	report, err := client.Report(context.Background(), &channel.ReportRequest{})
	if err == nil {
		for _, w := range report.GetInstances() {
			if w.GetName() == name && w.GetState() != "stopped" && w.GetState() != "refused" {
				return false, nil
			}
		}
	}
	return true, c.unrun(o, client, name, nil)
}

// unrun removes the record of a run that never ran, and tells the guardian, which forgets it.
func (c *call) unrun(o instances.Options, client channel.GuardianClient, name string, why error) error {
	if err := instances.Remove(o, name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	client.Reconcile(context.Background(), &channel.ReconcileRequest{Name: name})
	if why == nil {
		fmt.Fprintf(c.out, "%s has stopped, and its record is gone with it\n", name)
	}
	return why
}

// failed is a run that did not hold: the record stays, marked failed, for whoever needs to look at it.
func (c *call) failed(name string) error {
	return fmt.Errorf("%s failed; its record and state are kept until read or reclaimed — yoke instance show %s, yoke app prune", name, name)
}

// saysOfItself prints the weaker arrangements a running instance reports about itself, through its
// operator projection like any other client: the development waivers are among them.
func (c *call) saysOfItself(name string) {
	o, err := c.dial(name, true)
	if err != nil {
		return
	}
	defer o.Close()
	records, err := o.Read(context.Background(), "instance", "")
	if err != nil {
		return
	}
	for _, r := range records {
		for _, w := range r.GetInstance().GetWeaker() {
			fmt.Fprintf(c.out, "  it says of itself: %s\n", w)
		}
	}
}
