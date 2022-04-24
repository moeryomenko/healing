package gosql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/moeryomenko/healing"
)

var ErrPoolNotReady = errors.New("currently pool is busy")

const defaultPingInterval = 500 * time.Millisecond

type SqlController struct {
	db           *sql.DB
	pingInterval time.Duration
}

// New returns new instance of sql.DB readiness controller.
func New(ctx context.Context, db *sql.DB, opts ...Option) *SqlController {
	controller := &SqlController{
		db:           db,
		pingInterval: defaultPingInterval,
	}
	for _, opt := range opts {
		opt(controller)
	}
	return controller
}

// CheckReadinessProbe checks if the pool can acquire connection.
func (c *SqlController) CheckReadinessProbe(ctx context.Context) healing.CheckResult {
	return healing.CheckHelper(ctx, c.pingInterval, func() error {
		stats := c.db.Stats()
		switch {
		case stats.MaxOpenConnections == 0 || stats.InUse == 0:
			// since the original pool is not limited from above by
			// the number of max open connections, we check the possibility
			// of obtaining a new connection by ping.
			return c.db.PingContext(ctx)
		case stats.MaxOpenConnections-stats.InUse >= 0:
			return nil
		default:
			return ErrPoolNotReady
		}
	})
}

type Option func(*SqlController)

// WithPingInterval sets checks ping retry on full pool.
func WithPingInterval(interval time.Duration) Option {
	return func(sc *SqlController) {
		sc.pingInterval = interval
	}
}
