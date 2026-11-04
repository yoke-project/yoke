package compose

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/yoke-project/yoke/internal/gate"
)

// PluginFile is the file at a Plugin project's root, which every step reads.
const PluginFile = "yoke-plugin.yaml"

// ProjectKind is which of the composer's two kinds of project a directory is.
type ProjectKind int

const (
	ApplicationProject ProjectKind = iota + 1
	PluginProject
)

func (k ProjectKind) String() string {
	switch k {
	case ApplicationProject:
		return "an application project"
	case PluginProject:
		return "a Plugin project"
	}
	return "no project"
}

// KindOf says which kind of project the directory is: the one whose file is at its root. A directory
// holding both files, or neither, is refused.
func KindOf(dir string) (ProjectKind, error) {
	plugin := exists(filepath.Join(dir, PluginFile))
	application := exists(filepath.Join(dir, Descriptor))
	switch {
	case plugin && application:
		return 0, refusef("%s holds both %s and %s: a directory is one kind of project, a Plugin's or an application's", dir, PluginFile, Descriptor)
	case plugin:
		return PluginProject, nil
	case application:
		return ApplicationProject, nil
	}
	return 0, refusef("%s is no project: neither %s nor %s is at its root", dir, PluginFile, Descriptor)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

// CheckPlugin checks the Plugin project in the directory: its file, and the Manifest the file names. Only
// what the file names is read; try/ is an application project of its own.
func CheckPlugin(dir string) (gate.Report, *gate.PluginProject) {
	return gate.CheckPluginProject(gate.Read(filepath.Join(dir, PluginFile), gate.Descriptor))
}
