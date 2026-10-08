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
func (s *Supervisor) inContainer(m *managed) {
	if s.cfg.Containers == nil {
		s.failedLaunch(m, "no container engine is reached, and the unit names an image")
		return
	}
	token := s.cfg.Tokens.Issue(m.decl.ID)
	incarnation := s.cfg.Incarnations.Next(m.decl.ID)
	launch := engine.Launch{Image: m.decl.Image, Args: m.decl.Args, Env: s.Environment(m.decl, token), Directory: s.cfg.Root,
		Instance: s.cfg.Instance, Unit: m.decl.ID, Incarnation: incarnation, UID: os.Getuid(), GID: os.Getgid()}
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
	if err := s.followEvents(ctx); err != nil {
		fail(fmt.Sprintf("the container engine's events cannot be followed: %v", err))
		return
	}
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
	w.attached, err = c.Attach(ctx, id, w.stdout, w.stderr)
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
	s.publish(event.StateChanged(m.decl.ID, uint64(l.Incarnation), "", unit.Starting))
	m.status.Since = time.Now()
	s.apply(m, unit.ProcessStarted{})
	w.window = time.AfterFunc(s.policy(m).StartupWindow, func() { s.windowElapsed(m, l.Incarnation) })
}

// followEvents subscribes to the engine's events about this instance's containers, once; a stream that
// ended is subscribed to again by the next launch.
func (s *Supervisor) followEvents(ctx context.Context) error {
	s.ev.Lock()
	defer s.ev.Unlock()
	if s.events {
		return nil
	}
	if s.watching == nil {
		s.watching = map[string]*watched{}
	}
	events, err := s.cfg.Containers.Events(context.Background(), s.cfg.Instance)
	if err != nil {
		return err
	}
	s.events = true
	go func() {
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
	}()
	return nil
}

// containerEnded concludes a container's end: the last of its output, its removal, then its exit as the
// machine reads one.
func (s *Supervisor) containerEnded(id string, w *watched, status int) {
	<-w.recorded
	select {
	case <-w.attached:
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
