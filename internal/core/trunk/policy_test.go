package trunk_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/logstore"
	"github.com/yoke-project/yoke/internal/core/trunk"
	"github.com/yoke-project/yoke/internal/gate"
)

func deploymentOf(t *testing.T, document string) *gate.Deployment {
	t.Helper()
	r, dep := gate.Check(gate.Input{Document: gate.Document{Path: "bench.yaml", Kind: gate.Composition, Bytes: []byte(document)}})
	if dep == nil {
		t.Fatalf("the composition is refused: %v", r.Findings)
	}
	return dep
}

func storeOf(t *testing.T) *logstore.Store {
	t.Helper()
	s, err := logstore.Open(filepath.Join(t.TempDir(), "logs.db"), func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

const twoUnits = `
policy: { retention: { entries: 50 } }
units:
  a: { kind: oneshot, exec: /bin/true, policy: { retention: { age: 1h } } }
  b: { kind: oneshot, exec: /bin/true }
`

var week = 7 * 24 * time.Hour

// std: yoke:a-groups-policy.01
func TestAUnitsGroupTakesItsInnerScopeAndEveryOtherGroupTheDefault(t *testing.T) {
	dep := deploymentOf(t, twoUnits)
	resolve := trunk.RetentionOf(storeOf(t), func() *gate.Deployment { return dep })
	for group, want := range map[string]logstore.Limits{
		"a":                {Age: time.Hour, Bytes: 50_000_000, Entries: 50},
		"b":                {Age: week, Bytes: 50_000_000, Entries: 50},
		logstore.CoreGroup: {Age: week, Bytes: 50_000_000, Entries: 50},
		"renamed":          {Age: week, Bytes: 50_000_000, Entries: 50},
	} {
		if got := resolve(group); got != want {
			t.Errorf("%s resolves to %+v, want %+v", group, got, want)
		}
	}
}

// std: yoke:a-groups-policy.02
func TestAnExplicitZeroConstrainsNothingAndAnAbsentFieldTakesTheDefault(t *testing.T) {
	dep := deploymentOf(t, "policy: { retention: { bytes: 0 } }\nunits:\n  a: { kind: oneshot, exec: /bin/true }\n")
	got := trunk.RetentionOf(storeOf(t), func() *gate.Deployment { return dep })("a")
	if want := (logstore.Limits{Age: week, Bytes: 0, Entries: 100_000}); got != want {
		t.Errorf("a resolves to %+v, want %+v", got, want)
	}
}

// std: yoke:a-groups-policy.03
func TestAnOverrideReplacesWhatTheDescriptionResolvedToAndIsReadLive(t *testing.T) {
	dep := deploymentOf(t, twoUnits)
	logs := storeOf(t)
	resolve := trunk.RetentionOf(logs, func() *gate.Deployment { return dep })
	ten := uint64(10)
	if err := logs.SetOverride("a", logstore.Retention{Entries: &ten}); err != nil {
		t.Fatal(err)
	}
	if got, want := resolve("a"), (logstore.Limits{Entries: 10}); got != want {
		t.Errorf("with the override, a resolves to %+v, want %+v", got, want)
	}
	if err := logs.ClearOverride("a"); err != nil {
		t.Fatal(err)
	}
	if got, want := resolve("a"), (logstore.Limits{Age: time.Hour, Bytes: 50_000_000, Entries: 50}); got != want {
		t.Errorf("once cleared, a resolves to %+v, want %+v", got, want)
	}
}
