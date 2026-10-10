package gate_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke-project/yoke/internal/gate"
)

const containerised = "units:\n  probe:\n    kind: oneshot\n    image: localhost/probe@sha256:" + "abababababababababababababababababababababababababababababababab" + "\n"

// engineOf is a host whose engine answers as given, counting how often it is asked.
func engineOf(t *testing.T, facts gate.EngineFacts, err error) (*gate.Host, *int) {
	t.Helper()
	h := host(t)
	asked := 0
	h.Engine = func() (gate.EngineFacts, error) {
		asked++
		return facts, err
	}
	return h, &asked
}

func at(t *testing.T, moment gate.Moment, document string, h *gate.Host) (gate.Report, *gate.Deployment) {
	t.Helper()
	return gate.Check(gate.Input{Document: gate.Document{Path: "bench.yaml", Kind: gate.Composition, Bytes: []byte(document)},
		Moment: moment, Manifests: t.TempDir(), Host: h})
}

// std: yoke:the-engine-at-the-gate.01
func TestWhereAUnitNamesAnImageAnUnreachableEngineIsRefusedAndARootOwnedOneIsWeaker(t *testing.T) {
	h, _ := engineOf(t, gate.EngineFacts{}, errors.New("the engine at unix:///run/none.sock did not answer"))
	r, dep := at(t, gate.Starting, containerised, h)
	if got := codes(r); len(got) != 1 || got[0] != "engine.unreachable" || r.Findings[0].Class != gate.Refusal ||
		!strings.Contains(r.Findings[0].Message, "unix:///run/none.sock") || dep != nil {
		t.Errorf("an unreachable engine gives %v", r.Findings)
	}
	h, _ = engineOf(t, gate.EngineFacts{Rootless: true}, nil)
	if r, dep := at(t, gate.Starting, containerised, h); len(r.Findings) != 0 || dep == nil {
		t.Errorf("a rootless engine gives %v", r.Findings)
	}
	h, _ = engineOf(t, gate.EngineFacts{}, nil)
	r, dep = at(t, gate.Starting, containerised, h)
	if got := codes(r); len(got) != 1 || got[0] != "engine.rootful" || r.Findings[0].Class != gate.Weaker || dep == nil {
		t.Errorf("a root-owned engine gives %v", r.Findings)
	}
}

// std: yoke:the-engine-at-the-gate.02
func TestWhereNoUnitNamesAnImageNothingLooksForAnEngine(t *testing.T) {
	h, asked := engineOf(t, gate.EngineFacts{}, errors.New("asked"))
	if r, _ := at(t, gate.Starting, "units:\n  step: { kind: oneshot, exec: /bin/sh }\n", h); len(r.Findings) != 0 {
		t.Errorf("a deployment on the host gives %v", r.Findings)
	}
	if r, _ := at(t, gate.Composing, containerised, h); len(r.Findings) != 0 {
		t.Errorf("while composing, a containerised deployment gives %v", r.Findings)
	}
	if *asked != 0 {
		t.Errorf("the engine was asked %d times", *asked)
	}
}

// std: yoke:the-engine-at-the-gate.03
func TestAContainerisedUnitsBoundDeviceIsCheckedAgainstWhatItsLaunchMaps(t *testing.T) {
	h, _ := engineOf(t, gate.EngineFacts{Rootless: true}, nil)
	missing := filepath.Join(t.TempDir(), "spectro-head-a")
	r, _ := at(t, gate.Starting, "units:\n  panel:\n    kind: interface\n    image: localhost/panel@sha256:"+strings.Repeat("cd", 32)+
		"\n    needs: [ \"device:head-a\" ]\n    bind: { head-a: "+missing+" }\n", h)
	if got := codes(r); len(got) != 1 || got[0] != "path.missing" || !strings.Contains(r.Findings[0].Message, missing) ||
		r.Findings[0].Location != "units.panel.bind.head-a" {
		t.Errorf("a missing device in a container gives %v", r.Findings)
	}
}
