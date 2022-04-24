package goredis

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/moeryomenko/healing"
)

const defaultPingInterval = 100 * time.Millisecond

var ErrPoolNotReady = errors.New("currently pool is busy")

// RedisController is adapter for check health of redis pool connections.
type RedisController struct {
	client       *redis.Client
	pingInterval time.Duration
}

// New returns new instance of go-redis readiness controller.
func New(client *redis.Client, opts ...Option) *RedisController {
	c := &RedisController{
		client:       client,
		pingInterval: defaultPingInterval,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CheckReadinessProbe checks if the pool can acquire connection.
func (c *RedisController) CheckReadinessProbe(ctx context.Context) healing.CheckResult {
	return healing.CheckHelper(ctx, c.pingInterval, func() error {
		stat := c.client.PoolStats()
		if stat.IdleConns != 0 {
			res := c.client.Ping(ctx)
			return res.Err()
		}
		return nil
	})
}

type Option func(*RedisController)

// WithPingInterval sets checks interval between pings on failures.
func WithPingInterval(interval time.Duration) Option {
	return func(sc *RedisController) {
		sc.pingInterval = interval
	}
}
