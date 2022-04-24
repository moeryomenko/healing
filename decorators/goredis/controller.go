package goredis

import (
	"context"
	"errors"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-redis/redis/v8"
	"github.com/moeryomenko/healing"
)

var ErrPoolNotReady = errors.New("currently pool is busy")

// RedisController is adapter for check health of redis pool connections.
type RedisController struct {
	client       *redis.Client
	pingInterval time.Duration
}

// New returns new instance of go-redis readiness controller.
func New(client *redis.Client, opts ...Option) *RedisController {
	c := &RedisController{
		client: client,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CheckReadinessProbe checks if the pool can acquire connection.
func (c *RedisController) CheckReadinessProbe(ctx context.Context) healing.CheckResult {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	maxElapseTime := time.Until(deadline)

	err := backoff.Retry(func() error {
		stat := c.client.PoolStats()
		if stat.IdleConns != 0 {
			res := c.client.Ping(ctx)
			return res.Err()
		}
		return ErrPoolNotReady
	}, &backoff.ExponentialBackOff{
		InitialInterval:     c.pingInterval / 4,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         c.pingInterval / 2,
		MaxElapsedTime:      maxElapseTime,
		Clock:               backoff.SystemClock,
	})
	if err != nil {
		return healing.CheckResult{
			Err:     err,
			Details: err.Error(),
		}
	}

	return healing.CheckResult{
		Details: "OK",
	}
}

type Option func(*RedisController)

// WithPingInterval sets checks interval between pings on failures.
func WithPingInterval(interval time.Duration) Option {
	return func(sc *RedisController) {
		sc.pingInterval = interval
	}
}
