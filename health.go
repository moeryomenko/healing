package healing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultCheckPeriod   = 3 * time.Second
	defaultHealzEndpoint = "/live"
	defaultReadyEndpoint = "/ready"
)

// The checkers must be compatible with this type.
type checkFunc func(context.Context) CheckResult

type CheckResult struct {
	Err     error
	Details any
}

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
func (h *Health) AddLiveChecker(subsystem string, check checkFunc) {
	h.liveness.AddChecker(subsystem, check)
}

// AddReadyChecker adds a check routine for `ready` state of your service to the registry.
// Service readiness check only applies to external dependencies, whose state identifies
// the service readiness to accept load.
// see https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#when-should-you-use-a-readiness-probe.
func (h *Health) AddReadyChecker(subsystem string, check checkFunc) {
	h.readiness.AddChecker(subsystem, check)
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
func (h *Health) ListenAndServe(port int) func(context.Context) error {
	router := http.NewServeMux()

	handler := func(checker *CheckGroup) http.HandlerFunc {
		return noCache(func(w http.ResponseWriter, r *http.Request) {
			if !checker.IsOK() {
				w.WriteHeader(http.StatusServiceUnavailable)
			}

			w.Header().Add("Content-Type", "application/json")
			details := checker.GetDetails()
			body, _ := json.Marshal(details)
			w.Write(body)
		})
	}

	router.HandleFunc(h.healz, handler(h.liveness))
	router.HandleFunc(h.ready, handler(h.readiness))
	h.server = &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: router}

	return func(_ context.Context) error {
		return h.server.ListenAndServe()
	}
}

// Shutdown shutdowns health controller http server.
func (h *Health) Shutdown(ctx context.Context) error {
	if err := h.server.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Ported from Goji's middleware, source:
// https://github.com/zenazn/goji/tree/master/web/middleware

// Taken from https://github.com/mytrile/nocache
var noCacheHeaders = map[string]string{
	"Expires":         time.Unix(0, 0).Format(time.RFC1123),
	"Cache-Control":   "no-cache, no-store, no-transform, must-revalidate, private, max-age=0",
	"Pragma":          "no-cache",
	"X-Accel-Expires": "0",
}

var etagHeaders = []string{
	"ETag",
	"If-Modified-Since",
	"If-Match",
	"If-None-Match",
	"If-Range",
	"If-Unmodified-Since",
}

// noCache is a simple piece of middleware that sets a number of HTTP headers to prevent
// a router (or subrouter) from being cached by an upstream proxy and/or client.
//
// As per http://wiki.nginx.org/HttpProxyModule - NoCache sets:
//      Expires: Thu, 01 Jan 1970 00:00:00 UTC
//      Cache-Control: no-cache, private, max-age=0
//      X-Accel-Expires: 0
//      Pragma: no-cache (for HTTP/1.0 proxies/clients)
func noCache(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		// Delete any ETag headers that may have been set
		for _, v := range etagHeaders {
			if r.Header.Get(v) != "" {
				r.Header.Del(v)
			}
		}

		// Set our NoCache headers
		for k, v := range noCacheHeaders {
			w.Header().Set(k, v)
		}

		h.ServeHTTP(w, r)
	}
}
