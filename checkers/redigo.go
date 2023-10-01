package checkers

import (
	"context"
	"errors"

	"github.com/gomodule/redigo/redis"
	"github.com/moeryomenko/healing"
)

func RedigoReadinessProber(pool *redis.Pool, opts ...PoolOptions) func(context.Context) healing.CheckResult {
	cfg := pool_config{lowerLimit: defaultLowerLimit}

	for _, opt := range opts {
		opt(&cfg)
	}

	ping := func(ctx context.Context) error {
		return redigoQuery(ctx, pool, func(conn redis.Conn) error {
			pong, err := redis.String(conn.Do(`PING`))
			if err != nil {
				return err
			}
			if pong != `PONG` {
				return ErrPoolNotReady
			}
			return nil
		})
	}

	return func(ctx context.Context) healing.CheckResult {
		return CheckHelper(func() error {
			stats := pool.Stats()
			return poolCheck(ctx, stats.IdleCount, pool.MaxActive, cfg.lowerLimit, ping)
		})
	}
}

func redigoQuery(ctx context.Context, pool *redis.Pool, f func(redis.Conn) error) (err error) {
	conn, err := pool.GetContext(ctx)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := conn.Close()
		if closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	return f(conn)
}
