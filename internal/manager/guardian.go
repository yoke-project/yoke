package manager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/guardian"
	"github.com/yoke-project/yoke/internal/guardian/channel"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/instances"
)

// The verbs that write intent and then tell the account's guardian, which acts on the record and never on
// the message: a message lost costs latency and not correctness.
func init() {
	register("instance", "start", verb{"<name>", (*call).instanceStart})
	register("instance", "stop", verb{"<name>", (*call).instanceStop})
	register("instance", "restart", verb{"<name>", (*call).instanceRestart})
	register("instance", "enable", verb{"<name>", func(c *call) error { return c.instanceAutostart(true) }})
	register("instance", "disable", verb{"<name>", func(c *call) error { return c.instanceAutostart(false) }})
	register("instance", "clear", verb{"<name>", (*call).instanceClear})
	register("instance", "remove", verb{"<name>", (*call).instanceRemove})
}

// guardianProgram is the guardian the manager starts where nothing answers, found as the Core is by
// default: by its name on the PATH.
const guardianProgram = "yoke-guardian"

// guardian is the account's guardian, where one answers on its socket; nil where none does. Asking costs
// nothing and starts nothing.
func (c *call) guardian() (channel.GuardianClient, func()) {
	sock := filepath.Join(c.getenv("XDG_RUNTIME_DIR"), "yoke", "guardian.sock")
	if _, err := os.Stat(sock); err != nil {
		return nil, nil
	}
	conn, err := grpc.NewClient("unix://"+sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil
	}
	client := channel.NewGuardianClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Report(ctx, &channel.ReportRequest{}); err != nil {
		conn.Close()
		return nil, nil
	}
	return client, func() { conn.Close() }
}

// summon is the account's guardian, started where nothing answers: detached from this invocation, its
// output appended to `$XDG_STATE_HOME/yoke/guardian.log`. Two managers racing start one guardian, since
// the claim admits one.
func (c *call) summon() (channel.GuardianClient, func(), error) {
	if client, done := c.guardian(); client != nil {
		return client, done, nil
	}
	program, err := c.lookPath(guardianProgram)
	if err != nil {
		return nil, nil, err
	}
	logPath := filepath.Join(filepath.Dir(instancerecord.Root(c.getenv)), "guardian.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return nil, nil, err
	}
	log, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, nil, err
	}
	defer log.Close()
	cmd := exec.Command(program)
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// What the guardian needs to find its account, and what the display and audio needs of the instances
	// it starts are read from, which only the session that summoned it has.
	for _, key := range []string{"PATH", "HOME", "XDG_RUNTIME_DIR", "XDG_STATE_HOME", "XDG_CONFIG_HOME",
		"WAYLAND_DISPLAY", "DISPLAY", "XAUTHORITY", "PIPEWIRE_REMOTE", "PULSE_SERVER"} {
		if v := c.getenv(key); v != "" {
			cmd.Env = append(cmd.Env, key+"="+v)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("the guardian %s cannot be started: %w", program, err)
	}
	go cmd.Wait()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if client, done := c.guardian(); client != nil {
			return client, done, nil
		}
	}
	return nil, nil, fmt.Errorf("a guardian was started and never answered on its socket; its output is in %s", logPath)
}

// lookPath finds a program on the PATH this invocation was given.
func (c *call) lookPath(name string) (string, error) {
	for _, dir := range filepath.SplitList(c.getenv("PATH")) {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, nil
		}
	}
	return "", refusal{fmt.Errorf("%s is not on the PATH", name)}
}

// tell asks the guardian to reconcile one record, starting a guardian where nothing answers.
func (c *call) tell(name string) error {
	client, done, err := c.summon()
	if err != nil {
		return err
	}
	defer done()
	_, err = client.Reconcile(context.Background(), &channel.ReconcileRequest{Name: name})
	return err
}

func (c *call) named() (instances.Options, string, error) {
	if _, _, _, err := c.flags(nil); err != nil {
		return instances.Options{}, "", err
	}
	if err := c.positional(1, "an instance's name"); err != nil {
		return instances.Options{}, "", err
	}
	o, err := c.instances()
	if err != nil {
		return o, "", err
	}
	if _, err := instancerecord.Read(o.Instances, c.args[0]); err != nil {
		return o, "", refused(err)
	}
	return o, c.args[0], nil
}

// gateStart runs the gate the guardian will run before it executes a Core, and says each refusal to
// whoever typed the command: a start it refuses is said here, rather than only where the guardian's
// output goes. Refused is whether it refused.
func (c *call) gateStart(o instances.Options, r instancerecord.Record) (refused bool, err error) {
	cfg, err := LoadConfig(c.getenv)
	if err != nil {
		return false, refusal{err}
	}
	gated := guardian.GateStart(guardian.Options{Runtime: o.Runtime, Instances: o.Instances, Config: cfg}, r, true)
	if !gated.Refused() {
		return false, nil
	}
	for _, f := range gated.Findings {
		if f.Class == gate.Refusal {
			fmt.Fprintf(c.out, "  refused: %s at %s: %s\n", f.Code, f.Location, f.Message)
		}
	}
	return true, nil
}

// startRefused is a start the gate refuses, said, with the desired state left as it was asked. A guardian
// already running is still told, so its account of the instance carries the reason; none is started for
// a start it would refuse.
func (c *call) startRefused(o instances.Options, name string) error {
	r, err := instancerecord.Read(o.Instances, name)
	if err != nil {
		return refused(err)
	}
	if refused, err := c.gateStart(o, r); err != nil || !refused {
		return err
	}
	if client, done := c.guardian(); client != nil {
		client.Reconcile(context.Background(), &channel.ReconcileRequest{Name: name})
		done()
	}
	return refusef("%s should be running, and its start is refused: nothing ran", name)
}

func (c *call) instanceStart() error {
	o, name, err := c.named()
	if err != nil {
		return err
	}
	if err := instances.SetDesired(o, name, instancerecord.Running); err != nil {
		return err
	}
	if err := c.startRefused(o, name); err != nil {
		return err
	}
	if err := c.tell(name); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s should be running, and the guardian is told\n", name)
	return nil
}

func (c *call) instanceStop() error {
	o, name, err := c.named()
	if err != nil {
		return err
	}
	if err := instances.SetDesired(o, name, instancerecord.Stopped); err != nil {
		return err
	}
	// Nothing to stop and nobody to tell is not a reason to start a guardian.
	if up, _ := instances.Running(o, name); up {
		err = c.tell(name)
	} else if client, done := c.guardian(); client != nil {
		_, err = client.Reconcile(context.Background(), &channel.ReconcileRequest{Name: name})
		done()
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s should be stopped, and the guardian is told\n", name)
	return nil
}

// restartWithin bounds how long a restart waits for the instance to have stopped before it starts it.
const restartWithin = 2 * time.Minute

func (c *call) instanceRestart() error {
	o, name, err := c.named()
	if err != nil {
		return err
	}
	if up, _ := instances.Running(o, name); up {
		if err := instances.SetDesired(o, name, instancerecord.Stopped); err != nil {
			return err
		}
		if err := c.tell(name); err != nil {
			return err
		}
		for deadline := time.Now().Add(restartWithin); ; time.Sleep(100 * time.Millisecond) {
			if up, _ := instances.Running(o, name); !up {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s did not stop within %v, and its desired state is left stopped", name, restartWithin)
			}
		}
	}
	if err := instances.SetDesired(o, name, instancerecord.Running); err != nil {
		return err
	}
	if err := c.startRefused(o, name); err != nil {
		return err
	}
	if err := c.tell(name); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "%s is restarted, and the guardian is told\n", name)
	return nil
}

func (c *call) instanceAutostart(on bool) error {
	o, name, err := c.named()
	if err != nil {
		return err
	}
	return instances.SetAutostart(o, name, on)
}

func (c *call) instanceClear() error {
	o, name, err := c.named()
	if err != nil {
		return err
	}
	if err := instances.Clear(o, name); err != nil {
		return refused(err)
	}
	fmt.Fprintf(c.out, "%s's state is cleared, and the instance stays declared\n", name)
	return nil
}

func (c *call) instanceRemove() error {
	o, name, err := c.named()
	if err != nil {
		return err
	}
	if err := instances.Remove(o, name); err != nil {
		return refused(err)
	}
	fmt.Fprintf(c.out, "%s is removed, its record and its state together\n", name)
	return nil
}

// derived is what is known of each instance now: the guardian's account where one answers, otherwise
// what the claims say, and that no guardian answered.
func (c *call) derived(o instances.Options, names []string) (map[string]*channel.Watched, bool) {
	out := map[string]*channel.Watched{}
	if client, done := c.guardian(); client != nil {
		defer done()
		if r, err := client.Report(context.Background(), &channel.ReportRequest{}); err == nil {
			for _, w := range r.GetInstances() {
				out[w.GetName()] = w
			}
			return out, true
		}
	}
	for _, name := range names {
		state := "stopped"
		if up, err := instances.Running(o, name); err == nil && up {
			state = "running"
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			state = "unknown"
		}
		out[name] = &channel.Watched{Name: name, State: state}
	}
	return out, false
}
