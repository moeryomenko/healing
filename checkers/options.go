package checkers

import "time"

type PoolOptions func(*pool_config)

// WithLowerLimitPool sets the lower limit of free connections
// during which a real ping of the base is performed.
func WithLowerLimitPool(lower uint) PoolOptions {
	return func(p *pool_config) {
		p.lowerLimit = int(lower)
	}
}

// WithPoolLivenessPeriod sets period between real ping requests to database.
func WithPoolLivenessPeriod(period time.Duration) PoolOptions {
	return func(p *pool_config) {
		p.livenessPeriod = period
	}
}

const (
	defaultLowerLimit         = 5
	defaultPoolLivenessPeriod = 5 * time.Second
)

type pool_config struct {
	lowerLimit int

	livenessPeriod time.Duration
}
