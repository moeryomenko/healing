package checkers

import "time"

type PoolOptions func(*pool_config)

// WithRecheckInterval sets recheck interval between failed checks.
func WithRecheckInterval(interval time.Duration) PoolOptions {
	return func(c *pool_config) {
		c.recheckInterval = interval
	}
}

// WithLowerLimitPool sets the lower limit of free connections
// during which a real ping of the base is performed.
func WithLowerLimitPool(lower uint) PoolOptions {
	return func(c *pool_config) {
		c.lowerLimit = int(lower)
	}
}

const (
	defaultPingInterval = 500 * time.Millisecond

	defaultLowerLimit = 5
)

type pool_config struct {
	recheckInterval time.Duration
	lowerLimit      int
}
