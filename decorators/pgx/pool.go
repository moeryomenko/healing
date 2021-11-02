package pgx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

const pingInterval = 500 * time.Millisecond

type Config struct {
	Host           string
	Port           uint16
	User, Password string
	DBName         string
}

// Pool is decorated pgxpool.Pool with health checker of Pool.
type Pool struct {
	*pgxpool.Pool

	lastPingAt int64
}

// New create new postgres connection instance
func New(ctx context.Context, cfg Config, opts ...Option) (*Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(
		fmt.Sprintf("user=%s password=%s host=%s port=%d dbname=%s",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.DBName))
	if err != nil {
		return nil, err
	}

	shiftedPingChecker := time.Now().Unix()

	poolConfig.BeforeAcquire = func(_ context.Context, _ *pgx.Conn) bool {
		// shift ping check, in order not to load the pool with requests.
		atomic.StoreInt64(&shiftedPingChecker, time.Now().Unix())
		return true
	}

	for _, opt := range opts {
		opt(poolConfig)
	}

	pool, err := pgxpool.ConnectConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}
	decoratedPool := &Pool{pool, shiftedPingChecker}

	conn, err := decoratedPool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()

	// ping connection.
	if err := conn.Ping(ctx); err != nil {
		return nil, err
	}

	return decoratedPool, nil
}

// CheckReadinessProber checks if the pool can acquire connection.
// NOTE: perhaps you are confused that ping is used, but in this way
// we check first of all that we can capture the connection,
// but the availability of postgresql.
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
		return p.Ping(ctx)
	}, &backoff.ExponentialBackOff{
		InitialInterval:     pingInterval / 4,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         pingInterval / 2,
		MaxElapsedTime:      maxElapseTime,
		Clock:               backoff.SystemClock,
	})
}

type Option func(*pgxpool.Config)

// WithTLS sets tls cert for connect with postges.
// NOTE: if reading the certificate fails,
// it will not be set.
func WithTLS(certPath string) Option {
	return func(cfg *pgxpool.Config) {
		certPool := x509.NewCertPool()
		cert, err := ioutil.ReadFile(certPath)
		if err != nil {
			return
		}
		certPool.AppendCertsFromPEM(cert)
		// nolint:gosec // it's ok.
		cfg.ConnConfig.TLSConfig = &tls.Config{
			ServerName: cfg.ConnConfig.Host, // reuse host from intialization.
			RootCAs:    certPool,
		}
	}
}

// WithHealthCheckPeriod sets the duration between checks of the health of idle connections.
func WithHealthCheckPeriod(interval time.Duration) Option {
	return func(c *pgxpool.Config) {
		c.HealthCheckPeriod = interval
	}
}

type PoolConfig struct {
	// MaxConnLifetime is the duration since creation after which a connection will be automatically closed.
	MaxConnLifetime time.Duration
	// MaxConns is the maximum size of the pool.
	MaxConns int32
	// MinConns is the minimum size of the pool. The health check will increase the number of connections to this
	// amount if it had dropped below.
	MinConns int32
}

// WithDefaultPoolConfig sets default pool configuration.
func WithDefaultPoolConfig() Option {
	return WithPoolConfig(PoolConfig{
		MaxConnLifetime: 2 * time.Minute,
		MaxConns:        25,
		MinConns:        10,
	})
}

// WithPoolConfig sets pool configuration.
// Such as: maximum connection life, size of pool, and minimal healthed connection.
func WithPoolConfig(cfg PoolConfig) Option {
	return func(c *pgxpool.Config) {
		c.MaxConnLifetime = cfg.MaxConnLifetime
		c.MaxConns = cfg.MaxConns
		c.MinConns = cfg.MinConns
	}
}
