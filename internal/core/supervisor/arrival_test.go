package supervisor_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/supervisor"
	"github.com/yoke-project/yoke/internal/core/unit"
)

// std: yoke:what-a-unit-arrives-with.01
func TestTheVariablesAndNothingElse(t *testing.T) {
	t.Setenv("THE_CORES_OWN", "not for a unit")
	root := t.TempDir()
	out := &output{}
	s := supervisor.New(supervisor.Config{
		Root: root, Policy: fast(),
		Incarnations: supervisor.NewCounter(), Tokens: supervisor.NewTokens(), Output: out,
	})
	t.Cleanup(func() { s.Stop() })
	declarations := map[string]map[string]string{}
	for _, id := range []string{"first", "second"} {
		u := declared(t, id, unit.Plugin, "environment")
		u.Env["DECLARED_BY_"+strings.ToUpper(id)] = "yes"
		declarations[id] = u.Env
		s.Launch(u)
	}
	environment := func(id string) map[string]string {
		env := map[string]string{}
		for _, line := range out.all() {
			if rest, ok := strings.CutPrefix(line, id+"#1 env "); ok {
				name, value, _ := strings.Cut(rest, "=")
				env[name] = value
			}
		}
		return env
	}
	// A Plugin unit that exits has failed and is launched again: once a second life has begun, the
	// first has written everything it will.
	over := func(id string) bool {
		st := s.Status(id)
		return st.Incarnation >= 2 || (st.Incarnation == 1 && st.State == unit.Failed)
	}
	until(t, "both units writing their environment", 10*time.Second, func() bool { return over("first") && over("second") })
	for id, other := range map[string]string{"first": "second", "second": "first"} {
		env := environment(id)
		var names []string
		for name := range env {
			names = append(names, name)
		}
		slices.Sort(names)
		want := []string{"YOKE_BIND", "YOKE_PLUGIN", "YOKE_SOCKET", "YOKE_TOKEN", "YOKE_UNIT"}
		for name := range declarations[id] {
			want = append(want, name)
		}
		slices.Sort(want)
		if !slices.Equal(names, want) {
			t.Errorf("%s was handed %v, want %v", id, names, want)
		}
		for _, path := range []string{env["YOKE_SOCKET"], env["YOKE_BIND"]} {
			if rel, err := filepath.Rel(root, path); err != nil || strings.HasPrefix(rel, "..") {
				t.Errorf("%s was handed %q, which is not under the instance root", id, path)
			}
		}
		for name, value := range env {
			lower := strings.ToLower(value)
			if strings.Contains(value, other) || strings.Contains(lower, "incarnation") || strings.Contains(lower, "session") {
				t.Errorf("%s's %s=%q names what it is not given", id, name, value)
			}
		}
	}
}

// std: yoke:what-a-unit-arrives-with.02
func TestTheStartupWindowIsTheUnits(t *testing.T) {
	s, _ := started(t, fast())
	quick := declared(t, "quick", unit.Plugin, "serve")
	own := fast()
	own.StartupWindow = 300 * time.Millisecond
	quick.Policy = &own
	s.Launch(quick)
	s.Launch(declared(t, "patient", unit.Plugin, "serve"))
	time.Sleep(time.Second)
	// A Plugin unit that failed is launched again, so its first life is over once a second has begun.
	if st := s.Status("quick"); st.Incarnation < 2 && st.State != unit.Failed {
		t.Errorf("quick is %v in its incarnation %d after its own window, want its first ended Failed", st.State, st.Incarnation)
	}
	if st := s.Status("patient"); st.State != unit.Starting || st.Incarnation != 1 {
		t.Errorf("patient is %v in its incarnation %d inside the deployment's window, want its first Starting", st.State, st.Incarnation)
	}
}
