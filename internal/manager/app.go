package manager

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yoke-project/yoke/internal/compose"
	"github.com/yoke-project/yoke/internal/gate"
	"github.com/yoke-project/yoke/internal/install"
	"github.com/yoke-project/yoke/internal/installroot"
	"github.com/yoke-project/yoke/internal/pkgfile"
	"github.com/yoke-project/yoke/internal/truststore"
)

// The lifecycle verbs on artifacts: they act on the install root and reach no process.
func init() {
	register("app", "install", verb{"<package> [--seal-credential <file>]", (*call).appInstall})
	register("app", "list", verb{"", (*call).appList})
	register("app", "show", verb{"<id>", (*call).appShow})
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
		Credential: credential, Host: pkgfile.Host{Arch: compose.HostArch(), Models: []uint16{gate.DescriptionModel}, Core: core}})
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
			fmt.Fprintf(c.out, "  %s  %s  verified by %s  sealed %t  %s  shared %t  %s\n", v.Version, rec.State,
				rec.VerifiedBy, rec.Sealed, rec.Digest, rec.Shared, rec.InstalledAt.Format("2006-01-02T15:04:05Z"))
		}
		return nil
	}
	return refusal{fmt.Errorf("%s is not installed under %s", c.args[0], r.Path)}
}
