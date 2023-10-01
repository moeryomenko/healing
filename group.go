package healing

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/moeryomenko/synx"
)

const defaultCheckTimeout = 2 * time.Second

// CheckGroup launch checker concurrently.
type CheckGroup struct {
	checkers map[string]checkFunc
	timeout  time.Duration
	status   atomic.Bool

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

	group := synx.NewCtxGroup(ctx)

	// NOTE: flush status before checks.
	g.status.Store(true)

	for subsystem, checker := range g.checkers {
		subsystem := subsystem
		checker := checker
		group.Go(func(ctx context.Context) error {
			res := checker(ctx)
			g.setStatus(subsystem, res)
			return res.Error
		})
	}

	err := group.Wait()
	if err != nil {
		g.status.Store(false)
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
	return g.status.Load()
}

func (g *CheckGroup) setStatus(subsystem string, status CheckResult) {
	g.mu.Lock()
	g.checkStatuses[subsystem] = status
	g.mu.Unlock()
}
