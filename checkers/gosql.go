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

	check := func(ctx context.Context) error {
		conn, err := db.Conn(ctx)
		if err != nil {
			return err
		}
		return conn.Close()
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			stats := db.Stats()
			// NOTE: if connection pool dont limited or numbers connection into pool
			// less than can aquire, check availablity of pool.
			if stats.MaxOpenConnections == 0 || stats.InUse < stats.MaxOpenConnections {
				return check(ctx)
			}

			return poolCheck(ctx, stats.Idle, stats.MaxOpenConnections, cfg.lowerLimit, check)
		})
	}
}
