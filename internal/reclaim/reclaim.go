// Package reclaim removes what is installed: the bundles a removal names, refused while any record points
// at one of them, and — in the one reclaiming pass — whatever nothing references any more.
//
// A removal writes `withdrawn` into an install record before it deletes anything, so that the interval
// between deciding nothing points at a version and that version being gone cannot be entered by a new pin.
// Only the pass reaches the engine, and it considers only images in Yoke's reserved namespace.
package reclaim

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/core/instance"
	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/version"
)

// Options say where the account's artifacts, records and runtime directories are, and how the engine is
// reached. Without an engine the pass leaves images alone.
type Options struct {
	InstallRoot, Instances, Runtime string
	Engine                          func(ctx context.Context) (*engine.Engine, error)
	// withdrawn is called between the two acts of a removal, for a test to look at.
	withdrawn func(dir string)
}

var (
	// ErrNotInstalled is the refusal of a removal or a pass naming what is not installed.
	ErrNotInstalled = errors.New("not installed")
	// ErrPinned is the refusal of a removal while a record points at what it would remove.
	ErrPinned = errors.New("a record points at it")
)

// Report names what a pass removed.
type Report struct {
	Removed []string
	// Unconsidered says why the pass left the engine's store alone, where it could not reach it.
	Unconsidered string
}

// Remove removes the versions of an application named — one, or every one where ver is empty — and, once
// the last has gone, the application's directory. It is refused, removing nothing, while any record points
// at any of them.
func Remove(o Options, id, ver string) ([]string, error) {
	app, err := find(o, id)
	if err != nil {
		return nil, err
	}
	var targets []string
	for _, v := range app.Versions {
		if ver == "" || v.Version == ver {
			targets = append(targets, v.Version)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%s %s is %w under %s", id, ver, ErrNotInstalled, o.InstallRoot)
	}
	var pinned []string
	for _, v := range targets {
		names, err := instancerecord.PointingAt(o.Instances, id, v)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			pinned = append(pinned, fmt.Sprintf("%s is pinned to %s", n, v))
		}
	}
	if len(pinned) > 0 {
		return nil, fmt.Errorf("%s cannot be removed while %w (%s): re-point first", id, ErrPinned,
			strings.Join(pinned, ", "))
	}
	var removed []string
	for _, v := range targets {
		if err := withdraw(o, id, v); err != nil {
			return removed, err
		}
		removed = append(removed, v)
	}
	return removed, nil
}

// find is the application under the root.
func find(o Options, id string) (installroot.App, error) {
	apps, err := installroot.Root{Path: o.InstallRoot}.List()
	if err != nil {
		return installroot.App{}, err
	}
	for _, a := range apps {
		if a.ID == id {
			return a, nil
		}
	}
	return installroot.App{}, fmt.Errorf("%s is %w under %s", id, ErrNotInstalled, o.InstallRoot)
}

// withdraw removes one version: its record says `withdrawn`, nothing has pinned it meanwhile, and then its
// directory goes, and the application's after it if it was the last. A pin that entered between the check
// and the record is honoured: the record is put back and the removal refused.
func withdraw(o Options, id, ver string) error {
	dir := filepath.Join(o.InstallRoot, id, ver)
	if rec, err := installroot.ReadRecord(dir); err == nil && rec.State != installroot.Withdrawn {
		previous := rec
		rec.State = installroot.Withdrawn
		inst := &installroot.Installation{Dir: dir}
		if err := inst.Write(rec); err != nil {
			return err
		}
		names, err := instancerecord.PointingAt(o.Instances, id, ver)
		if err != nil || len(names) > 0 {
			inst.Write(previous)
			if err != nil {
				return err
			}
			return fmt.Errorf("%s %s cannot be removed: %w, %s, pinned while it was being removed", id, ver, ErrPinned,
				strings.Join(names, ", "))
		}
	}
	if o.withdrawn != nil {
		o.withdrawn(dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(o.InstallRoot, id)); err != nil && !errors.Is(err, syscall.ENOTEMPTY) &&
		!errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Prune is the one reclaiming pass, over one application or — where id is empty — over everything the
// account can see: superseded versions nothing points at, incomplete installs, Yoke's images no installed
// bundle references, an ephemeral record a failure left, and the runtime directory of an instance whose
// claim is free. A pre-release is never collected. An engine the pass cannot reach leaves images alone,
// and the report says so: an account need not run containers for the rest to be reclaimed.
func Prune(o Options, id string) (Report, error) {
	var r Report
	apps, err := installroot.Root{Path: o.InstallRoot}.List()
	if err != nil {
		return r, err
	}
	if id != "" {
		if _, err := find(o, id); err != nil {
			return r, err
		}
	}
	for _, a := range apps {
		if id != "" && a.ID != id {
			continue
		}
		for _, v := range collectable(a) {
			names, err := instancerecord.PointingAt(o.Instances, a.ID, v.Version)
			if err != nil {
				return r, err
			}
			if v.State == installroot.Complete && len(names) > 0 {
				continue
			}
			if err := withdraw(o, a.ID, v.Version); err != nil {
				return r, err
			}
			r.Removed = append(r.Removed, a.ID+" "+v.Version)
		}
	}
	if err := records(o, id, &r); err != nil {
		return r, err
	}
	if err := runtimes(o, id, &r); err != nil {
		return r, err
	}
	if o.Engine == nil {
		return r, nil
	}
	err = images(o, id, &r)
	var unreachable *engine.Unreachable
	if errors.As(err, &unreachable) {
		r.Unconsidered = err.Error()
		return r, nil
	}
	return r, err
}

// collectable are an application's incomplete installs and the complete versions a higher-sorting
// complete version which is not a pre-release supersedes. A pre-release is neither.
func collectable(a installroot.App) []installroot.Version {
	var out []installroot.Version
	var newest *version.Version
	for i := len(a.Versions) - 1; i >= 0; i-- {
		v := a.Versions[i]
		parsed := version.MustParse(v.Version)
		switch {
		case v.State == installroot.Incomplete:
			out = append(out, v)
		case v.State != installroot.Complete || parsed.IsPrerelease():
		case newest == nil:
			newest = &parsed
		default:
			out = append(out, v)
		}
	}
	slices.Reverse(out)
	return out
}

// images removes the images Yoke obtained that no installed bundle references: for each, its reserved tags
// go and then, where it carries no other tag, the image. An image without a reserved tag is the account's
// and is never considered.
func images(o Options, id string, r *Report) error {
	ctx := context.Background()
	e, err := o.Engine(ctx)
	if err != nil {
		return err
	}
	held, err := e.Images(ctx)
	if err != nil {
		return err
	}
	apps, err := installroot.Root{Path: o.InstallRoot}.List()
	if err != nil {
		return err
	}
	installed := map[string]bool{}
	for _, a := range apps {
		for _, v := range a.Versions {
			installed[a.ID+":"+v.Version] = true
		}
	}
	for _, img := range held {
		var stale []string
		kept := false
		for _, tag := range img.Tags {
			app, ver, ok := engine.Reserved(tag)
			if ok && (id == "" || app == id) && !installed[app+":"+ver] {
				stale = append(stale, tag)
			} else {
				kept = true
			}
		}
		for _, tag := range stale {
			if err := e.RemoveImage(ctx, tag); err != nil {
				return err
			}
			r.Removed = append(r.Removed, "image "+tag)
		}
		if len(stale) > 0 && !kept {
			if err := e.RemoveImage(ctx, img.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// records removes an ephemeral record whose run failed, and its state with it.
func records(o Options, id string, r *Report) error {
	listed, err := instancerecord.List(o.Instances)
	if err != nil {
		return err
	}
	for _, l := range listed {
		rec := l.Record
		if rec == nil || !rec.Ephemeral || rec.Outcome != instancerecord.Failed || (id != "" && rec.ID != id) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(o.Instances, l.Name)); err != nil {
			return err
		}
		r.Removed = append(r.Removed, "record "+l.Name)
	}
	return nil
}

// runtimes removes the runtime directory of every instance whose claim is free. The pass takes the claim
// itself, so nothing can start in the directory while it goes; a claim another process holds leaves it.
func runtimes(o Options, id string, r *Report) error {
	entries, err := os.ReadDir(o.Runtime)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		dir := filepath.Join(o.Runtime, e.Name())
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, instance.LockName)); err != nil {
			continue
		}
		if id != "" {
			if rec, err := instancerecord.Read(o.Instances, e.Name()); err != nil || rec.ID != id {
				continue
			}
		}
		held, err := instance.Claim(dir, 0o700)
		var running *instance.AlreadyRunning
		if errors.As(err, &running) {
			continue
		}
		if err != nil {
			return err
		}
		err = os.RemoveAll(dir)
		held.Release()
		if err != nil {
			return err
		}
		r.Removed = append(r.Removed, "runtime directory "+e.Name())
	}
	return nil
}
