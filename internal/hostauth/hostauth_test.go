package hostauth_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/hostauth"
)

// beside writes a Core's executable and, unless body is empty, a program beside it with that body, and
// answers the Core's path.
func beside(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	core := filepath.Join(dir, "yoke-core")
	os.WriteFile(core, []byte("#!/bin/sh\n"), 0o755)
	if body != "" {
		os.WriteFile(filepath.Join(dir, hostauth.Program), []byte("#!/bin/sh\n"+body), 0o755)
	}
	return core
}

// std: yoke:asking-the-host.01
func TestTheProgramBesideTheCoreIsAsked(t *testing.T) {
	record := filepath.Join(t.TempDir(), "asked")
	core := beside(t, `echo "$#" > `+record+`.args; cat > `+record+`; exit $(cat `+record+`.status)`+"\n")
	for status, want := range map[string]hostauth.Verdict{"0": hostauth.Accepted, "1": hostauth.Refused, "2": hostauth.Unasked, "7": hostauth.Unasked} {
		os.WriteFile(record+".status", []byte(status), 0o644)
		got, _ := hostauth.Asker{Core: core}.Ask(context.Background(), "alice", "hunter2x")
		if got != want {
			t.Errorf("an exit %s was read as %v", status, got)
		}
		if in, _ := os.ReadFile(record); string(in) != "alice\x00hunter2x" {
			t.Errorf("the program was given %q", in)
		}
		if args, _ := os.ReadFile(record + ".args"); strings.TrimSpace(string(args)) != "0" {
			t.Errorf("the program was given %s arguments", args)
		}
	}
}

// std: yoke:asking-the-host.02
func TestAProgramNotThereOrNotAnsweringCouldNotBeAsked(t *testing.T) {
	if got, err := (hostauth.Asker{Core: beside(t, "")}).Ask(context.Background(), "alice", "hunter2x"); got != hostauth.Unasked || err == nil {
		t.Errorf("with no program: %v %v", got, err)
	}
	pidfile := filepath.Join(t.TempDir(), "pid")
	core := beside(t, "echo $$ > "+pidfile+"; exec sleep 60\n")
	got, err := hostauth.Asker{Core: core, Wait: 100 * time.Millisecond}.Ask(context.Background(), "alice", "hunter2x")
	if got != hostauth.Unasked || err == nil {
		t.Errorf("a program that never exits: %v %v", got, err)
	}
	pid, _ := os.ReadFile(pidfile)
	if _, err := os.Stat("/proc/" + strings.TrimSpace(string(pid))); err == nil {
		t.Error("the program is still running")
	}
	if hostauth.Wait != 30*time.Second {
		t.Errorf("the wait is %v", hostauth.Wait)
	}
}

// std: yoke:asking-the-host.03
func TestTheProbeSaysWhetherTheServiceCanBeAsked(t *testing.T) {
	if err := hostauth.Probe(beside(t, "")); err == nil {
		t.Error("with no program the probe answered nothing")
	}
	if err := hostauth.Probe(beside(t, `[ "$1" = --probe ] && exit 2; exit 0`+"\n")); err == nil {
		t.Error("a probe exiting 2 answered nothing")
	}
	if err := hostauth.Probe(beside(t, `[ "$1" = --probe ] && exit 0; exit 2`+"\n")); err != nil {
		t.Errorf("a probe exiting 0 answered %v", err)
	}
}
