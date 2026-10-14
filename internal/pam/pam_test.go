package pam_test

import (
	"bytes"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/pam"
)

// stack writes a directory of stacks whose `yoke` accepts alice with secret, asking the secret by the
// conversation and handing it to a script, and answers the directory.
func stack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	check := filepath.Join(dir, "check.sh")
	os.WriteFile(check, []byte("#!/bin/sh\nread -r secret\n[ \"$PAM_USER\" = alice ] && [ \"$secret\" = hunter2x ]\n"), 0o755)
	stacks := filepath.Join(dir, "pam.d")
	os.MkdirAll(stacks, 0o755)
	os.WriteFile(filepath.Join(stacks, "yoke"), []byte("auth required pam_exec.so expose_authtok quiet "+check+"\naccount required pam_permit.so\n"), 0o644)
	return stacks
}

func run(stacks, input string, args ...string) (int, string) {
	var out, errs bytes.Buffer
	code := pam.Run(args, strings.NewReader(input), &out, &errs, func(k string) string {
		if k == pam.StacksVariable {
			return stacks
		}
		return ""
	})
	return code, out.String() + "\x00" + errs.String()
}

// std: yoke:the-authentication-program.01
func TestTheRightSecretIsAcceptedAndAnythingElseRefused(t *testing.T) {
	stacks := stack(t)
	for input, want := range map[string]int{
		"alice\x00hunter2x": 0,
		"alice\x00wrong":    1,
		"bob\x00hunter2x":   1,
		"alice hunter2x":    1,
		"\x00hunter2x":      1,
	} {
		code, said := run(stacks, input)
		if code != want {
			t.Errorf("%q exited %d: %s", input, code, said)
		}
		if out, _, _ := strings.Cut(said, "\x00"); out != "" {
			t.Errorf("%q wrote %q", input, out)
		}
		if strings.Contains(said, "hunter2x") {
			t.Errorf("%q: the secret appears in %q", input, said)
		}
	}
}

// std: yoke:the-authentication-program.02
func TestAServiceThatCannotBeAskedIsSaidApart(t *testing.T) {
	empty := t.TempDir()
	if code, said := run(empty, "alice\x00hunter2x"); code != 2 {
		t.Errorf("against no stack: %d %s", code, said)
	}
	if code, said := run(empty, "", "--probe"); code != 2 {
		t.Errorf("the probe against no stack: %d %s", code, said)
	}
	if code, said := run(stack(t), "", "--probe"); code != 0 {
		t.Errorf("the probe against the stack: %d %s", code, said)
	}
}

// std: yoke:the-authentication-program.03
func TestOnlyTheAuthenticationProgramNamesAnInterpreter(t *testing.T) {
	bin := t.TempDir()
	for program, dynamic := range map[string]bool{"yoke-core": false, "yoke": false, "yoke-guardian": false, "yoke-auth": true} {
		path := filepath.Join(bin, program)
		build := exec.Command("go", "build", "-o", path, "github.com/yoke-project/yoke/cmd/"+program)
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if said, err := build.CombinedOutput(); err != nil {
			t.Fatalf("%s does not build: %v\n%s", program, err, said)
		}
		f, err := elf.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		interpreted := false
		for _, p := range f.Progs {
			interpreted = interpreted || p.Type == elf.PT_INTERP
		}
		f.Close()
		if interpreted != dynamic {
			t.Errorf("%s names an interpreter: %v", program, interpreted)
		}
	}
}
