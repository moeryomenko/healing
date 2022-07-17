package checkers

import (
	"context"
	"errors"
	"time"

	"github.com/moeryomenko/healing"
)

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
	lastPing := time.Now()
	return func() error {
		now := time.Now()
		if now.After(lastPing.Add(period)) {
			return ping(ctx)
		}
		return nil
	}
}

func poolCheck(ctx context.Context, idle, max, lower int, check func(context.Context) error) error {
	ratioFree := idle / max * 100

	if ratioFree > lower {
		return check(ctx)
	}

	// NOTE: don't have enough free connections
	// that will be useful for processing incoming requests.
	return nil
}
