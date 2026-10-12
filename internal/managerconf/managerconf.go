// Package managerconf is `manager.conf`: the account's machine configuration, which the manager and the
// guardian both read — it parameterises the machine and the account, not one tool.
package managerconf

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"go.yaml.in/yaml/v3"

	"github.com/yoke-project/yoke/internal/gate"
)

// Config is the account's machine configuration: where bundles are installed, how the engine is reached,
// which Core to execute, and the retention ceiling this account puts on what a description declares.
type Config struct {
	InstallRoot string
	Engine      string
	Core        string
	// Retention is the ceiling per limit — `age`, `bytes`, `entries` — as written; an absent limit has none.
	Retention map[string]string
}

type configFile struct {
	InstallRoot *string `yaml:"install_root"`
	Engine      *string `yaml:"engine"`
	Core        *string `yaml:"core"`
	Retention   *struct {
		Age     *string `yaml:"age"`
		Bytes   *string `yaml:"bytes"`
		Entries *string `yaml:"entries"`
	} `yaml:"retention"`
}

// Path is where the configuration is read from: `$XDG_CONFIG_HOME/yoke/manager.conf`, the
// configuration home being `$HOME/.config` where the variable is unset.
func Path(getenv func(string) string) string {
	home := getenv("XDG_CONFIG_HOME")
	if home == "" {
		home = filepath.Join(getenv("HOME"), ".config")
	}
	return filepath.Join(home, "yoke", "manager.conf")
}

// Load reads the configuration. A missing file is not an error: every value takes its default from
// the host's conventions. An unknown key is refused at any level.
func Load(getenv func(string) string) (Config, error) {
	c := Config{
		InstallRoot: "/var/lib/yoke/apps",
		Engine:      "unix://" + filepath.Join(getenv("XDG_RUNTIME_DIR"), "podman", "podman.sock"),
		Core:        "yoke-core",
		Retention:   map[string]string{},
	}
	path := Path(getenv)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return Config{}, err
	}
	var f configFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) { // an empty file states nothing
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if f.InstallRoot != nil {
		if !filepath.IsAbs(*f.InstallRoot) {
			return Config{}, fmt.Errorf("%s: install_root %q is not an absolute path", path, *f.InstallRoot)
		}
		c.InstallRoot = *f.InstallRoot
	}
	if f.Engine != nil {
		c.Engine = *f.Engine
	}
	if f.Core != nil {
		c.Core = *f.Core
	}
	if r := f.Retention; r != nil {
		if r.Age != nil {
			if _, ok := gate.ParseDuration(*r.Age); !ok && *r.Age != "0" {
				return Config{}, fmt.Errorf("%s: retention.age %q is not an integer followed by s, m, h or d", path, *r.Age)
			}
			c.Retention["age"] = *r.Age
		}
		if r.Bytes != nil {
			if _, ok := gate.ParseSize(*r.Bytes); !ok {
				return Config{}, fmt.Errorf("%s: retention.bytes %q is not an integer followed by KB, MB or GB", path, *r.Bytes)
			}
			c.Retention["bytes"] = *r.Bytes
		}
		if r.Entries != nil {
			if _, err := strconv.ParseUint(*r.Entries, 10, 64); err != nil {
				return Config{}, fmt.Errorf("%s: retention.entries %q is not a count", path, *r.Entries)
			}
			c.Retention["entries"] = *r.Entries
		}
	}
	return c, nil
}
