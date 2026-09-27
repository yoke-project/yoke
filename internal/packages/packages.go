// Package packages makes the definitions' crate and wheel: it generates their sources, tests them, and
// packages them at a version, each in a container image the repository pins.
package packages

import "errors"

// A language the definitions are packaged for.
type Language string

const (
	Rust   Language = "rust"
	Python Language = "python"
)

var errNotYet = errors.New("the definitions are not packaged yet")

// Generate writes the sources the definitions generate for lang into out.
func Generate(root string, lang Language, out string) error { return errNotYet }

// Test builds the package for lang and runs its tests.
func Test(root string, lang Language) error { return errNotYet }

// Package packages both at version into out, and returns the crate's path and the wheel's.
func Package(root, version, out string) (crate, wheel string, err error) { return "", "", errNotYet }

// CrateSources reads the files a crate carries, keyed by their path inside it.
func CrateSources(path string) (map[string][]byte, error) { return nil, errNotYet }

// WheelSources reads the files a wheel carries, keyed by their path inside it.
func WheelSources(path string) (map[string][]byte, error) { return nil, errNotYet }

// Images returns every container image a script names.
func Images(script string) []string { return nil }
