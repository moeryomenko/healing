package checkers

import (
	"context"
	"database/sql"

	"github.com/moeryomenko/healing"
)

// SQLPoolReadinessChecker returns readiness checker function for golang sql.DB.
func SQLPoolReadinessChecker(db *sql.DB, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			stats := db.Stats()
			if stats.MaxOpenConnections == 0 || stats.InUse < stats.MaxOpenConnections {
				return db.PingContext(ctx)
			}

			return poolCheck(
				ctx,
				stats.Idle, stats.MaxOpenConnections, cfg.lowerLimit,
				db.PingContext,
			)
		})
	}
}
