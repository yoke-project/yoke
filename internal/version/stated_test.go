package version

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// std: yoke:the-version-stated.01
func TestAReleasesVersionIsStatedAsItsNumber(t *testing.T) {
	if got := Line("yoke-core", "v0.12.0"); got != "yoke-core 0.12.0" {
		t.Errorf("the line is %q", got)
	}
}

// std: yoke:the-version-stated.02
func TestABuildThatIsNoReleaseSaysSo(t *testing.T) {
	for _, stated := range []string{"v0.12.1-0.20261106100000-0123456789ab", "v0.12.0+dirty", "(devel)", ""} {
		got := Line("yoke", stated)
		want := strings.TrimPrefix(stated, "v")
		if want == "" {
			want = "(devel)"
		}
		if !strings.HasPrefix(got, "yoke "+want+" ") || !strings.HasSuffix(got, "no release") {
			t.Errorf("%q gives %q", stated, got)
		}
	}
}

// std: yoke:the-version-stated.03
func TestEveryProgramAnswersVersionAndDoesNothingElse(t *testing.T) {
	if testing.Short() {
		t.Skip("builds six programs")
	}
	root, _ := filepath.Abs("../..")
	bin := t.TempDir()
	home := t.TempDir()
	for _, program := range []string{"yoke", "yoke-core", "yoke-guardian", "yoke-auth", "yoke-compose", "yoke-conformance"} {
		path := filepath.Join(bin, program)
		build := exec.Command("go", "build", "-o", path, "./cmd/"+program)
		build.Dir = root
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, out)
		}
		run := exec.Command(path, "--version")
		run.Dir = home
		run.Env = []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
		out, err := run.CombinedOutput()
		if err != nil {
			t.Errorf("%s --version: %v\n%s", program, err, out)
			continue
		}
		lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
		if len(lines) != 1 || !strings.HasPrefix(lines[0], program+" ") {
			t.Errorf("%s --version printed %q", program, out)
		}
		if entries, _ := os.ReadDir(home); len(entries) != 0 {
			t.Errorf("%s --version wrote into its home: %v", program, entries)
		}
	}
}
