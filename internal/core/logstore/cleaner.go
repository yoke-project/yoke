package logstore

import (
	"context"
	"hash/fnv"
	"time"
)

// CoreGroup is the group of the entries that belong to no unit. It begins with a path separator, which
// no unit name may, so no declaration can collide with it.
const CoreGroup = "/core"

// vacuumAfter is how many deletions accumulate before the file is rewritten to give the space back.
const vacuumAfter = 10_000

// Limits are a group's three limits. Each is independent of the others, and zero constrains nothing.
type Limits struct {
	Age            time.Duration // how old an entry may be
	Bytes, Entries uint64        // how much, and how many, a group may hold
}

// DefaultLimits are the three figures a group takes when nothing says otherwise: seven days, fifty
// megabytes, a hundred thousand entries. They are inherited rather than measured.
func DefaultLimits() Limits {
	return Limits{Age: 7 * 24 * time.Hour, Bytes: 50_000_000, Entries: 100_000}
}

// Groups are the groups the store holds entries for: one per unit, and CoreGroup for what belongs to none.
func (s *Store) Groups() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT coalesce(unit, ?) FROM entry`, CoreGroup)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Clean applies l to one group, in the order age, bytes, count, each pass seeing what the last left, and
// returns how many entries it removed. Bytes are a message's and its detail's, which undercounts the row.
// What is spent is spent oldest first, by sequence, and nothing else about an entry is read.
func (s *Store) Clean(group string, l Limits, now time.Time) (int, error) {
	where, args := "unit = ?", []any{group}
	if group == CoreGroup {
		where, args = "unit IS NULL", nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	removed := 0
	run := func(statement string, extra ...any) error {
		result, err := tx.Exec(statement, append(append([]any(nil), args...), extra...)...)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		removed += int(n)
		return nil
	}
	if l.Age > 0 {
		if err := run(`DELETE FROM entry WHERE `+where+` AND at < ?`, now.Add(-l.Age).UTC().Format(stamp)); err != nil {
			return 0, err
		}
	}
	if l.Bytes > 0 {
		// What is kept is the newest entries whose bytes, counted from the newest, fit.
		if err := run(`DELETE FROM entry WHERE seq IN (SELECT seq FROM (
			SELECT seq, sum(length(CAST(message AS BLOB)) + coalesce(length(detail), 0)) OVER (ORDER BY seq DESC) AS kept
			FROM entry WHERE `+where+`) WHERE kept > ?)`, int64(l.Bytes)); err != nil {
			return 0, err
		}
	}
	if l.Entries > 0 {
		if err := run(`DELETE FROM entry WHERE seq IN (SELECT seq FROM entry WHERE `+where+` ORDER BY seq DESC LIMIT -1 OFFSET ?)`, int64(l.Entries)); err != nil {
			return 0, err
		}
	}
	return removed, tx.Commit()
}

// Vacuum rewrites the file, giving back the space deleted entries left.
func (s *Store) Vacuum() error {
	if _, err := s.db.Exec(`VACUUM`); err != nil {
		return err
	}
	s.vacuums.Add(1)
	return nil
}

func (s *Store) vacuumed() int { return int(s.vacuums.Load()) }

// Offset is the instance's place in the interval: derived from its name, so it is the same at every start
// of that instance and differs between instances brought up together.
func Offset(instance string, interval time.Duration) time.Duration {
	if interval <= 0 {
		return 0
	}
	h := fnv.New64a()
	h.Write([]byte(instance))
	return time.Duration(h.Sum64() % uint64(interval))
}

// Cleaner runs retention over a store, a cycle every interval.
type Cleaner struct {
	Store    *Store
	Policy   func(group string) Limits // a group's limits, resolved at every cycle
	Instance string                    // whose offset the first cycle takes
	Interval time.Duration             // an hour when zero
	// After is how the cleaner waits; time.After when nil.
	After func(time.Duration) <-chan time.Time
	// Report is told of a cycle that failed; optional. A failed cycle costs nothing but its own work.
	Report func(error)

	deleted int // since the last VACUUM
}

func (c *Cleaner) interval() time.Duration {
	if c.Interval <= 0 {
		return time.Hour
	}
	return c.Interval
}

// First is when the first cycle runs for a cleaner started at started: one interval later, plus the
// instance's offset. Startup is the busiest moment a Core has, and a scan buys nothing there.
func (c *Cleaner) First(started time.Time) time.Time {
	return started.Add(c.interval() + Offset(c.Instance, c.interval()))
}

// Cycle cleans every group the store holds, and vacuums once enough has been deleted since the last time.
func (c *Cleaner) Cycle(now time.Time) (deleted int, vacuumed bool, err error) {
	groups, err := c.Store.Groups()
	if err != nil {
		return 0, false, err
	}
	for _, g := range groups {
		n, err := c.Store.Clean(g, c.Policy(g), now)
		deleted += n
		if err != nil {
			c.deleted += deleted
			return deleted, false, err
		}
	}
	c.deleted += deleted
	if c.deleted >= vacuumAfter {
		if err := c.Store.Vacuum(); err != nil {
			return deleted, false, err
		}
		c.deleted = 0
		vacuumed = true
	}
	return deleted, vacuumed, nil
}

// Run runs a cycle at First and every interval after, until ctx ends.
func (c *Cleaner) Run(ctx context.Context) {
	after := c.After
	if after == nil {
		after = time.After
	}
	wait := c.interval() + Offset(c.Instance, c.interval())
	for {
		select {
		case <-ctx.Done():
			return
		case <-after(wait):
		}
		if _, _, err := c.Cycle(time.Now()); err != nil && c.Report != nil {
			c.Report(err)
		}
		wait = c.interval()
	}
}
