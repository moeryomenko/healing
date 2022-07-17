package checkers

import (
	"context"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/moeryomenko/healing"
)

// PgxReadinessProber returns pg conn pool readiness checker function.
func PgxReadinessProber(pool *pgxpool.Pool, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			stats := pool.Stat()

			total, max := stats.TotalConns(), stats.MaxConns()

			if total == 0 || total < max {
				return pool.Ping(ctx)
			}

			return poolCheck(ctx, int(stats.IdleConns()), int(total), cfg.lowerLimit, pool.Ping)
		})
	}
}
