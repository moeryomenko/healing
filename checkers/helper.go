package checkers

import (
	"context"
	"errors"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
	"github.com/moeryomenko/healing"
)

var ErrPoolNotReady = errors.New("currently pool is busy")

// CheckHelper is helper function for check liveness and readiness.
func CheckHelper(ctx context.Context, pingInterval time.Duration, check func() error) healing.CheckResult {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(time.Second)
	}
	maxElapseTime := time.Until(deadline)

	err := backoff.Retry(check, &backoff.ExponentialBackOff{
		InitialInterval:     pingInterval / 4,
		RandomizationFactor: backoff.DefaultRandomizationFactor,
		Multiplier:          backoff.DefaultMultiplier,
		MaxInterval:         pingInterval / 2,
		MaxElapsedTime:      maxElapseTime,
		Clock:               backoff.SystemClock,
	})
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

func poolCheck(ctx context.Context, idle, max, lower int, check func(context.Context) error) error {
	ratioFree := idle / max * 100

	if ratioFree == 0 {
		return ErrPoolNotReady
	}

	if ratioFree > lower {
		return check(ctx)
	}

	// NOTE: don't have enough free connections
	// that will be useful for processing incoming requests.
	return nil
}
