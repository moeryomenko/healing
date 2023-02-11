package checkers

import (
	"context"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/moeryomenko/healing"
)

// PgxProbes returns liveness and readiness probes for pgxpool.Pool.
func PgxProbes(pool *pgxpool.Pool, opts ...PoolOptions) (
	liveness, readiness func(context.Context) healing.CheckResult,
) {
	cfg := pool_config{lowerLimit: defaultLowerLimit, livenessPeriod: defaultPoolLivenessPeriod}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(checkPoolLiveness(ctx, cfg.livenessPeriod, pool.Ping))
	}, PgxReadinessProber(pool, opts...)
}

// PgxReadinessProber returns pg conn pool readiness checker function.
func PgxReadinessProber(pool *pgxpool.Pool, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	// NOTE: only check we can aquire connection from pool w/o real execution ping command.
	check := func(ctx context.Context) error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return err
		}
		conn.Release()
		return nil
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			stats := pool.Stat()

			total, max := stats.TotalConns(), pool.Config().MaxConns

			// NOTE: if connection pool dont limited or numbers connection into pool
			// less than can aquire, check availablity of pool.
			if max == 0 || total < max {
				return check(ctx)
			}

			return poolCheck(ctx, int(stats.IdleConns()), int(total), cfg.lowerLimit, check)
		})
	}
}
