package supervisor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/event"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// engineWait bounds each act asked of the engine.
const engineWait = 30 * time.Second

// attachedWait bounds how long a container's end waits for the last of its output.
const attachedWait = 2 * time.Second

// container is a unit launched in a container, ended by asking the engine to signal it.
type container struct {
	c  Containers
	id string
}

func (r container) terminate() { r.signal("SIGTERM") }
func (r container) kill()      { r.signal("SIGKILL") }

func (r container) signal(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), engineWait)
	defer cancel()
	r.c.Signal(ctx, r.id, name)
}

// watched is a container launched and not yet concluded: whose it is, its output, and what its end owes.
type watched struct {
	m              *managed
	incarnation    int
	attached       <-chan struct{}
	stdout, stderr *lineWriter
	recorded       chan struct{} // closed once the launch is recorded, or abandoned
	window         *time.Timer
	exited         chan struct{}
}

// inContainer launches a unit that names an image. What is decided here is decided with the lock held,
// as on the host; what the engine is asked is asked without it, and the outcome recorded under it again,
// unless a stop or another attempt has superseded this one meanwhile. Lock held.
func (s *Supervisor) inContainer(m *managed, needs expansion) {
	if s.cfg.Containers == nil {
		s.failedLaunch(m, "no container engine is reached, and the unit names an image")
		return
	}
	token := s.cfg.Tokens.Issue(m.decl.ID)
	incarnation := s.cfg.Incarnations.Next(m.decl.ID)
	launch := engine.Launch{Image: m.decl.Image, Args: m.decl.Args, Env: append(s.Environment(m.decl, token), needs.env...),
		Directory: s.cfg.Root, Instance: s.cfg.Instance, Unit: m.decl.ID, Incarnation: incarnation, UID: os.Getuid(), GID: os.Getgid(),
		Devices: needs.devices, Mounts: needs.mounts, Groups: needs.groups, Network: needs.network}
	go s.launchContainer(m, m.launching, launch, token)
}

// launchContainer asks the engine for the container, attaches to it, and starts it.
func (s *Supervisor) launchContainer(m *managed, attempt int, l engine.Launch, token string) {
	c := s.cfg.Containers
	ctx, cancel := context.WithTimeout(context.Background(), engineWait)
	defer cancel()
	fail := func(why string) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if m.launching == attempt && !s.stopping {
			s.failedLaunch(m, why)
		}
	}
	if err := s.follow(); err != nil {
		fail(fmt.Sprintf("the container engine cannot be reached: %v", err))
		return
	}
	// Until it is watched, a container being launched is not mistaken for one nobody launched.
	life := fmt.Sprintf("%s#%d", l.Unit, l.Incarnation)
	s.ev.Lock()
	s.inflight[life] = true
	s.ev.Unlock()
	defer func() {
		s.ev.Lock()
		delete(s.inflight, life)
		s.ev.Unlock()
	}()
	id, err := c.Create(ctx, l)
	var absent *engine.Absent
	switch {
	case errors.As(err, &absent):
		fail(fmt.Sprintf("the image %s is not held by the container engine, and the Core never obtains one", l.Image))
		return
	case err != nil:
		fail(fmt.Sprintf("the container could not be created: %v", err))
		return
	}
	w := &watched{m: m, incarnation: l.Incarnation, recorded: make(chan struct{}), exited: make(chan struct{}),
		stdout: &lineWriter{emit: func(line string) { s.cfg.Output.Line(l.Unit, l.Incarnation, "stdout", line) }},
		stderr: &lineWriter{emit: func(line string) { s.cfg.Output.Line(l.Unit, l.Incarnation, "stderr", line) }}}
	attached, err := c.Attach(ctx, id, w.stdout, w.stderr)
	w.attached = attached
	if err != nil {
		s.remove(id)
		fail(fmt.Sprintf("the container's output could not be attached: %v", err))
		return
	}
	// Watched before it starts, so that an end however early is not missed.
	s.ev.Lock()
	s.watching[id] = w
	s.ev.Unlock()
	if err := c.Start(ctx, id); err != nil {
		s.ev.Lock()
		delete(s.watching, id)
		s.ev.Unlock()
		close(w.recorded)
		s.remove(id)
		fail(fmt.Sprintf("the container could not be started: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	defer close(w.recorded)
	r := container{c, id}
	if m.launching != attempt || s.stopping {
		// Superseded while the engine was asked: what it started is ended, and its end removes it.
		go r.kill()
		return
	}
	m.incarnation, m.running, m.exited = l.Incarnation, r, w.exited
	m.status.Incarnation, m.status.PID, m.status.Token = l.Incarnation, 0, token
	s.publish(event.StateChangedOf(m.decl.ID, pluginOf(m.decl), uint64(l.Incarnation), "", unit.Starting))
	m.status.Since = time.Now()
	s.apply(m, unit.ProcessStarted{})
	w.window = time.AfterFunc(s.policy(m).StartupWindow, func() { s.windowElapsed(m, l.Incarnation) })
}

// follow makes sure the engine's events are followed, subscribing where they are not. An engine that went
// quiet is reconciled with as soon as it is followed again, before anything else is asked of it, and is
// observable again once that is done. It fails where the engine cannot be reached.
func (s *Supervisor) follow() error {
	s.follows.Lock()
	defer s.follows.Unlock()
	s.ev.Lock()
	following := s.events
	s.ev.Unlock()
	s.mu.Lock()
	_, quiet := s.quiet[containerBackend]
	s.mu.Unlock()
	if following && !quiet {
		return nil
	}
	if !following {
		// Followed before the engine is asked what it holds, so that an end between the two is not missed.
		events, err := s.cfg.Containers.Events(s.life, s.cfg.Instance)
		if err != nil {
			return err
		}
		s.ev.Lock()
		s.events = true
		s.ev.Unlock()
		go s.read(events)
	}
	if quiet {
		if err := s.reconcile(); err != nil {
			return err
		}
		s.mu.Lock()
		s.observable(containerBackend)
		s.mu.Unlock()
	}
	return nil
}

// read concludes each container's end the engine reports. When the reports stop arriving, the engine has
// gone quiet: nothing is concluded, and its return is waited for.
func (s *Supervisor) read(events <-chan engine.Event) {
	for e := range events {
		if e.Action != "die" {
			continue
		}
		s.ev.Lock()
		w := s.watching[e.ID]
		delete(s.watching, e.ID)
		s.ev.Unlock()
		if w != nil {
			go s.containerEnded(e.ID, w, e.ExitCode)
		}
	}
	s.ev.Lock()
	s.events = false
	s.ev.Unlock()
	if s.life.Err() != nil {
		return
	}
	s.mu.Lock()
	s.goQuiet(containerBackend, time.Now())
	s.mu.Unlock()
	s.awaitReturn()
}

// awaitReturn waits for the engine to come back: one attempt to follow it at once, then one after each
// notice that its socket was bound again. Nothing looks for it on a clock; a launch attempt that reaches it
// is the other way back.
func (s *Supervisor) awaitReturn() {
	s.ev.Lock()
	if s.returning {
		s.ev.Unlock()
		return
	}
	s.returning = true
	s.ev.Unlock()
	defer func() {
		s.ev.Lock()
		s.returning = false
		s.ev.Unlock()
	}()
	ctx, cancel := context.WithCancel(s.life)
	defer cancel()
	// Watched before the first attempt, so that a return between the two is not missed.
	notices, err := s.cfg.Containers.Returned(ctx)
	if s.follow() == nil || err != nil {
		return
	}
	for range notices {
		// A socket bound is answered a moment later, once its engine listens: a few attempts close
		// together, and then the next notice.
		for try, pause := 0, 50*time.Millisecond; try < 5; try, pause = try+1, pause*2 {
			if s.follow() == nil {
				return
			}
			select {
			case <-time.After(pause):
			case <-ctx.Done():
				return
			}
		}
	}
}

// reconcile joins what the engine holds under the instance's label against what was launched: what ended
// while nobody was told is concluded with the status the engine kept, what runs is left alone and its
// output attached again, what is gone has ended, and what nobody here launched is removed.
func (s *Supervisor) reconcile() error {
	ctx, cancel := context.WithTimeout(s.life, engineWait)
	defer cancel()
	found, err := s.cfg.Containers.List(ctx, s.cfg.Instance)
	if err != nil {
		return err
	}
	held := map[string]engine.Found{}
	for _, f := range found {
		held[f.ID] = f
	}
	type ending struct {
		id     string
		w      *watched
		status int
	}
	var ended []ending
	runs := map[string]*watched{}
	var debris []string
	s.ev.Lock()
	for _, f := range found {
		if s.watching[f.ID] == nil && !s.inflight[fmt.Sprintf("%s#%d", f.Unit, f.Incarnation)] {
			debris = append(debris, f.ID)
		}
	}
	for id, w := range s.watching {
		f, ok := held[id]
		switch {
		case ok && f.Running:
			runs[id] = w
			continue
		case ok:
			ended = append(ended, ending{id, w, f.ExitCode})
		default:
			// Gone with no status kept: the process is the evidence, and there is none.
			ended = append(ended, ending{id, w, -1})
		}
		delete(s.watching, id)
	}
	s.ev.Unlock()
	for _, e := range ended {
		go s.containerEnded(e.id, e.w, e.status)
	}
	for _, id := range debris {
		go s.remove(id)
	}
	for id, w := range runs {
		// The attachment did not outlive the engine's absence; what is written from now on is kept.
		if attached, err := s.cfg.Containers.Attach(ctx, id, w.stdout, w.stderr); err == nil {
			s.ev.Lock()
			w.attached = attached
			s.ev.Unlock()
		}
	}
	return nil
}

// containerEnded concludes a container's end: the last of its output, its removal, then its exit as the
// machine reads one.
func (s *Supervisor) containerEnded(id string, w *watched, status int) {
	<-w.recorded
	s.ev.Lock()
	attached := w.attached
	s.ev.Unlock()
	select {
	case <-attached:
	case <-time.After(attachedWait):
	}
	w.stdout.flush()
	w.stderr.flush()
	s.remove(id)
	if w.window != nil {
		w.window.Stop()
	}
	s.ended(w.m, w.incarnation, status)
	close(w.exited)
}

// remove removes a container and what it holds.
func (s *Supervisor) remove(id string) {
	ctx, cancel := context.WithTimeout(context.Background(), engineWait)
	defer cancel()
	s.cfg.Containers.Remove(ctx, id)
}
