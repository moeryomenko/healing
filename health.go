package health

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultCheckPeriod   = 3 * time.Second
	defaultHealzEndpoint = "/healthz"
	defaultReadyEndpoint = "/ready"
)

// The checkers must be compatible with this type.
type checkFunc func(context.Context) error

type Health struct {
	liveness    *CheckGroup
	readiness   *CheckGroup
	server      *http.Server
	checkPeriod time.Duration

	healz, ready string
}

func New(opts ...Option) *Health {
	h := &Health{
		liveness:    NewCheckGroup(defaultCheckTimeout),
		readiness:   NewCheckGroup(defaultCheckTimeout),
		checkPeriod: defaultCheckPeriod,
		healz:       defaultHealzEndpoint,
		ready:       defaultReadyEndpoint,
	}

	for _, opt := range opts {
		opt(h)
	}

	return h
}

type Option func(*Health)

// WithCheckPeriod sets period of launch checks.
func WithCheckPeriod(period time.Duration) Option {
	return func(h *Health) {
		h.checkPeriod = period
	}
}

// WithHealthzEndpoint sets custom endpoint to probe liveness.
func WithHealthzEndpoint(endpoint string) Option {
	return func(h *Health) {
		h.healz = endpoint
	}
}

// WithReadyEndpoint sets custom endpoint to probe readiness.
func WithReadyEndpoint(endpoint string) Option {
	return func(h *Health) {
		h.ready = endpoint
	}
}

// WithLivenessTimeout sets custom timeout for check liveness.
func WithLivenessTimeout(timeout time.Duration) Option {
	return func(h *Health) {
		h.liveness = NewCheckGroup(timeout)
	}
}

// WithReadinessTimeout sets custom timeout for check readiness.
func WithReadinessTimeout(timeout time.Duration) Option {
	return func(h *Health) {
		h.readiness = NewCheckGroup(timeout)
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
	checkTicker := time.NewTicker(h.checkPeriod)
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
	router.HandleFunc(h.healz, func(rw http.ResponseWriter, r *http.Request) {
		if h.liveness.IsOK() {
			return
		}
		rw.WriteHeader(http.StatusServiceUnavailable)
	})
	router.HandleFunc(h.ready, func(rw http.ResponseWriter, r *http.Request) {
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
