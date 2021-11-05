package mysql

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/go-mysql-org/go-mysql/client"
)

const pingInterval = 500 * time.Millisecond

type Config struct {
	Host           string
	Port           uint16
	User, Password string
	DBName         string
}

// Pool is decorated *client.Pool with health checker of Pool.
type Pool struct {
	*client.Pool

	lastPingAt int64
}

// New return new instance of mysql Pool.
func New(ctx context.Context, cfg Config, opts ...Option) (*Pool, error) {
	defaultPoolConfig := &PoolConfig{
		MinAlive: 10,
		MaxAlive: 25,
		MaxIdle:  25,
	}
	for _, opt := range opts {
		opt(defaultPoolConfig)
	}
	pool := client.NewPool(
		log.Default().Printf,
		defaultPoolConfig.MinAlive,
		defaultPoolConfig.MaxAlive,
		defaultPoolConfig.MaxIdle,
		fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		cfg.User, cfg.Password,
		cfg.DBName,
	)

	decoratedPool := &Pool{pool, 0}
	conn, err := decoratedPool.GetConn(ctx)
	if err != nil {
		return nil, err
	}
	defer pool.PutConn(conn)
	if err := conn.Ping(); err != nil {
		return nil, err
	}

	return decoratedPool, nil
}

func (p *Pool) GetConn(ctx context.Context) (*client.Conn, error) {
	// shift ping check, in order not to load the pool with requests.
	atomic.StoreInt64(&p.lastPingAt, time.Now().Unix())
	return p.Pool.GetConn(ctx)
}

// CheckReadinessProber checks if the pool can acquire connection.
// NOTE: perhaps you are confused that ping is used, but in this way
// we check first of all that we can capture the connection,
// but the availability of mysql.
func (p *Pool) CheckReadinessProber(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	maxElapseTime := time.Until(deadline)

	return backoff.Retry(func() error {
		// NOTE: avoid load pool by acquiring connection from
		// and reduce contention for connection under service load.
		lastPingAt := time.Unix(atomic.LoadInt64(&p.lastPingAt), 0)
		if time.Now().Before(lastPingAt.Add(pingInterval)) {
			return nil
		}
		// NOTE: get connection without affecting lastPingAt.
		conn, err := p.Pool.GetConn(ctx)
		if err != nil {
			return err
		}
		defer p.PutConn(conn)
		return conn.Ping()
	}, &backoff.ExponentialBackOff{
		InitialInterval:     pingInterval / 4,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         pingInterval / 2,
		MaxElapsedTime:      maxElapseTime,
		Clock:               backoff.SystemClock,
	})
}

type Option func(*PoolConfig)

func WithPoolConfig(cfg PoolConfig) Option {
	return func(pc *PoolConfig) {
		pc.MaxAlive = cfg.MaxAlive
		pc.MinAlive = cfg.MinAlive
		pc.MaxIdle = cfg.MaxIdle
	}
}

type PoolConfig struct {
	// MinAlive is the minimum size of the pool. The health check will increase the number of connections to this
	// amount if it had dropped below.
	MinAlive int
	// MaxAlive is the maximum size of the pool.
	MaxAlive int
	// MaxIdle is maximum amount idke connection in pool.
	MaxIdle int
}
