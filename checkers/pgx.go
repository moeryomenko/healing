package checkers

import (
	"context"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/moeryomenko/healing"
)

// PgxReadinessProber returns pg conn pool readiness checker function.
func PgxReadinessProber(pool *pgxpool.Pool, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{recheckInterval: defaultPingInterval, lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(ctx, cfg.recheckInterval, func() error {
			stats := pool.Stat()

			if stats.TotalConns() == 0 {
				return pool.Ping(ctx)
			}

			return poolCheck(ctx,
				int(stats.IdleConns()), int(stats.TotalConns()), cfg.lowerLimit,
				pool.Ping)
		})
	}
}
