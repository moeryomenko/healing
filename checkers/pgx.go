package checkers

import (
	"context"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/moeryomenko/healing"
)

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

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			stats := pool.Stat()

			total, max := stats.TotalConns(), pool.Config().MaxConns

			if max == 0 || total < max {
				return checkPgPoolAvailability(pool)(ctx)
			}

			return poolCheck(ctx, int(stats.IdleConns()), int(total), cfg.lowerLimit, checkPgPoolAvailability(pool))
		})
	}
}

func checkPgPoolAvailability(pool *pgxpool.Pool) func(context.Context) error {
	return func(ctx context.Context) error {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			return err
		}
		conn.Release()
		return nil
	}
}
