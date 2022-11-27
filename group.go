package healing

import (
	"context"
	"sync"
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
	checkers map[string]checkFunc
	timeout  time.Duration
	status   int32

	checkStatuses map[string]CheckResult
	mu            sync.Mutex
}

// NewCheckGroup returns new instacnce CheckGroup.
func NewCheckGroup(timeout time.Duration) *CheckGroup {
	group := &CheckGroup{
		timeout:       timeout,
		checkers:      make(map[string]checkFunc),
		checkStatuses: make(map[string]CheckResult),
	}
	return group
}

// AddChecker adds checker to CheckGroup.
func (g *CheckGroup) AddChecker(subsystem string, checker checkFunc) {
	g.checkers[subsystem] = checker
}

// Check runs checkers.
func (g *CheckGroup) Check(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	group, checkCtx := errgroup.WithContext(ctx)

	// NOTE: flush status before checks.
	atomic.StoreInt32(&g.status, successCheck)

	for subsystem, checker := range g.checkers {
		subsystem := subsystem
		checker := checker
		group.Go(func() error {
			select {
			case <-checkCtx.Done():
				g.setStatus(subsystem, CheckResult{Error: ctx.Err(), Status: DOWN})
				return ctx.Err()
			case res := <-asyncInvoke(checkCtx, checker):
				g.setStatus(subsystem, res)
				return res.Error
			}
		})
	}

	err := group.Wait()
	if err != nil {
		atomic.StoreInt32(&g.status, failedCheck)
	}
}

// GetDetails returns result of checks.
func (g *CheckGroup) GetDetails() map[string]CheckResult {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.checkStatuses
}

// IsOK returns true if all checks passed normal.
func (g *CheckGroup) IsOK() bool {
	return atomic.LoadInt32(&g.status) == successCheck
}

func (g *CheckGroup) setStatus(subsystem string, status CheckResult) {
	g.mu.Lock()
	g.checkStatuses[subsystem] = status
	g.mu.Unlock()
}

func asyncInvoke(ctx context.Context, fn checkFunc) chan CheckResult {
	ch := make(chan CheckResult, 1)

	go func() {
		ch <- fn(ctx)
	}()

	return ch
}
