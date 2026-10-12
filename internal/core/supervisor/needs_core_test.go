package supervisor_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
)

// std: yoke:what-a-need-expands-into.06
func TestUnderPodmanAUnitUsesItsNeeds(t *testing.T) {
	image := fixtureImage(t, "needs")
	socket := enginetest.Serve(t).Socket
	unit := func(name, needs string) string {
		return "  " + name + ":\n    kind: oneshot\n    image: " + image + "\n    needs: [ \"storage:data\", \"device:sink\"" + needs + " ]\n" +
			"    bind: { sink: /dev/null }\n    args: [ \"${bind.data}\", \"${bind.sink}\" ]\n"
	}
	d := deployed(t, socket, "units:\n"+unit("closed", "")+unit("open", ", network"))
	core, lines := d.start(t)
	r := &reader{t: t, lines: lines}
	said := map[string]string{}
	completed := map[string]bool{}
	r.until("both units to complete", 2*time.Minute, func(line string) bool {
		for _, name := range []string{"closed", "open"} {
			if strings.Contains(line, "unit="+name+" incarnation=1 stream=stdout") {
				said[name] = line
			}
			if strings.Contains(line, "subject=unit:"+name+"#1") && strings.Contains(line, "to=Completed") {
				completed[name] = true
			}
		}
		return completed["closed"] && completed["open"]
	})
	core.Process.Signal(syscall.SIGTERM)
	r.rest()
	core.Wait()

	if !strings.Contains(said["closed"], "wrote opened interfaces=0") {
		t.Errorf("the unit with no network said %q", said["closed"])
	}
	if !strings.Contains(said["open"], "wrote opened interfaces=") || strings.Contains(said["open"], "interfaces=0") {
		t.Errorf("the unit with a network said %q", said["open"])
	}
	data := filepath.Join(filepath.Dir(d.config), "state", "storage", "data")
	for _, name := range []string{"closed", "open"} {
		info, err := os.Stat(filepath.Join(data, "written-"+name))
		if err != nil {
			t.Errorf("the host does not find what %s wrote: %v", name, err)
			continue
		}
		if uid := info.Sys().(*syscall.Stat_t).Uid; strconv.Itoa(int(uid)) != strconv.Itoa(os.Getuid()) {
			t.Errorf("what %s wrote is owned by %d", name, uid)
		}
	}
}

// std: yoke:what-a-need-expands-into.09
func TestUnderTheEngineAUnitReadsItsSecretFromItsPath(t *testing.T) {
	image := fixtureImage(t, "secret")
	socket := enginetest.Serve(t).Socket
	self, _ := os.Executable()
	// The role is set by a wrapper: a declaration may not write into the prefix the Core hands a process.
	wrapper := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(wrapper, []byte("#!/bin/sh\nexport "+role+"=secret\nexec "+self+" \"$@\"\n"), 0o755)
	d := deployed(t, socket, "units:\n"+
		"  host:\n    kind: oneshot\n    exec: "+wrapper+"\n    needs: [ \"secret:token\" ]\n    args: [ \"${bind.token}\" ]\n"+
		"  contained:\n    kind: oneshot\n    image: "+image+"\n    needs: [ \"secret:token\" ]\n    args: [ \"${bind.token}\" ]\n")
	secrets := filepath.Join(filepath.Dir(d.config), "state", "secrets")
	os.MkdirAll(secrets, 0o700)
	os.WriteFile(filepath.Join(secrets, "token"), []byte("s3cr3t-value"), 0o600)
	core, lines := d.start(t)
	r := &reader{t: t, lines: lines}
	said := map[string]string{}
	completed := map[string]bool{}
	r.until("both units to complete", 2*time.Minute, func(line string) bool {
		for _, name := range []string{"host", "contained"} {
			if strings.Contains(line, "unit="+name+" incarnation=1 stream=stdout") {
				said[name] = line
			}
			if strings.Contains(line, "subject=unit:"+name+"#1") && strings.Contains(line, "to=Completed") {
				completed[name] = true
			}
		}
		return completed["host"] && completed["contained"]
	})
	core.Process.Signal(syscall.SIGTERM)
	r.rest()
	core.Wait()
	for _, name := range []string{"host", "contained"} {
		if !strings.Contains(said[name], "read 12 bytes clean=true") {
			t.Errorf("%s said %q", name, said[name])
		}
	}
}
