package manager

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/core/engine"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/install"
	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/instancerecord"
	"github.com/yoke-project/yoke/internal/pkgfile"
	"github.com/yoke-project/yoke/internal/reclaim"
	"github.com/yoke-project/yoke/internal/truststore"
)

// The lifecycle verbs on artifacts: they act on the install root and reach no process.
func init() {
	register("app", "install", verb{"<package> [--seal-credential <file>]", (*call).appInstall})
	register("app", "list", verb{"", (*call).appList})
	register("app", "show", verb{"<id>", (*call).appShow})
	register("app", "remove", verb{"<id> [--version <v>]", (*call).appRemove})
	register("app", "prune", verb{"[<id>]", (*call).appPrune})
}

// trust is this host's trust store: the file the system keeps, or — where it keeps none — no key
// trusted and no signature required, unless the environment tightens it.
func trust(getenv func(string) string) (*truststore.Store, error) {
	s, err := truststore.Load(truststore.Path, getenv)
	if errors.Is(err, fs.ErrNotExist) {
		return truststore.Empty(getenv), nil
	}
	if err != nil {
		return nil, refusal{err}
	}
	return s, nil
}

func (c *call) appInstall() error {
	var credentialFile *string
	if _, _, _, err := c.flags(func(fs *flag.FlagSet) {
		credentialFile = fs.String("seal-credential", "", "a file holding the credential that opens a sealed package")
	}); err != nil {
		return err
	}
	if err := c.positional(1, "a package"); err != nil {
		return err
	}
	cfg, err := LoadConfig(c.getenv)
	if err != nil {
		return refusal{err}
	}
	store, err := trust(c.getenv)
	if err != nil {
		return err
	}
	supplied := ""
	if *credentialFile != "" {
		data, err := os.ReadFile(*credentialFile)
		if err != nil {
			return err
		}
		supplied = string(data)
	}
	credential, err := store.Credential(supplied)
	if err != nil {
		return refusal{err}
	}
	binary, err := exec.LookPath(cfg.Core)
	if err != nil {
		return refusal{fmt.Errorf("the Core %s is not there: %w", cfg.Core, err)}
	}
	core, err := install.CoreVersion(binary)
	if err != nil {
		return refusal{err}
	}
	res, err := install.Install(install.Options{Package: c.args[0], Root: cfg.InstallRoot, Keys: store, Policy: store.Policy(),
		Credential: credential, Host: pkgfile.Host{Arch: compose.HostArch(), Models: []uint16{gate.DescriptionModel}, Core: core},
		Engine: func(ctx context.Context) (*engine.Engine, error) { return engine.Reach(ctx, cfg.Engine) }, Out: c.out})
	var refused *install.Refusal
	switch {
	case errors.As(err, &refused):
		return refusal{err}
	case err != nil:
		return err
	case res.Present:
		fmt.Fprintf(c.out, "already installed at %s: nothing was done\n", res.Dir)
	default:
		fmt.Fprintf(c.out, "installed at %s\n", res.Dir)
	}
	return nil
}

func (c *call) root() (installroot.Root, error) {
	cfg, err := LoadConfig(c.getenv)
	if err != nil {
		return installroot.Root{}, refusal{err}
	}
	return installroot.Root{Path: cfg.InstallRoot}, nil
}

func (c *call) appList() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(0, "no subject"); err != nil {
		return err
	}
	r, err := c.root()
	if err != nil {
		return err
	}
	apps, err := r.List()
	if err != nil {
		return err
	}
	for _, a := range apps {
		for _, v := range a.Versions {
			fmt.Fprintf(c.out, "%s %s %s\n", a.ID, v.Version, v.State)
		}
	}
	return nil
}

func (c *call) appShow() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	if err := c.positional(1, "an application's identifier"); err != nil {
		return err
	}
	r, err := c.root()
	if err != nil {
		return err
	}
	apps, err := r.List()
	if err != nil {
		return err
	}
	for _, a := range apps {
		if a.ID != c.args[0] {
			continue
		}
		fmt.Fprintf(c.out, "%s, installed by %s\n", a.ID, a.Owner)
		for _, v := range a.Versions {
			rec, err := installroot.ReadRecord(filepath.Join(r.Path, a.ID, v.Version))
			if err != nil {
				fmt.Fprintf(c.out, "  %s  unreadable record: %v\n", v.Version, err)
				continue
			}
			// An unsigned package was verified by no key.
			verified := "verified by " + rec.VerifiedBy
			if rec.VerifiedBy == "unsigned" {
				verified = "unsigned"
			}
			fmt.Fprintf(c.out, "  %s  %s  %s  sealed %t  %s  shared %t  %s\n", v.Version, rec.State,
				verified, rec.Sealed, rec.Digest, rec.Shared, rec.InstalledAt.Format("2006-01-02T15:04:05Z"))
		}
		return nil
	}
	return refusal{fmt.Errorf("%s is not installed under %s", c.args[0], r.Path)}
}

// reclaiming is where removing and the reclaiming pass act: the install root, the account's records and
// runtime directories, and the engine.
func (c *call) reclaiming() (reclaim.Options, error) {
	cfg, err := LoadConfig(c.getenv)
	if err != nil {
		return reclaim.Options{}, refusal{err}
	}
	return reclaim.Options{InstallRoot: cfg.InstallRoot, Instances: instancerecord.Root(c.getenv),
		Runtime: filepath.Join(c.getenv("XDG_RUNTIME_DIR"), "yoke"),
		Engine:  func(ctx context.Context) (*engine.Engine, error) { return engine.Reach(ctx, cfg.Engine) }}, nil
}

func (c *call) appRemove() error {
	var ver *string
	if _, _, _, err := c.flags(func(fs *flag.FlagSet) {
		ver = fs.String("version", "", "the one version to remove; every version where none is given")
	}); err != nil {
		return err
	}
	if err := c.positional(1, "an application's identifier"); err != nil {
		return err
	}
	o, err := c.reclaiming()
	if err != nil {
		return err
	}
	removed, err := reclaim.Remove(o, c.args[0], *ver)
	for _, v := range removed {
		fmt.Fprintf(c.out, "removed %s %s\n", c.args[0], v)
	}
	return reclaimed(err)
}

func (c *call) appPrune() error {
	if _, _, _, err := c.flags(nil); err != nil {
		return err
	}
	id := ""
	if len(c.args) > 0 {
		if err := c.positional(1, "at most an application's identifier"); err != nil {
			return err
		}
		id = c.args[0]
	}
	o, err := c.reclaiming()
	if err != nil {
		return err
	}
	r, err := reclaim.Prune(o, id)
	for _, what := range r.Removed {
		fmt.Fprintf(c.out, "removed %s\n", what)
	}
	if r.Unconsidered != "" {
		fmt.Fprintf(c.out, "images were not considered: %s\n", r.Unconsidered)
	}
	if err == nil && len(r.Removed) == 0 {
		fmt.Fprintln(c.out, "nothing to reclaim")
	}
	return reclaimed(err)
}

// reclaimed is a removal's or a pass's error: naming what is not installed, and removing what a record
// points at, are refusals; anything else is a fault.
func reclaimed(err error) error {
	if errors.Is(err, reclaim.ErrNotInstalled) || errors.Is(err, reclaim.ErrPinned) {
		return refusal{err}
	}
	return err
}
