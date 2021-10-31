package health

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	defaultCheckTimeout = 2 * time.Second

	successCheck = iota
	failedCheck
)

// CheckGroup launch checker concurrently.
type CheckGroup struct {
	checkers []checkFunc
	timeout  time.Duration
	status   int32
}

// NewCheckGroup returns new instacnce CheckGroup.
func NewCheckGroup(timeout time.Duration, checkers ...checkFunc) *CheckGroup {
	group := &CheckGroup{timeout: timeout, checkers: make([]checkFunc, 0, len(checkers))}
	group.checkers = append(group.checkers, checkers...)
	return group
}

// AddChecker adds checker to CheckGroup.
func (g *CheckGroup) AddChecker(checker checkFunc) {
	g.checkers = append(g.checkers, checker)
}

// Check runs checkers.
func (g *CheckGroup) Check(ctx context.Context) {
	group := &errgroup.Group{}
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	// NOTE: flush status before checks.
	atomic.StoreInt32(&g.status, successCheck)

	for _, checker := range g.checkers {
		checker := checker
		group.Go(func() error { return checker(ctx) })
	}

	err := group.Wait()
	if err != nil {
		atomic.StoreInt32(&g.status, failedCheck)
	}
}

// IsOK returns true if all checks passed normal.
func (g *CheckGroup) IsOK() bool {
	return atomic.LoadInt32(&g.status) == successCheck
}
