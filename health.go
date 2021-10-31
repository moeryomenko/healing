package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const checkPeriod = 3 * time.Second

// The checkers must be compatible with this type.
type checkFunc func(context.Context) error

type Health struct {
	liveness  *CheckGroup
	readiness *CheckGroup
	server    *http.Server
}

func New() *Health {
	return &Health{
		liveness:  NewCheckGroup(),
		readiness: NewCheckGroup(),
	}
}

// AddLiveChecker adds a check routine for `live` state of your service to the registry.
// Service health check only applies to internal components, whose state identifies the service liveness.
// see https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#when-should-you-use-a-liveness-probe.
func (h *Health) AddLiveChecker(check checkFunc) {
	h.liveness.AddChecker(check)
}

// AddReadyChecker adds a check routine for `ready` state of your service to the registry.
// Service readiness check only applies to external dependencies, whose state identifies
// the service readiness to accept load.
// see https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#when-should-you-use-a-readiness-probe.
func (h *Health) AddReadyChecker(check checkFunc) {
	h.readiness.AddChecker(check)
}

// Heartbeat periodically run all checkers for both `live` and `ready` states.
func (h *Health) Heartbeat(ctx context.Context) error {
	checkTicker := time.NewTicker(checkPeriod)
	defer checkTicker.Stop()

	for {
		select {
		case <-checkTicker.C:
			h.liveness.Check(ctx)
			h.readiness.Check(ctx)
		case <-ctx.Done():
			return nil
		}
	}
}

// ListenAndServe provides HTTP listener with results of checks. It
// offers reference HTTP server that offer all in one routes for liveness and readiness.
func (h *Health) ListenAndServe(port int) error {
	router := http.NewServeMux()
	router.HandleFunc("/healthz", func(rw http.ResponseWriter, r *http.Request) {
		if h.liveness.IsOK() {
			return
		}
		rw.WriteHeader(http.StatusServiceUnavailable)
	})
	router.HandleFunc("/ready", func(rw http.ResponseWriter, r *http.Request) {
		if h.readiness.IsOK() {
			return
		}
		rw.WriteHeader(http.StatusServiceUnavailable)
	})
	h.server = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: router}
	return h.server.ListenAndServe()
}

func (h *Health) Shutdown(ctx context.Context) error {
	if err := h.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
