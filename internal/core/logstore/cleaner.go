package logstore

import (
	"context"
	"errors"
	"time"
)

// CoreGroup is the group of entries that belong to no unit.
const CoreGroup = "/core"

// Limits are a group's three limits; zero constrains nothing.
type Limits struct {
	Age            time.Duration
	Bytes, Entries uint64
}

// DefaultLimits are the three figures a group takes when nothing says otherwise.
func DefaultLimits() Limits { return Limits{} }

var errNotYet = errors.New("not yet")

func (s *Store) Groups() ([]string, error)                                { return nil, errNotYet }
func (s *Store) Clean(group string, l Limits, now time.Time) (int, error) { return 0, errNotYet }
func (s *Store) Vacuum() error                                            { return errNotYet }
func (s *Store) vacuumed() int                                            { return -1 }
func Offset(instance string, interval time.Duration) time.Duration        { return -1 }

// Cleaner runs retention over a store.
type Cleaner struct {
	Store    *Store
	Policy   func(group string) Limits
	Instance string
	Interval time.Duration
	After    func(time.Duration) <-chan time.Time
}

func (c *Cleaner) Cycle(now time.Time) (int, bool, error) { return 0, false, errNotYet }
func (c *Cleaner) Run(ctx context.Context)                {}
