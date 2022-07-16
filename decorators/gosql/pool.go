package gosql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/moeryomenko/healing"
)

var ErrPoolNotReady = errors.New("currently pool is busy")

const (
	defaultPingInterval = 500 * time.Millisecond

	defaultPingPolicy = 5
)

type SqlController struct {
	db           *sql.DB
	pingInterval time.Duration
	lowerLimit   int
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
		if stats.MaxOpenConnections == 0 {
			return c.db.PingContext(ctx)
		}

		ratioFree := (stats.MaxOpenConnections - stats.InUse) / stats.MaxOpenConnections * 100

		if ratioFree == 0 {
			return ErrPoolNotReady
		}

		if ratioFree > c.lowerLimit {
			return c.db.PingContext(ctx)
		}

		// NOTE: don't have enough free connections
		// that will be useful for processing incoming requests.
		return nil
	})
}

type Option func(*SqlController)

// WithPingInterval sets checks ping retry on full pool.
func WithPingInterval(interval time.Duration) Option {
	return func(sc *SqlController) {
		sc.pingInterval = interval
	}
}

// WithLowerLimitPool sets the lower limit of free connections
// during which a real ping of the base is performed.
func WithLowerLimitPool(lower uint) Option {
	return func(sc *SqlController) {
		sc.lowerLimit = int(lower)
	}
}
