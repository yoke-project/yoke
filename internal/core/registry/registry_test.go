package registry

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func fresh(t *testing.T) (*Registry, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), File)
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r, path
}

func declared(t *testing.T, r *Registry, id string) {
	t.Helper()
	if err := r.Declare(Declared{ID: id, Protocol: 1, ManifestDigest: "sha256:aa"}); err != nil {
		t.Fatal(err)
	}
}

func plugin(t *testing.T, r *Registry, id string) Plugin {
	t.Helper()
	p, found, err := r.Plugin(id)
	if err != nil || !found {
		t.Fatalf("the plugin %s: found=%v err=%v", id, found, err)
	}
	return p
}

func history(t *testing.T, r *Registry, id string) []Decision {
	t.Helper()
	h, err := r.History(id)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// header reads the schema number from the file's header, through a connection of its own.
func header(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open(driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("PRAGMA user_version").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// std: yoke:the-registry.02
func TestOpenedWithTheRegistrysFourSettings(t *testing.T) {
	r, _ := fresh(t)
	for pragma, want := range map[string]string{"journal_mode": "wal", "synchronous": "2", "foreign_keys": "1", "busy_timeout": "5000"} {
		var got string
		if err := r.db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s is %q, want %q", pragma, got, want)
		}
	}
}

// std: yoke:the-registry.03
func TestANewFileIsCreatedAtTheSchemaThisCoreImplements(t *testing.T) {
	r, path := fresh(t)
	r.Close()
	if got := header(t, path); got != len(steps) {
		t.Fatalf("the header says %d, and this Core carries %d steps", got, len(steps))
	}
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
	if got := header(t, path); got != len(steps) {
		t.Fatalf("a second open moved the header to %d", got)
	}
}

// std: yoke:the-registry.04
func TestALowerNumberHasTheMissingStepsApplied(t *testing.T) {
	three := []string{
		"CREATE TABLE one (a INTEGER)",
		"CREATE TABLE two (b INTEGER)",
		"CREATE TABLE three (c INTEGER)",
	}
	path := filepath.Join(t.TempDir(), File)
	r, err := open(path, three[:1])
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	if r, err = open(path, three); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"one", "two", "three"} {
		if _, err := r.db.Exec("SELECT * FROM " + table); err != nil {
			t.Errorf("the table %s is missing: %v", table, err)
		}
	}
	r.Close()
	if got := header(t, path); got != 3 {
		t.Fatalf("the header says %d, want 3", got)
	}

	failing := filepath.Join(t.TempDir(), File)
	if r, err = open(failing, three[:1]); err != nil {
		t.Fatal(err)
	}
	r.Close()
	broken := []string{three[0], "CREATE TABLE half (d INTEGER); CREATE TABLE nonsense (", three[2]}
	if _, err := open(failing, broken); err == nil {
		t.Fatal("a failing step did not refuse the open")
	}
	if got := header(t, failing); got != 1 {
		t.Fatalf("after a failed step the header says %d, want 1", got)
	}
	db, _ := sql.Open(driver, failing)
	defer db.Close()
	if _, err := db.Exec("SELECT * FROM half"); err == nil {
		t.Fatal("the failed step left part of itself behind")
	}
}

// std: yoke:the-registry.06
func TestFiveTablesRootedOnThePlugin(t *testing.T) {
	r, _ := fresh(t)
	want := map[string][]string{
		"plugin":     {"id", "protocol", "manifest_digest", "version", "language", "sdk"},
		"policy":     {"plugin_id", "enabled"},
		"grant":      {"plugin_id", "capability"},
		"credential": {"plugin_id", "mode", "fingerprint"},
		"decision":   {"seq", "plugin_id", "at", "actor", "action", "capability"},
	}
	rows, err := r.db.Query("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}
	rows.Close()
	slices.Sort(tables)
	if names := slices.Sorted(func(yield func(string) bool) {
		for k := range want {
			if !yield(k) {
				return
			}
		}
	}); !slices.Equal(tables, names) {
		t.Fatalf("the tables are %v, want %v", tables, names)
	}
	for table, columns := range want {
		rows, err := r.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for rows.Next() {
			var name string
			rows.Scan(&name)
			got = append(got, name)
		}
		rows.Close()
		if !slices.Equal(got, columns) {
			t.Errorf("%s has %v, want %v", table, got, columns)
		}
	}
}

// std: yoke:the-registry.07
func TestARowWithNoPluginFailsWhereItIsWritten(t *testing.T) {
	r, _ := fresh(t)
	if _, err := r.Grant("com.example.nobody", "stream.x.publish", "davide"); err == nil {
		t.Error("a grant for a plugin never declared was written")
	}
	if _, err := r.Enable("com.example.nobody", "davide"); err == nil {
		t.Error("enabling a plugin never declared succeeded")
	}
	for _, table := range []string{"policy", "grant", "decision"} {
		var n int
		r.db.QueryRow(`SELECT count(*) FROM "` + table + `"`).Scan(&n)
		if n != 0 {
			t.Errorf("%s holds %d rows", table, n)
		}
	}
}

// std: yoke:the-registry.08
func TestDeclaringAPluginGrantsNothing(t *testing.T) {
	r, _ := fresh(t)
	if err := r.Declare(Declared{ID: "com.yoke.station.acquire", Protocol: 1, ManifestDigest: "sha256:c3d9"}); err != nil {
		t.Fatal(err)
	}
	want := Plugin{ID: "com.yoke.station.acquire", Protocol: 1, ManifestDigest: "sha256:c3d9", Enabled: true, Mode: "bootstrap"}
	if got := plugin(t, r, "com.yoke.station.acquire"); !reflect.DeepEqual(got, want) {
		t.Fatalf("declared as %+v, want %+v", got, want)
	}
	if h := history(t, r, "com.yoke.station.acquire"); len(h) != 0 {
		t.Fatalf("declaring took decisions: %+v", h)
	}
}

// std: yoke:the-registry.09
func TestDeclaringAgainTouchesNothingDecided(t *testing.T) {
	r, _ := fresh(t)
	declared(t, r, "com.example.p")
	r.Disable("com.example.p", "davide")
	r.Grant("com.example.p", "stream.x.publish", "davide")
	if err := r.Declare(Declared{ID: "com.example.p", Protocol: 2, ManifestDigest: "sha256:bb"}); err != nil {
		t.Fatal(err)
	}
	got := plugin(t, r, "com.example.p")
	if got.Protocol != 2 || got.ManifestDigest != "sha256:bb" {
		t.Errorf("the declaration was not updated: %+v", got)
	}
	if got.Enabled || !slices.Equal(got.Grants, []string{"stream.x.publish"}) {
		t.Errorf("declaring again touched the policy: %+v", got)
	}
	if h := history(t, r, "com.example.p"); len(h) != 2 {
		t.Errorf("the history holds %d decisions, want 2", len(h))
	}
}

// std: yoke:the-registry.10
func TestARegistrationRecordsVersionLanguageAndSDK(t *testing.T) {
	r, _ := fresh(t)
	declared(t, r, "com.example.p")
	if err := r.Record("com.example.p", Registration{Version: "2.1.0", Language: "go", SDK: "yoke-sdk-go 1.4.2"}); err != nil {
		t.Fatal(err)
	}
	want := Plugin{ID: "com.example.p", Protocol: 1, ManifestDigest: "sha256:aa", Version: "2.1.0", Language: "go", SDK: "yoke-sdk-go 1.4.2", Enabled: true, Mode: "bootstrap"}
	if got := plugin(t, r, "com.example.p"); !reflect.DeepEqual(got, want) {
		t.Fatalf("recorded as %+v, want %+v", got, want)
	}
	if h := history(t, r, "com.example.p"); len(h) != 0 {
		t.Fatalf("a registration took decisions: %+v", h)
	}
}

// std: yoke:the-registry.11
func TestEachChangeOfAuthorityWritesOneDecision(t *testing.T) {
	r, _ := fresh(t)
	declared(t, r, "com.example.p")
	instant := time.Date(2026, 8, 16, 10, 4, 52, 0, time.UTC)
	r.now = func() time.Time { return instant }
	steps := []struct {
		act  func() (bool, error)
		want Decision
	}{
		{func() (bool, error) { return r.Disable("com.example.p", "davide") }, Decision{Action: "disabled"}},
		{func() (bool, error) { return r.Grant("com.example.p", "stream.x.publish", "davide") }, Decision{Action: "granted", Capability: "stream.x.publish"}},
		{func() (bool, error) { return r.Withdraw("com.example.p", "stream.x.publish", "davide") }, Decision{Action: "withdrawn", Capability: "stream.x.publish"}},
		{func() (bool, error) { return r.Enable("com.example.p", "davide") }, Decision{Action: "enabled"}},
	}
	for i, step := range steps {
		changed, err := step.act()
		if err != nil || !changed {
			t.Fatalf("step %d: changed=%v err=%v", i, changed, err)
		}
		if i == 1 && !slices.Equal(plugin(t, r, "com.example.p").Grants, []string{"stream.x.publish"}) {
			t.Fatal("a grant is not a row while it holds")
		}
	}
	if grants := plugin(t, r, "com.example.p").Grants; len(grants) != 0 {
		t.Fatalf("a withdrawn grant is still there: %v", grants)
	}
	h := history(t, r, "com.example.p")
	if len(h) != len(steps) {
		t.Fatalf("%d decisions, want %d", len(h), len(steps))
	}
	for i, d := range h {
		want := steps[i].want
		if d.Action != want.Action || d.Capability != want.Capability || d.Actor != "davide" || !d.At.Equal(instant) || d.Plugin != "com.example.p" {
			t.Errorf("decision %d is %+v, want %s %q by davide at %s", i, d, want.Action, want.Capability, instant)
		}
		if i > 0 && d.Seq <= h[i-1].Seq {
			t.Errorf("decision %d is out of order", i)
		}
	}
}

// std: yoke:the-registry.12
func TestAnOperationAlreadyTrueRecordsNothing(t *testing.T) {
	r, _ := fresh(t)
	declared(t, r, "com.example.p")
	r.Grant("com.example.p", "stream.x.publish", "davide")
	before := history(t, r, "com.example.p")
	for name, act := range map[string]func() (bool, error){
		"enable":   func() (bool, error) { return r.Enable("com.example.p", "davide") },
		"grant":    func() (bool, error) { return r.Grant("com.example.p", "stream.x.publish", "davide") },
		"withdraw": func() (bool, error) { return r.Withdraw("com.example.p", "command.y.accept", "davide") },
	} {
		changed, err := act()
		if err != nil || changed {
			t.Errorf("%s: changed=%v err=%v, want an unchanged success", name, changed, err)
		}
	}
	if after := history(t, r, "com.example.p"); len(after) != len(before) {
		t.Fatalf("the history grew from %d to %d", len(before), len(after))
	}
}

// std: yoke:the-registry.13
func TestNothingRemovesAPlugin(t *testing.T) {
	r, path := fresh(t)
	declared(t, r, "com.example.gone")
	r.Grant("com.example.gone", "stream.x.publish", "davide")
	r.Withdraw("com.example.gone", "stream.x.publish", "davide")
	r.Disable("com.example.gone", "davide")
	r.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	got := plugin(t, again, "com.example.gone")
	if got.Enabled || len(got.Grants) != 0 || got.Mode != "bootstrap" {
		t.Errorf("the record after disabling and withdrawing is %+v", got)
	}
	if h := history(t, again, "com.example.gone"); len(h) != 3 {
		t.Errorf("the history holds %d decisions, want 3", len(h))
	}
	methods := reflect.TypeOf(again)
	for i := range methods.NumMethod() {
		name := methods.Method(i).Name
		for _, verb := range []string{"Remove", "Delete", "Forget", "Drop", "Purge"} {
			if strings.HasPrefix(name, verb) {
				t.Errorf("the Registry offers %s", name)
			}
		}
	}
}
