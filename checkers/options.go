package checkers

type PoolOptions func(*pool_config)

// WithLowerLimitPool sets the lower limit of free connections
// during which a real ping of the base is performed.
func WithLowerLimitPool(lower uint) PoolOptions {
	return func(c *pool_config) {
		c.lowerLimit = int(lower)
	}
}

const defaultLowerLimit = 5

type pool_config struct {
	lowerLimit int
}
