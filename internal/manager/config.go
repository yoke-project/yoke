// Package manager is `yoke`: one command whose lifecycle verbs act on declarations and whose client verbs
// act on a running instance, through the published administrative library and nothing else.
package manager

import "github.com/yoke-project/yoke/internal/managerconf"

// Config is the account's machine configuration, which the guardian reads as well.
type Config = managerconf.Config

// ConfigPath is where the configuration is read from.
func ConfigPath(getenv func(string) string) string { return managerconf.Path(getenv) }

// LoadConfig reads the configuration.
func LoadConfig(getenv func(string) string) (Config, error) { return managerconf.Load(getenv) }
