package checkers

import (
	"context"

	"github.com/go-mysql-org/go-mysql/client"
	"github.com/moeryomenko/healing"
)

// MySQLReadinessProber returns mysql conn pool readiness checker function.
func MySQLReadinessProber(pool *client.Pool, maxAlive int, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			var stats client.ConnectionStats
			pool.GetStats(&stats)

			ping := func(ctx context.Context) error {
				conn, err := pool.GetConn(ctx)
				if err != nil {
					return err
				}
				defer func() { pool.PutConn(conn) }()

				return conn.Ping()
			}

			if stats.TotalCount < maxAlive {
				return ping(ctx)
			}

			return poolCheck(ctx, stats.IdleCount, maxAlive, cfg.lowerLimit, ping)
		})
	}
}
