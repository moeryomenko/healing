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
func New(_ context.Context, db *sql.DB, opts ...Option) *SqlController {
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
		switch stats.MaxOpenConnections - stats.InUse {
		case 0:
			return ErrPoolNotReady
		default:
			// NOTE: if the value is non-zero, then this means that
			// the connection pool has the ability to allocate
			// a connection and perform a ping.
			return c.db.PingContext(ctx)
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
