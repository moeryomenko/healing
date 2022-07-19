package checkers

import (
	"context"
	"errors"
	"time"

	"github.com/moeryomenko/healing"
	"github.com/streadway/amqp"
)

func AQMPProber(conn *amqp.Connection, heartbeatPeriod time.Duration) (
	liveness, readiness func(context.Context) healing.CheckResult,
) {
	blocked := make(chan amqp.Blocking, 0)
	blocked = conn.NotifyBlocked(blocked)

	closed := make(chan *amqp.Error, 0)
	closed = conn.NotifyClose(closed)

	return func(ctx context.Context) healing.CheckResult {
			return CheckHelper(func() error {
				select {
				case c := <-closed:
					close(closed)
					closed = make(chan *amqp.Error, 0)
					closed = conn.NotifyClose(closed)

					return errors.New(c.Reason)
				default:
				}

				return nil
			})
		}, func(ctx context.Context) healing.CheckResult {
			return CheckHelper(func() error {
				select {
				case b := <-blocked:
					close(blocked)
					blocked = make(chan amqp.Blocking, 0)
					blocked = conn.NotifyBlocked(blocked)

					return errors.New(b.Reason)
				default:
				}

				return nil
			})
		}
}
