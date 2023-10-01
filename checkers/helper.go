package checkers

import (
	"context"
	"errors"
	"time"

	"github.com/moeryomenko/healing"
)

// ErrPoolNotReady indicate than database connection pool waste all available connection.
var ErrPoolNotReady = errors.New("currently pool is busy")

// CheckHelper is helper function for check liveness and readiness.
func CheckHelper(check func() error) healing.CheckResult {
	err := check()
	if err != nil {
		return healing.CheckResult{
			Error:  err,
			Status: healing.DOWN,
		}
	}

	return healing.CheckResult{
		Status: healing.UP,
	}
}

func checkPoolLiveness(ctx context.Context, period time.Duration, ping func(context.Context) error) func() error {
	// NOTE: liveness ping has owen check period, cause dont annoy db by ping command.
	lastPing := time.Now()
	return func() error {
		now := time.Now()
		if now.After(lastPing) {
			lastPing = now
			return ping(ctx)
		}
		return nil
	}
}

func poolCheck(ctx context.Context, idle, max, lower int, check func(context.Context) error) error {
	// NOTE: if ratio idle connection in pull great than lower ration
	// we can check pool, but don't have enough free connections
	// that will be useful for processing incoming requests leave check.

	if max == 0 {
		return check(ctx)
	}

	ratioFree := idle / max * 100

	if ratioFree > lower {
		return check(ctx)
	}

	return ErrPoolNotReady
}
