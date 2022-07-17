package checkers

import (
	"context"

	"github.com/go-mysql-org/go-mysql/client"
	"github.com/moeryomenko/healing"
)

// MySQLProbes returns liveness and readiness probes for mysql pool.
func MySQLProbes(pool *client.Pool, maxAlive int, opts ...PoolOptions) (
	liveness, readiness func(context.Context) healing.CheckResult,
) {
	cfg := pool_config{lowerLimit: defaultLowerLimit, livenessPeriod: defaultPoolLivenessPeriod}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(checkPoolLiveness(ctx, cfg.livenessPeriod, func(ctx context.Context) error {
			conn, err := pool.GetConn(ctx)
			if err != nil {
				return err
			}
			defer func() { pool.PutConn(conn) }()
			return conn.Ping()

		}))
	}, MySQLReadinessProber(pool, maxAlive, opts...)
}

// MySQLReadinessProber returns mysql conn pool readiness checker function.
func MySQLReadinessProber(pool *client.Pool, maxAlive int, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	check := checkMySQLPoolAvailability(pool)

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			var stats client.ConnectionStats
			pool.GetStats(&stats)

			if stats.TotalCount < maxAlive {
				return check(ctx)
			}

			return poolCheck(ctx, stats.IdleCount, maxAlive, cfg.lowerLimit, check)
		})
	}
}

func checkMySQLPoolAvailability(pool *client.Pool) func(context.Context) error {
	return func(ctx context.Context) error {
		conn, err := pool.GetConn(ctx)
		if err != nil {
			return err
		}
		pool.PutConn(conn)
		return nil
	}
}
