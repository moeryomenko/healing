package checkers

import (
	"context"
	"database/sql"

	"github.com/moeryomenko/healing"
)

// SQLProbes returns liveness and readiness probes for go sql.DB.
func SQLProbes(db *sql.DB, opts ...PoolOptions) (
	liveness, readiness func(context.Context) healing.CheckResult,
) {
	cfg := pool_config{lowerLimit: defaultLowerLimit, livenessPeriod: defaultPoolLivenessPeriod}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(checkPoolLiveness(ctx, cfg.livenessPeriod, db.PingContext))
	}, SQLPoolReadinessChecker(db, opts...)
}

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
				return checkPoolAvailability(db)(ctx)
			}

			return poolCheck(
				ctx,
				stats.Idle, stats.MaxOpenConnections, cfg.lowerLimit,
				checkPoolAvailability(db),
			)
		})
	}
}

func checkPoolAvailability(pool *sql.DB) func(context.Context) error {
	return func(ctx context.Context) error {
		conn, err := pool.Conn(ctx)
		if err != nil {
			return err
		}
		return conn.Close()
	}
}
