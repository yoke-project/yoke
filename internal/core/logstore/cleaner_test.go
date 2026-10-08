package logstore_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/yoke-project/yoke/internal/core/logstore"
)

// fill appends n entries to unit's group, at, each a message of ten bytes, and waits until they are
// written — by the store's last sequence, which deletions do not move back.
func fill(t *testing.T, s *logstore.Store, unit string, incarnation uint64, at time.Time, n int, severity int, detail []byte) {
	t.Helper()
	before, err := s.Last()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if err := s.Append(logstore.Entry{At: at, Unit: unit, Incarnation: incarnation, Source: logstore.Stdout,
			Severity: severity, Message: "0123456789", Detail: detail}); err != nil {
			t.Fatal(err)
		}
	}
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		last, err := s.Last()
		if err != nil {
			t.Fatal(err)
		}
		if last >= before+uint64(n) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d entries were stored", last-before, n)
		}
	}
}

// of are the entries of a group, oldest first.
func of(t *testing.T, s *logstore.Store, group string) []logstore.Entry {
	t.Helper()
	all, err := s.Entries(0)
	if err != nil {
		t.Fatal(err)
	}
	var out []logstore.Entry
	for _, e := range all {
		if e.Unit == group || (group == logstore.CoreGroup && e.Unit == "") {
			out = append(out, e)
		}
	}
	return out
}

func seqs(entries []logstore.Entry) []uint64 {
	var out []uint64
	for _, e := range entries {
		out = append(out, e.Seq)
	}
	return out
}

// std: yoke:the-cleaner.01
func TestThreeLimitsInTheirOrder(t *testing.T) {
	s, _, _ := open(t)
	now := time.Now()
	fill(t, s, "acquire", 1, now.Add(-8*24*time.Hour), 4, 10, nil)
	fill(t, s, "acquire", 1, now, 6, 10, nil)
	fill(t, s, "acquire", 1, now, 2, 10, []byte("0123456789"))
	all := seqs(of(t, s, "acquire"))

	if _, err := s.Clean("acquire", logstore.Limits{Age: 7 * 24 * time.Hour}, now); err != nil {
		t.Fatal(err)
	}
	if got := seqs(of(t, s, "acquire")); !slices.Equal(got, all[4:]) {
		t.Fatalf("after the age pass %v are left, want %v", got, all[4:])
	}
	// Eight entries measure 6×10 + 2×20 = 100 bytes; eighty is reached by removing the two oldest.
	if _, err := s.Clean("acquire", logstore.Limits{Bytes: 80}, now); err != nil {
		t.Fatal(err)
	}
	if got := seqs(of(t, s, "acquire")); !slices.Equal(got, all[6:]) {
		t.Fatalf("after the bytes pass %v are left, want %v", got, all[6:])
	}
	if _, err := s.Clean("acquire", logstore.Limits{Entries: 4}, now); err != nil {
		t.Fatal(err)
	}
	if got := seqs(of(t, s, "acquire")); !slices.Equal(got, all[8:]) {
		t.Fatalf("after the count pass %v are left, want %v", got, all[8:])
	}

	// All three at once, on a fresh store: age, then bytes, then count, each seeing what the last left.
	s2, _, _ := open(t)
	fill(t, s2, "acquire", 1, now.Add(-8*24*time.Hour), 4, 10, nil)
	fill(t, s2, "acquire", 1, now, 6, 10, nil)
	fill(t, s2, "acquire", 1, now, 2, 10, []byte("0123456789"))
	all2 := seqs(of(t, s2, "acquire"))
	removed, err := s2.Clean("acquire", logstore.Limits{Age: 7 * 24 * time.Hour, Bytes: 80, Entries: 4}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := seqs(of(t, s2, "acquire")); removed != 8 || !slices.Equal(got, all2[8:]) {
		t.Errorf("all three removed %d and left %v, want 8 and %v", removed, got, all2[8:])
	}
}

// std: yoke:the-cleaner.02
func TestZeroConstrainsNothing(t *testing.T) {
	s, _, _ := open(t)
	now := time.Now()
	fill(t, s, "acquire", 1, now.Add(-30*24*time.Hour), 12, 10, nil)
	if removed, err := s.Clean("acquire", logstore.Limits{}, now); err != nil || removed != 0 || len(of(t, s, "acquire")) != 12 {
		t.Errorf("all three at zero removed %d (%v)", removed, err)
	}
	if d := logstore.DefaultLimits(); d.Age != 7*24*time.Hour || d.Bytes != 50_000_000 || d.Entries != 100_000 {
		t.Errorf("the defaults are %+v", d)
	}
}

// std: yoke:the-cleaner.03
func TestTheGroupIsTheUnit(t *testing.T) {
	s, _, _ := open(t)
	now := time.Now()
	fill(t, s, "noisy", 1, now, 100, 10, nil)
	fill(t, s, "quiet", 1, now, 5, 10, nil)
	fill(t, s, "quiet", 0, now, 3, 10, nil)
	fill(t, s, "", 0, now, 4, 10, nil)
	groups, err := s.Groups()
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(groups)
	if !slices.Equal(groups, []string{"/core", "noisy", "quiet"}) {
		t.Fatalf("the groups are %v", groups)
	}
	c := &logstore.Cleaner{Store: s, Policy: func(string) logstore.Limits { return logstore.Limits{Entries: 10} }}
	if _, _, err := c.Cycle(now); err != nil {
		t.Fatal(err)
	}
	if n, q, core := len(of(t, s, "noisy")), len(of(t, s, "quiet")), len(of(t, s, logstore.CoreGroup)); n != 10 || q != 8 || core != 4 {
		t.Errorf("noisy kept %d, quiet %d, /core %d", n, q, core)
	}
}

// std: yoke:the-cleaner.04
func TestSeverityIsNotRead(t *testing.T) {
	s, _, _ := open(t)
	now := time.Now()
	fill(t, s, "acquire", 1, now, 3, 90, nil)
	fill(t, s, "acquire", 1, now, 3, 10, nil)
	if _, err := s.Clean("acquire", logstore.Limits{Entries: 3}, now); err != nil {
		t.Fatal(err)
	}
	for _, e := range of(t, s, "acquire") {
		if e.Severity != 10 {
			t.Errorf("an entry at %d survived", e.Severity)
		}
	}
}

// clock hands the cleaner timers the test fires, recording each wait asked for.
type clock struct {
	mu    sync.Mutex
	waits []time.Duration
	fire  chan time.Time
}

func (c *clock) after(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.waits = append(c.waits, d)
	return c.fire
}

func (c *clock) asked() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

// std: yoke:the-cleaner.05
func TestACycleEveryIntervalTheFirstAfterTheOffset(t *testing.T) {
	offsets := map[string]time.Duration{}
	for _, name := range []string{"yoke", "bench", "bench-2"} {
		first, again := logstore.Offset(name, time.Hour), logstore.Offset(name, time.Hour)
		if first != again || first < 0 || first >= time.Hour {
			t.Errorf("%s's offset is %v then %v", name, first, again)
		}
		offsets[name] = first
	}
	if offsets["yoke"] == offsets["bench"] || offsets["bench"] == offsets["bench-2"] || offsets["yoke"] == offsets["bench-2"] {
		t.Errorf("the offsets coincide: %v", offsets)
	}

	s, _, _ := open(t)
	cycles := make(chan struct{}, 4)
	clk := &clock{fire: make(chan time.Time)}
	c := &logstore.Cleaner{Store: s, Instance: "bench", Interval: time.Hour, After: clk.after,
		Policy: func(string) logstore.Limits { cycles <- struct{}{}; return logstore.Limits{} }}
	fill(t, s, "acquire", 1, time.Now(), 1, 10, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)
	for deadline := time.Now().Add(2 * time.Second); len(clk.asked()) == 0; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the cleaner never waited")
		}
	}
	select {
	case <-cycles:
		t.Fatal("a cycle ran before its first wait elapsed")
	default:
	}
	clk.fire <- time.Now()
	<-cycles
	clk.fire <- time.Now()
	<-cycles
	if got := clk.asked(); len(got) < 2 || got[0] != time.Hour+offsets["bench"] || got[1] != time.Hour {
		t.Errorf("the cleaner waited %v, want %v then %v", got, time.Hour+offsets["bench"], time.Hour)
	}
}

// std: yoke:the-cleaner.06
func TestVacuumOnceTenThousandDeletionsAccumulated(t *testing.T) {
	s, _, _ := open(t)
	c := &logstore.Cleaner{Store: s, Policy: func(string) logstore.Limits { return logstore.Limits{Entries: 1} }}
	now := time.Now()
	fill(t, s, "acquire", 1, now, 6000, 10, nil)
	deleted, vacuumed, err := c.Cycle(now)
	if err != nil || deleted != 5999 || vacuumed || logstore.Vacuums(s) != 0 {
		t.Fatalf("the first cycle deleted %d, vacuumed %v (%d) %v", deleted, vacuumed, logstore.Vacuums(s), err)
	}
	fill(t, s, "acquire", 1, now, 6000, 10, nil)
	deleted, vacuumed, err = c.Cycle(now)
	if err != nil || deleted != 6000 || !vacuumed || logstore.Vacuums(s) != 1 {
		t.Fatalf("the second cycle deleted %d, vacuumed %v (%d) %v", deleted, vacuumed, logstore.Vacuums(s), err)
	}
	fill(t, s, "acquire", 1, now, 10, 10, nil)
	if _, vacuumed, _ := c.Cycle(now); vacuumed {
		t.Error("the count did not start again")
	}
}
