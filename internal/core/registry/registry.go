// Package registry is the store of authority: what a Manifest and a registration declared about a
// plugin, what an operator authorised, and the history of who decided it.
//
// Every table is rooted on the plugin and nothing is keyed by a unit. Nothing observed enters — current
// state, the Session, a count, a time something was last seen — because a stored claim about now is
// the claim that goes stale when it matters. Nothing removes a plugin: disabling and withdrawing
// reduce its record to what was once decided, and the account of who decided it survives.
package registry

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	_ "modernc.org/sqlite"
)

// File is the Registry's name in the instance's state directory.
const File = "registry.db"

const driver = "sqlite"

// steps lead to the schema this Core implements; a file's number is how many it has had applied.
var steps = []string{
	`CREATE TABLE plugin (
		id              TEXT PRIMARY KEY,
		protocol        INTEGER NOT NULL,
		manifest_digest TEXT NOT NULL,
		version         TEXT NOT NULL DEFAULT '',
		language        TEXT NOT NULL DEFAULT '',
		sdk             TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE policy (
		plugin_id TEXT PRIMARY KEY REFERENCES plugin (id),
		enabled   INTEGER NOT NULL
	);
	CREATE TABLE "grant" (
		plugin_id  TEXT NOT NULL REFERENCES policy (plugin_id),
		capability TEXT NOT NULL,
		PRIMARY KEY (plugin_id, capability)
	);
	CREATE TABLE credential (
		plugin_id   TEXT PRIMARY KEY REFERENCES plugin (id),
		mode        TEXT NOT NULL,
		fingerprint TEXT
	);
	CREATE TABLE decision (
		seq        INTEGER PRIMARY KEY,
		plugin_id  TEXT NOT NULL REFERENCES plugin (id),
		at         TEXT NOT NULL,
		actor      TEXT NOT NULL,
		action     TEXT NOT NULL CHECK (action IN ('enabled', 'disabled', 'granted', 'withdrawn')),
		capability TEXT
	);`,
}

// Schema is the number of the schema this Core implements.
func Schema() int { return len(steps) }

// Registry is one instance's store of authority. The Core is its one writer.
type Registry struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens the Registry at path, creating it if absent, and migrates it forward to the schema this
// Core implements. A file written by a newer Core is refused, naming both numbers, and left untouched.
func Open(path string) (*Registry, error) { return open(path, steps) }

func open(path string, steps []string) (*Registry, error) {
	// The number is read before anything is set: setting the journal mode rewrites the header, and a
	// file this Core refuses is left exactly as it was.
	if err := readable(path, len(steps)); err != nil {
		return nil, err
	}
	// A decision an operator made must survive a power cut, and a reader never blocks the writer.
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)"
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db, path, steps); err != nil {
		db.Close()
		return nil, err
	}
	return &Registry{db: db, now: time.Now}, nil
}

// migrate applies the steps the file has not had, each in one transaction that also advances the
// number, so a store is at one number or the next and never between them.
func migrate(db *sql.DB, path string, steps []string) error {
	var at int
	if err := db.QueryRow("PRAGMA user_version").Scan(&at); err != nil {
		return fmt.Errorf("the Registry %s cannot be read: %w", path, err)
	}
	if at > len(steps) {
		return newer(path, at, len(steps))
	}
	for n := at; n < len(steps); n++ {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(steps[n]); err != nil {
			tx.Rollback()
			return fmt.Errorf("the Registry %s: step %d: %w", path, n+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", n+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// readable refuses a file already at a number above implements, reading it without writing.
func readable(path string, implements int) error {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	db, err := sql.Open(driver, "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var at int
	if err := db.QueryRow("PRAGMA user_version").Scan(&at); err != nil {
		return fmt.Errorf("the Registry %s cannot be read: %w", path, err)
	}
	if at > implements {
		return newer(path, at, implements)
	}
	return nil
}

func newer(path string, at, implements int) error {
	return fmt.Errorf("the Registry %s is at schema %d, and this Core implements %d: a store written by a newer Core is not read", path, at, implements)
}

// Close closes the store.
func (r *Registry) Close() error { return r.db.Close() }

// Declared is what discovery writes from a Manifest.
type Declared struct {
	ID             string
	Protocol       int
	ManifestDigest string
}

// Declare writes a plugin's declared identity. A plugin declared for the first time is enabled, holds no
// grant and authenticates by the bootstrap token, and none of that is a decision; declaring it again
// updates what was declared and touches nothing an operator decided.
func (r *Registry) Declare(d Declared) error {
	return r.within(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO plugin (id, protocol, manifest_digest) VALUES (?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET protocol = excluded.protocol, manifest_digest = excluded.manifest_digest`,
			d.ID, d.Protocol, d.ManifestDigest); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO policy (plugin_id, enabled) VALUES (?, 1) ON CONFLICT DO NOTHING`, d.ID); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO credential (plugin_id, mode) VALUES (?, 'bootstrap') ON CONFLICT DO NOTHING`, d.ID)
		return err
	})
}

// Registration is what only a process can say about itself, recorded at admission and weighed by nothing.
type Registration struct {
	Version  string
	Language string
	SDK      string
}

// Record writes what a registration said about the artifact.
func (r *Registry) Record(id string, reg Registration) error {
	result, err := r.db.Exec(`UPDATE plugin SET version = ?, language = ?, sdk = ? WHERE id = ?`, reg.Version, reg.Language, reg.SDK, id)
	if err != nil {
		return err
	}
	return known(result, id)
}

// Plugin is everything the Registry holds about one plugin: what was declared, and what is authorised.
type Plugin struct {
	ID             string
	Protocol       int
	ManifestDigest string
	Version        string
	Language       string
	SDK            string

	Enabled     bool
	Grants      []string // capability names, in order
	Mode        string
	Fingerprint string
}

// Plugin reads one plugin.
func (r *Registry) Plugin(id string) (Plugin, bool, error) {
	p := Plugin{ID: id}
	var fingerprint sql.NullString
	err := r.db.QueryRow(`SELECT p.protocol, p.manifest_digest, p.version, p.language, p.sdk, pol.enabled, c.mode, c.fingerprint
		FROM plugin p JOIN policy pol ON pol.plugin_id = p.id JOIN credential c ON c.plugin_id = p.id WHERE p.id = ?`, id).
		Scan(&p.Protocol, &p.ManifestDigest, &p.Version, &p.Language, &p.SDK, &p.Enabled, &p.Mode, &fingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return Plugin{}, false, nil
	}
	if err != nil {
		return Plugin{}, false, err
	}
	p.Fingerprint = fingerprint.String
	rows, err := r.db.Query(`SELECT capability FROM "grant" WHERE plugin_id = ? ORDER BY capability`, id)
	if err != nil {
		return Plugin{}, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var capability string
		if err := rows.Scan(&capability); err != nil {
			return Plugin{}, false, err
		}
		p.Grants = append(p.Grants, capability)
	}
	return p, true, rows.Err()
}

// Plugins lists every plugin the Registry holds, whether or not anything declares it any more.
func (r *Registry) Plugins() ([]string, error) {
	rows, err := r.db.Query(`SELECT id FROM plugin ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Decision is one row of the history: who changed a plugin's authority, how, and when.
type Decision struct {
	Seq        int64
	Plugin     string
	At         time.Time
	Actor      string
	Action     string // enabled, disabled, granted, withdrawn
	Capability string // for granted and withdrawn
}

// History reads the decisions taken about one plugin, in the order they were taken.
func (r *Registry) History(id string) ([]Decision, error) {
	rows, err := r.db.Query(`SELECT seq, at, actor, action, capability FROM decision WHERE plugin_id = ? ORDER BY seq`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Decision
	for rows.Next() {
		d := Decision{Plugin: id}
		var at string
		var capability sql.NullString
		if err := rows.Scan(&d.Seq, &at, &d.Actor, &d.Action, &capability); err != nil {
			return nil, err
		}
		if d.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, err
		}
		d.Capability = capability.String
		out = append(out, d)
	}
	return out, rows.Err()
}

// Enable lets a plugin run. It reports whether anything changed; an effect already true records nothing.
func (r *Registry) Enable(id, actor string) (bool, error) { return r.setEnabled(id, actor, true) }

// Disable stops a plugin running at all, whatever it is granted.
func (r *Registry) Disable(id, actor string) (bool, error) { return r.setEnabled(id, actor, false) }

func (r *Registry) setEnabled(id, actor string, enabled bool) (bool, error) {
	action := map[bool]string{true: "enabled", false: "disabled"}[enabled]
	changed := false
	err := r.within(func(tx *sql.Tx) error {
		var now bool
		if err := tx.QueryRow(`SELECT enabled FROM policy WHERE plugin_id = ?`, id).Scan(&now); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("no plugin %s is declared", id)
			}
			return err
		}
		if now == enabled {
			return nil
		}
		if _, err := tx.Exec(`UPDATE policy SET enabled = ? WHERE plugin_id = ?`, enabled, id); err != nil {
			return err
		}
		changed = true
		return r.decided(tx, id, actor, action, "")
	})
	return changed, err
}

// Grant authorises a capability. A grant is a row.
func (r *Registry) Grant(id, capability, actor string) (bool, error) {
	changed := false
	err := r.within(func(tx *sql.Tx) error {
		result, err := tx.Exec(`INSERT INTO "grant" (plugin_id, capability) VALUES (?, ?) ON CONFLICT DO NOTHING`, id, capability)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return nil
		}
		changed = true
		return r.decided(tx, id, actor, "granted", capability)
	})
	return changed, err
}

// Withdraw removes a grant; the account of both survives in the history.
func (r *Registry) Withdraw(id, capability, actor string) (bool, error) {
	changed := false
	err := r.within(func(tx *sql.Tx) error {
		result, err := tx.Exec(`DELETE FROM "grant" WHERE plugin_id = ? AND capability = ?`, id, capability)
		if err != nil {
			return err
		}
		if n, _ := result.RowsAffected(); n == 0 {
			return nil
		}
		changed = true
		return r.decided(tx, id, actor, "withdrawn", capability)
	})
	return changed, err
}

func (r *Registry) decided(tx *sql.Tx, id, actor, action, capability string) error {
	var named any
	if capability != "" {
		named = capability
	}
	_, err := tx.Exec(`INSERT INTO decision (plugin_id, at, actor, action, capability) VALUES (?, ?, ?, ?, ?)`,
		id, r.now().UTC().Format(time.RFC3339Nano), actor, action, named)
	return err
}

func (r *Registry) within(work func(*sql.Tx) error) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	if err := work(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func known(result sql.Result, id string) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no plugin %s is declared", id)
	}
	return nil
}
