package interfaces_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/engine/enginetest"
)

// containerised starts the Core with the environment's engine, and a composition whose managed interface
// panel-ui runs this test binary, attaching to its channel, from an image; it returns the runtime
// directory and the Core's lines.
func containerised(t *testing.T, channels string) (string, <-chan string) {
	t.Helper()
	dir := t.TempDir()
	build := exec.Command("go", "test", "-c", "-o", filepath.Join(dir, "unit"), ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if said, err := build.CombinedOutput(); err != nil {
		t.Fatalf("the fixture does not build: %v\n%s", err, said)
	}
	os.WriteFile(filepath.Join(dir, "Containerfile"), []byte("FROM scratch\nCOPY unit /unit\nENV "+role+"=attach\nENTRYPOINT [\"/unit\"]\n"), 0o644)
	image := enginetest.Image(t, dir, "yoke-l3-attach")
	socket := enginetest.Serve(t).Socket
	t.Cleanup(func() { enginetest.Remove(enginetest.Labelled("dev.yoke-project.unit=panel-ui")...) })
	// The %s placeholders startCore fills are taken by a comment: this composition runs no host program.
	return startCoreWith(t, "engine: unix://"+socket+"\n", "# %s %s\nunits:\n  panel-ui: { kind: interface, image: "+image+" }\n"+channels)
}

// lineWith reads the Core's lines until one holds every part, within the time given.
func lineWith(t *testing.T, lines <-chan string, said *[]string, within time.Duration, parts ...string) string {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case line, open := <-lines:
			if !open {
				t.Fatalf("the Core exited waiting for %q:\n%s", parts, strings.Join(*said, "\n"))
			}
			*said = append(*said, line)
			holds := true
			for _, p := range parts {
				holds = holds && strings.Contains(line, p)
			}
			if holds {
				return line
			}
		case <-deadline:
			t.Fatalf("no line with %q within %v:\n%s", parts, within, strings.Join(*said, "\n"))
		}
	}
}

// std: yoke:a-managed-interface-in-a-container.01
func TestFromItsContainerAManagedInterfaceAttachesAndBindsNothing(t *testing.T) {
	_, lines := containerised(t, "channels:\n  front: { unit: panel-ui, transport: local, clients: single }\n")
	var said []string
	lineWith(t, lines, &said, 2*time.Minute, "type=channel.attached", "channel:front")
	ids := enginetest.Labelled("dev.yoke-project.unit=panel-ui")
	if len(ids) != 1 {
		t.Fatalf("the interface runs in %v", ids)
	}
	network, _ := enginetest.CLI("inspect", "-f", "{{.HostConfig.NetworkMode}}", ids[0]).Output()
	ports, _ := enginetest.CLI("inspect", "-f", "{{len .HostConfig.PortBindings}}", ids[0]).Output()
	if strings.TrimSpace(string(network)) != "none" || strings.TrimSpace(string(ports)) != "0" {
		t.Errorf("the interface's container has the network %q and %s port bindings", network, ports)
	}
}

// std: yoke:a-managed-interface-in-a-container.02
func TestWhenItsContainerStopsItsChannelIsDetachedAndArbitrationRecomputed(t *testing.T) {
	run, lines := containerised(t, "channels:\n  front: { unit: panel-ui, transport: local, clients: single }\n"+
		"  bench: { transport: local, clients: single }\narbitration:\n  - { prevails: front, over: [ bench ] }\n")
	var said []string
	lineWith(t, lines, &said, 2*time.Minute, "type=channel.attached", "channel:front")
	bench, err := attachTo(t, filepath.Join(run, "interfaces", "bench.sock"))
	if err != nil {
		t.Fatal(err)
	}
	bench.next(t)
	lineWith(t, lines, &said, 10*time.Second, "type=channel.suspended", "channel:bench")
	ids := enginetest.Labelled("dev.yoke-project.unit=panel-ui")
	if len(ids) != 1 {
		t.Fatalf("the interface runs in %v", ids)
	}
	if out, err := enginetest.CLI("kill", ids[0]).CombinedOutput(); err != nil {
		t.Fatalf("the container could not be stopped: %v %s", err, out)
	}
	lineWith(t, lines, &said, 30*time.Second, "type=channel.detached", "channel:front")
	lineWith(t, lines, &said, 10*time.Second, "type=channel.resumed", "channel:bench")
}
