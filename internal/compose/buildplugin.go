package compose

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/yoke-project/yoke/internal/gate"
)

// PluginBuildOptions are what a Plugin's build is asked for.
type PluginBuildOptions struct {
	// Arch is the architecture to build for, in uname names; empty is the workstation's own.
	Arch string
	// Stdout and Stderr receive what the build's command prints; nil discards it.
	Stdout, Stderr io.Writer
}

// BuildPlugin runs the build contract in the Plugin project in dir and returns the path of the executable
// it left in build/<arch>/<program>. The composer sets the five variables and runs the file's command; it
// interprets no language.
func BuildPlugin(dir string, o PluginBuildOptions) (string, error) {
	report, p := CheckPlugin(dir)
	if p == nil {
		return "", refusef("%s", refusalLines(report))
	}
	arch := o.Arch
	if arch == "" {
		arch = HostArch()
	}
	exe, err := buildPlugin(p, arch, o)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(filepath.Dir(exe))
	dest := filepath.Join(p.Root(), "build", arch, p.Program)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	if err := copyExecutable(exe, dest); err != nil {
		return "", err
	}
	return dest, nil
}

// refusalLines is every refusal of a report, one per line, as the command line shows a finding.
func refusalLines(r gate.Report) string {
	var lines []string
	for _, f := range r.Findings {
		if f.Class == gate.Refusal {
			at := f.Location
			if at == "" {
				at = f.Document
			}
			lines = append(lines, fmt.Sprintf("%s  %s  %s", f.Code, at, f.Message))
		}
	}
	return strings.Join(lines, "\n")
}

// buildPlugin runs the four steps of the build contract, the file already checked, and returns the
// executable in the build's own output directory, which the caller removes.
func buildPlugin(p *gate.PluginProject, arch string, o PluginBuildOptions) (string, error) {
	spelling, known := gate.Architectures[arch]
	if !known || !slices.Contains(p.Architectures, arch) {
		return "", refusef("%s is built for %s, and %s is not among them: nothing was run", p.ID, strings.Join(p.Architectures, ", "), arch)
	}
	out, err := os.MkdirTemp("", "yoke-plugin-build-")
	if err != nil {
		return "", err
	}
	vars := []string{
		"YOKE_OUT=" + out,
		"YOKE_ARCH=" + arch,
		"YOKE_GOARCH=" + spelling.Go,
		"YOKE_RUST_TARGET=" + spelling.Rust,
		"YOKE_DEB_ARCH=" + spelling.Deb,
	}
	in := filepath.Join(p.Root(), filepath.FromSlash(p.BuildIn))
	var cmd *exec.Cmd
	if p.BuildContainer == "" {
		cmd = exec.Command("/bin/sh", "-c", p.BuildRun)
		cmd.Dir = in
		cmd.Env = append(os.Environ(), vars...)
	} else {
		engine := containerEngine()
		if engine == "" {
			os.RemoveAll(out)
			return "", refusef("%s's build runs in %s, and no container engine is on PATH: name one with CONTAINER_ENGINE", p.ID, p.BuildContainer)
		}
		args := []string{"run", "--rm"}
		if filepath.Base(engine) == "podman" {
			args = append(args, "--userns=keep-id")
		}
		args = append(args,
			"--user", strconv.Itoa(os.Getuid())+":"+strconv.Itoa(os.Getgid()),
			"-v", p.Root()+":"+p.Root(), "-v", out+":"+out, "-w", in)
		for _, v := range vars {
			args = append(args, "-e", v)
		}
		args = append(args, p.BuildContainer, "/bin/sh", "-c", p.BuildRun)
		cmd = exec.Command(engine, args...)
	}
	cmd.Stdout, cmd.Stderr = o.Stdout, o.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(out)
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", refusef("%s's build exited with status %d", p.ID, exit.ExitCode())
		}
		return "", fmt.Errorf("%s's build could not be run: %w", p.ID, err)
	}
	exe := filepath.Join(out, p.Program)
	info, err := os.Stat(exe)
	switch {
	case err != nil:
		os.RemoveAll(out)
		return "", refusef("the build ended well and left no $YOKE_OUT/%s", p.Program)
	case !info.Mode().IsRegular() || info.Mode()&0o111 == 0:
		os.RemoveAll(out)
		return "", refusef("the build left $YOKE_OUT/%s, and it is not executable", p.Program)
	}
	return exe, nil
}

// containerEngine is the engine a build that names an image runs in: CONTAINER_ENGINE, or podman, or
// docker, the first found.
func containerEngine() string {
	if e := os.Getenv("CONTAINER_ENGINE"); e != "" {
		if path, err := exec.LookPath(e); err == nil {
			return path
		}
		return ""
	}
	for _, e := range []string{"podman", "docker"} {
		if path, err := exec.LookPath(e); err == nil {
			return path
		}
	}
	return ""
}

func copyExecutable(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(filepath.Dir(to), "."+filepath.Base(to)+"-")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), to); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
