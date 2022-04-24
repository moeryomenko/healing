package healing

import (
	"context"
	"time"

	backoff "github.com/cenkalti/backoff/v4"
)

// CheckHelper is helper function for check liveness and readiness.
func CheckHelper(ctx context.Context, pingInterval time.Duration, check func() error) CheckResult {
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
		return CheckResult{
			Err:     err,
			Details: err.Error(),
		}
	}

	return CheckResult{
		Details: "OK",
	}
}
