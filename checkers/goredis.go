package checkers

import (
	"context"

	"github.com/go-redis/redis/v8"
	"github.com/moeryomenko/healing"
)

// RedisReadinessProber returns redis conn pool readiness checker function.
func RedisReadinessProber(client *redis.Client, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			stats := client.PoolStats()

			if stats.TotalConns == 0 {
				res := client.Ping(ctx)
				return res.Err()
			}

			return poolCheck(ctx,
				int(stats.IdleConns), int(stats.TotalConns), cfg.lowerLimit,
				func(ctx context.Context) error {
					res := client.Ping(ctx)
					return res.Err()
				})
		})
	}
}
