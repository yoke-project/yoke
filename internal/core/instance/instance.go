// Package instance derives every path an instance uses from its name, and takes the claim that decides
// which process serves it.
//
// The name is assigned from outside and validated before anything is derived from it; it is never
// transformed. The claim is a POSIX record lock over the whole of `<root>/instance.lock`: atomic,
// released by the kernel however its holder dies, and able to say who holds it.
package instance

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// ServiceName is the service form's name: there is one deployment per host.
const ServiceName = "yoke"

// LockName is the claim's file, directly under an instance's root.
const LockName = "instance.lock"

// Paths are what an instance's name decides.
type Paths struct {
	Name  string
	Root  string // the socket tree's root, where the claim is taken
	State string // where the two stores live
	Lock  string
}

// ValidateName refuses a name that could choose where the instance writes.
func ValidateName(name string) error {
	switch {
	case name == "":
		return errors.New("an instance name cannot be empty")
	case name == "." || name == "..":
		return fmt.Errorf("the instance name %q names a directory rather than an entry in one", name)
	case strings.ContainsRune(name, '/'):
		return fmt.Errorf("the instance name %q holds a path separator", name)
	case strings.ContainsRune(name, 0):
		return fmt.Errorf("the instance name %q holds a null byte", name)
	}
	return nil
}

// ServicePaths are the service form's: its root and its stores are the configured directories.
func ServicePaths(runtimeDir, stateDir string) Paths {
	return Paths{Name: ServiceName, Root: runtimeDir, State: stateDir, Lock: filepath.Join(runtimeDir, LockName)}
}

// ApplicationPaths are the application form's, derived from the name under the account's own
// directories.
func ApplicationPaths(name string, env func(string) string) (Paths, error) {
	if err := ValidateName(name); err != nil {
		return Paths{}, err
	}
	runtime, state := env("XDG_RUNTIME_DIR"), env("XDG_STATE_HOME")
	if runtime == "" || state == "" {
		return Paths{}, errors.New("XDG_RUNTIME_DIR and XDG_STATE_HOME must both be set")
	}
	root := filepath.Join(runtime, "yoke", name)
	return Paths{
		Name:  name,
		Root:  root,
		State: filepath.Join(state, "yoke", "instances", name, "state"),
		Lock:  filepath.Join(root, LockName),
	}, nil
}

// AlreadyRunning is the refusal of a claim another process holds.
type AlreadyRunning struct {
	Root string
	PID  int
}

func (e *AlreadyRunning) Error() string {
	return fmt.Sprintf("the instance at %s is already running, as process %d", e.Root, e.PID)
}

// Held is a claim, held for the life of the process that took it.
type Held struct {
	root string
	file *os.File
}

// The roots this process has claimed. A record lock is released when any descriptor on its file is
// closed, so a lock file is opened once by a process and never again.
var (
	claimedMu sync.Mutex
	claimed   = map[string]bool{}
)

// Claim creates root if it is absent and takes the instance: it succeeds, or it fails saying who holds it.
func Claim(root string) (*Held, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	claimedMu.Lock()
	defer claimedMu.Unlock()
	if claimed[absolute] {
		return nil, fmt.Errorf("the instance at %s is already claimed by this process", absolute)
	}

	if err := os.MkdirAll(absolute, 0o700); err != nil {
		return nil, fmt.Errorf("the instance root %s cannot be created: %w", absolute, err)
	}
	file, err := os.OpenFile(filepath.Join(absolute, LockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("the claim at %s cannot be opened: %w", absolute, err)
	}
	whole := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := syscall.FcntlFlock(file.Fd(), syscall.F_SETLK, &whole); err != nil {
		defer file.Close()
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EACCES) {
			return nil, fmt.Errorf("the claim at %s cannot be taken: %w", absolute, err)
		}
		holder := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: 0, Start: 0, Len: 0}
		if err := syscall.FcntlFlock(file.Fd(), syscall.F_GETLK, &holder); err != nil || holder.Type == syscall.F_UNLCK {
			return nil, fmt.Errorf("the instance at %s is already running, and its holder could not be asked", absolute)
		}
		return nil, &AlreadyRunning{Root: absolute, PID: int(holder.Pid)}
	}
	claimed[absolute] = true
	return &Held{root: absolute, file: file}, nil
}

// Release gives the instance up. The lock file stays: removing what a previous incarnation left is the
// next start's work.
func (c *Held) Release() error {
	claimedMu.Lock()
	defer claimedMu.Unlock()
	delete(claimed, c.root)
	return c.file.Close()
}
