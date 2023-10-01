package checkers

import (
	"context"

	"github.com/moeryomenko/healing"
	"github.com/redis/go-redis/v9"
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

			return poolCheck(ctx,
				int(stats.IdleConns), int(stats.TotalConns), cfg.lowerLimit,
				func(ctx context.Context) error {
					return client.Ping(ctx).Err()
				})
		})
	}
}
