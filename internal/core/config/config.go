// Package config reads the Core's configuration in the service form: `core.yaml`, every key it leaves
// out taking its default, and the environment overriding one value at a time.
//
// It is read once, at startup, and never again. A value is typed by the key it sets and never by how it
// is written, an unknown key is refused, and a value from the environment passes the same checks as one
// from the file. Whatever does not pass prevents startup.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// DefaultPath is where the service form's configuration is read from, unless YOKE_CONFIG names another.
const DefaultPath = "/etc/yoke/core.yaml"

// ConventionalComposition is the composition in force when nothing names another.
const ConventionalComposition = "/etc/yoke/deployment.yaml"

// Config is the process's configuration. It is about the process and never about the deployment.
type Config struct {
	StateDir   string
	RuntimeDir string
	Plugins    Plugins
	Engine     string
	Log        Log
}

// Plugins says where Plugins are found, and how often the directory is scanned.
type Plugins struct {
	Manifests    string
	Executables  string
	ScanInterval time.Duration
}

// Log says what the process logger emits.
type Log struct {
	Level string
}

// Defaults is the configuration of an empty `core.yaml`.
func Defaults() Config {
	return Config{
		StateDir:   "/var/lib/yoke",
		RuntimeDir: "/run/yoke",
		Plugins: Plugins{
			Manifests:    "/etc/yoke/plugins.d",
			Executables:  "/usr/lib/yoke/plugins",
			ScanInterval: 30 * time.Second,
		},
		Engine: "unix:///run/podman/podman.sock",
		Log:    Log{Level: "info"},
	}
}

// A key of the file: its dotted path, and how a value written for it is set.
type key struct {
	path string
	set  func(*Config, string) error
}

var keys = []key{
	{"state_dir", func(c *Config, v string) error { c.StateDir = v; return nil }},
	{"runtime_dir", func(c *Config, v string) error { c.RuntimeDir = v; return nil }},
	{"plugins.manifests", func(c *Config, v string) error { c.Plugins.Manifests = v; return nil }},
	{"plugins.executables", func(c *Config, v string) error { c.Plugins.Executables = v; return nil }},
	{"plugins.scan_interval", func(c *Config, v string) error {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("%q is not a duration, such as 30s", v)
		}
		c.Plugins.ScanInterval = d
		return nil
	}},
	{"engine", func(c *Config, v string) error { c.Engine = v; return nil }},
	{"log.level", func(c *Config, v string) error {
		switch v {
		case "debug", "info", "warn", "error":
			c.Log.Level = v
			return nil
		}
		return fmt.Errorf("%q is not one of debug, info, warn, error", v)
	}},
}

// variable is the environment variable that overrides a key: its path, upper-cased, levels joined by `_`.
func variable(path string) string {
	return "YOKE_" + strings.ToUpper(strings.ReplaceAll(path, ".", "_"))
}

// Load reads the file YOKE_CONFIG names, or the default one, with the environment's overrides.
func Load(env func(string) string) (Config, error) {
	path := env("YOKE_CONFIG")
	if path == "" {
		path = DefaultPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("the configuration %s cannot be read: %w", path, err)
	}
	c, err := Parse(data, env)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse reads a `core.yaml`, applies the environment's overrides, and validates both.
func Parse(data []byte, env func(string) string) (Config, error) {
	c := Defaults()
	written := map[string]string{}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return Config{}, fmt.Errorf("it does not parse: %w", err)
	}
	if len(document.Content) > 0 {
		if err := collect(document.Content[0], "", written); err != nil {
			return Config{}, err
		}
	}

	for _, k := range keys {
		if value, ok := written[k.path]; ok {
			if err := k.set(&c, value); err != nil {
				return Config{}, fmt.Errorf("%s: %w", k.path, err)
			}
		}
		if value := env(variable(k.path)); value != "" {
			if err := k.set(&c, value); err != nil {
				return Config{}, fmt.Errorf("%s: %w", variable(k.path), err)
			}
		}
	}
	return c, nil
}

// collect walks a mapping, recording every scalar under its dotted path and refusing every key the
// configuration does not have.
func collect(node *yaml.Node, prefix string, into map[string]string) error {
	if node.Kind != yaml.MappingNode {
		if prefix == "" && node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
			return nil
		}
		return errors.New(where(prefix) + "is not a mapping")
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		path := prefix + node.Content[i].Value
		value := node.Content[i+1]
		if value.Kind == yaml.MappingNode {
			if !isSection(path) {
				return fmt.Errorf("%s: unknown key", path)
			}
			if err := collect(value, path+".", into); err != nil {
				return err
			}
			continue
		}
		if !isKey(path) {
			return fmt.Errorf("%s: unknown key", path)
		}
		// A value is typed by its key: whatever the notation says, it is read as written.
		if value.Kind != yaml.ScalarNode || value.Tag == "!!null" {
			return fmt.Errorf("%s: has no value", path)
		}
		into[path] = value.Value
	}
	return nil
}

func where(prefix string) string {
	if prefix == "" {
		return "the document "
	}
	return strings.TrimSuffix(prefix, ".") + " "
}

func isKey(path string) bool {
	for _, k := range keys {
		if k.path == path {
			return true
		}
	}
	return false
}

func isSection(path string) bool {
	for _, k := range keys {
		if strings.HasPrefix(k.path, path+".") {
			return true
		}
	}
	return false
}

// Composition is the composition document in force: the conventional path, overridden by
// YOKE_COMPOSITION, overridden by the flag.
func Composition(flag string, env func(string) string) string {
	if flag != "" {
		return flag
	}
	if chosen := env("YOKE_COMPOSITION"); chosen != "" {
		return chosen
	}
	return ConventionalComposition
}
