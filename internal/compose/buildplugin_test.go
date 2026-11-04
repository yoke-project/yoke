package compose

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// recording is a build command that writes its working directory and the five variables into the file
// given, one per line, then writes the program into $YOKE_OUT.
func recording(record string) string {
	return `{ pwd; echo "$YOKE_OUT"; echo "$YOKE_ARCH"; echo "$YOKE_GOARCH"; echo "$YOKE_RUST_TARGET"; echo "$YOKE_DEB_ARCH"; } > ` + record +
		` && printf '#!/bin/sh\necho greeter\n' > "$YOKE_OUT/greeter" && chmod +x "$YOKE_OUT/greeter"`
}

// withRun is the greeter's file with its build command replaced.
func withRun(run string) string {
	old := `  run: CGO_ENABLED=0 GOOS=linux GOARCH="$YOKE_GOARCH" go build -trimpath -o "$YOKE_OUT/greeter" .`
	return strings.Replace(greeterFile, old, "  run: '"+strings.ReplaceAll(run, "'", "''")+"'", 1)
}

func lines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the build recorded nothing: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// std: yoke:building-a-plugin.01
func TestTheBuildRunsTheFilesCommandWithTheFiveVariablesAndLeavesTheExecutable(t *testing.T) {
	record := filepath.Join(t.TempDir(), "record")
	dir := pluginProject(t, withRun(recording(record)))
	before := raw(t, filepath.Join(dir, "plugin", "com.example.greeter.yaml"))

	if code, out := run("build", "--arch", "aarch64", "--dir", dir); code != Done {
		t.Fatalf("build --arch aarch64: %d %s", code, out)
	}
	got := lines(t, record)
	if got[0] != filepath.Join(dir, "plugin") {
		t.Errorf("the command ran in %s, not in build.in", got[0])
	}
	if strings.Join(got[2:], " ") != "aarch64 arm64 aarch64-unknown-linux-gnu arm64" {
		t.Errorf("the variables for aarch64 are %v", got[2:])
	}
	if got[1] == "" || strings.HasPrefix(got[1], dir) {
		t.Errorf("YOKE_OUT is %q, not a directory of its own", got[1])
	}
	if _, err := os.Stat(got[1]); err == nil {
		t.Errorf("YOKE_OUT %s is left behind", got[1])
	}
	assertExecutable(t, filepath.Join(dir, "build", "aarch64", "greeter"))

	if HostArch() != "x86_64" {
		t.Skipf("the workstation's own architecture is %s; the case is written for x86_64", HostArch())
	}
	if code, out := run("build", "--dir", dir); code != Done {
		t.Fatalf("build: %d %s", code, out)
	}
	if got := lines(t, record); strings.Join(got[2:], " ") != "x86_64 amd64 x86_64-unknown-linux-gnu amd64" {
		t.Errorf("the variables for the workstation's own are %v", got[2:])
	}
	assertExecutable(t, filepath.Join(dir, "build", "x86_64", "greeter"))
	if raw(t, filepath.Join(dir, "plugin", "com.example.greeter.yaml")) != before {
		t.Errorf("the build changed the Manifest kept in plugin/")
	}
}

func assertExecutable(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil || info.Mode()&0o111 == 0 || !info.Mode().IsRegular() {
		t.Errorf("%s is not an executable: %v %v", path, info, err)
	}
}

// std: yoke:building-a-plugin.02
func TestAnArchitectureTheFileDoesNotListIsRefusedBeforeAnythingRuns(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "ran")
	file := strings.Replace(withRun("touch "+mark), "architectures: [x86_64, aarch64]", "architectures: [x86_64]", 1)
	dir := pluginProject(t, file)
	code, out := run("build", "--arch", "aarch64", "--dir", dir)
	if code == Done || !strings.Contains(out, "aarch64") || !strings.Contains(out, "x86_64") {
		t.Errorf("an architecture the file does not list is not refused naming both: %d %s", code, out)
	}
	os.WriteFile(filepath.Join(dir, PluginFile), []byte(strings.Replace(file, "language: go", "language: java", 1)), 0o644)
	code, out = run("build", "--dir", dir)
	if code == Done || !strings.Contains(out, "java") {
		t.Errorf("a file the validator refuses is not refused with its finding: %d %s", code, out)
	}
	if _, err := os.Stat(mark); err == nil {
		t.Errorf("the command ran")
	}
	if _, err := os.Stat(filepath.Join(dir, "build")); err == nil {
		t.Errorf("build/ was created")
	}
}

// std: yoke:building-a-plugin.03
func TestACommandThatFailsOrThatLeavesNoExecutableFailsTheBuild(t *testing.T) {
	for _, c := range []struct{ run, says string }{
		{"echo compiling; exit 3", "3"},
		{"true", "$YOKE_OUT/greeter"},
		{`echo x > "$YOKE_OUT/greeter"`, "not executable"},
	} {
		dir := pluginProject(t, withRun(c.run))
		code, out := run("build", "--dir", dir)
		if code == Done || !strings.Contains(out, c.says) {
			t.Errorf("%q: %d %s", c.run, code, out)
		}
		if c.says == "3" && !strings.Contains(out, "compiling") {
			t.Errorf("the command's output is not passed through: %s", out)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, "build", HostArch())); len(entries) != 0 {
			t.Errorf("%q left %v in build/", c.run, entries)
		}
	}
}

// std: yoke:building-a-plugin.04
func TestTheComposerSetsTheFiveVariablesAndNothingElseOfTheBuild(t *testing.T) {
	record := filepath.Join(t.TempDir(), "env")
	dir := pluginProject(t, withRun(`env > `+record+` && printf x > "$YOKE_OUT/greeter" && chmod +x "$YOKE_OUT/greeter"`))
	t.Setenv("PROBE", "1")
	invoker := environ()
	if code, out := run("build", "--dir", dir); code != Done {
		t.Fatalf("build: %d %s", code, out)
	}
	got := map[string]string{}
	for _, l := range lines(t, record) {
		if k, v, ok := strings.Cut(l, "="); ok {
			got[k] = v
		}
	}
	// The shell sets a few of its own; they are not the composer's.
	for _, own := range []string{"PWD", "OLDPWD", "SHLVL", "_"} {
		delete(got, own)
		delete(invoker, own)
	}
	for k, v := range invoker {
		if got[k] != v {
			t.Errorf("the invoker's %s=%q reached the build as %q", k, v, got[k])
		}
		delete(got, k)
	}
	for _, k := range []string{"YOKE_OUT", "YOKE_ARCH", "YOKE_GOARCH", "YOKE_RUST_TARGET", "YOKE_DEB_ARCH"} {
		if _, there := got[k]; !there {
			t.Errorf("the build was not given %s", k)
		}
		delete(got, k)
	}
	if len(got) != 0 {
		t.Errorf("the composer added %v to the build's environment", got)
	}
}

func environ() map[string]string {
	m := map[string]string{}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

// standIn is a container engine that records its arguments, one per line, and runs on the host the
// command it was handed after the image: what a real engine would run inside it.
func standIn(t *testing.T) (engine, record string) {
	t.Helper()
	at := t.TempDir()
	record = filepath.Join(at, "asked")
	engine = filepath.Join(at, "engine")
	script := `#!/bin/sh
printf '%s\n' "$@" > ` + record + `
while [ $# -gt 0 ]; do
  case "$1" in
    -w) cd "$2"; shift 2 ;;
    -e) export "$2"; shift 2 ;;
    *@sha256:*) shift; exec "$@" ;;
    *) shift ;;
  esac
done
exit 125
`
	os.WriteFile(engine, []byte(script), 0o755)
	return engine, record
}

// std: yoke:building-a-plugin.05
func TestABuildThatNamesAnImageRunsInsideItTheProjectAtItsOwnPathAsTheInvoker(t *testing.T) {
	engine, asked := standIn(t)
	t.Setenv("CONTAINER_ENGINE", engine)
	record := filepath.Join(t.TempDir(), "record")
	file := strings.Replace(withRun(recording(record)), "architectures:", "  container: "+pinned+"\narchitectures:", 1)
	dir := pluginProject(t, file)
	if code, out := run("build", "--dir", dir); code != Done {
		t.Fatalf("build: %d %s", code, out)
	}
	args := lines(t, asked)
	joined := "\n" + strings.Join(args, "\n") + "\n"
	got := lines(t, record)
	out := got[1]
	for _, want := range []string{
		"\nrun\n", "\n--rm\n",
		"\n-v\n" + dir + ":" + dir + "\n",
		"\n-v\n" + out + ":" + out + "\n",
		"\n-w\n" + filepath.Join(dir, "plugin") + "\n",
		"\n--user\n" + strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid()) + "\n",
		"\n-e\nYOKE_OUT=" + out + "\n", "\n-e\nYOKE_ARCH=" + HostArch() + "\n",
		"\n" + pinned + "\n/bin/sh\n-c\n",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("the engine was not asked %q: %v", strings.TrimSpace(want), args)
		}
	}
	if args[len(args)-1] != strings.ReplaceAll(recording(record), "''", "'") {
		t.Errorf("the command handed over is not build.run verbatim: %q", args[len(args)-1])
	}
	for _, k := range []string{"YOKE_GOARCH", "YOKE_RUST_TARGET", "YOKE_DEB_ARCH"} {
		if !strings.Contains(joined, "\n-e\n"+k+"=") {
			t.Errorf("the image's build was not given %s", k)
		}
	}
	exe := filepath.Join(dir, "build", HostArch(), "greeter")
	assertExecutable(t, exe)
	if info, err := os.Stat(exe); err == nil {
		if uid, ok := ownerOf(info); ok && uid != os.Getuid() {
			t.Errorf("the executable belongs to %d, not to the invoker", uid)
		}
	}

	os.Remove(asked)
	os.WriteFile(filepath.Join(dir, PluginFile), []byte(strings.Replace(file, pinned, "registry.example/build/go:1.24", 1)), 0o644)
	code, msg := run("build", "--dir", dir)
	if code == Done || !strings.Contains(msg, "build.container") {
		t.Errorf("an image without a digest is not refused naming build.container: %d %s", code, msg)
	}
	if _, err := os.Stat(asked); err == nil {
		t.Errorf("the engine was asked to run something")
	}
}

func ownerOf(info os.FileInfo) (int, bool) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), true
	}
	return 0, false
}
