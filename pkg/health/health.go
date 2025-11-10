// Package health provides HTTP health check endpoints for Kubernetes liveness and readiness probes.
//
// Liveness (/healthz): Always returns 200 if the process is running.
// Kubernetes uses this to decide whether to restart the pod.
//
// Readiness (/readyz): Returns 200 only when the service is ready to accept traffic.
// Kubernetes uses this to include/exclude the pod from load balancing.
//
// A service starts as NOT ready and explicitly marks itself ready after connecting
// to all dependencies (database, message broker, etc.).
//
// Usage:
//
//	checker := health.New()
//	go checker.ListenAndServe(":8081")  // Start health check server
//
//	// ... connect to dependencies ...
//	checker.SetReady(true)  // Now Kubernetes will send traffic
//
//	// During graceful shutdown:
//	checker.SetReady(false)  // Stop receiving new traffic
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Checker manages health check state and serves HTTP endpoints.
type Checker struct {
	mu    sync.RWMutex // Protects the ready flag from concurrent access.
	ready bool         // Whether the service is ready to accept traffic.
}

// New creates a Checker that starts in a not-ready state.
// Call SetReady(true) after all dependencies are connected.
func New() *Checker {
	return &Checker{ready: false}
}

// SetReady updates the readiness state. This is safe to call from any goroutine.
//
// Typical usage:
//   - SetReady(true) after successful startup
//   - SetReady(false) at the beginning of graceful shutdown
func (c *Checker) SetReady(ready bool) {
	c.mu.Lock()
	c.ready = ready
	c.mu.Unlock()
}

// IsReady reports whether the service is currently ready.
func (c *Checker) IsReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ready
}

// Handler returns an http.Handler that serves both /healthz and /readyz.
// Mount this on a separate port from your main API so health checks
// don't interfere with application routing or middleware.
func (c *Checker) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", c.handleLiveness)
	mux.HandleFunc("GET /readyz", c.handleReadiness)
	return mux
}

// ListenAndServe starts an HTTP server for health checks on the given address.
// It blocks until the context is cancelled, then shuts down gracefully.
//
// Example:
//
//	checker := health.New()
//	go checker.ListenAndServe(ctx, ":8081")
func (c *Checker) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           c.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Shut down when context is cancelled.
	go func() { //nolint:gosec // G118: intentionally using Background for shutdown after parent cancel
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:gosec // G118: need fresh context for shutdown after parent cancellation
		defer cancel()

		_ = srv.Shutdown(shutdownCtx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// response is the JSON body returned by health check endpoints.
type response struct {
	Status string `json:"status"`
}

// handleLiveness always returns 200 OK — if the process is running, it's alive.
func (c *Checker) handleLiveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, response{Status: "alive"})
}

// handleReadiness returns 200 if ready, 503 Service Unavailable if not.
func (c *Checker) handleReadiness(w http.ResponseWriter, _ *http.Request) {
	if c.IsReady() {
		writeJSON(w, http.StatusOK, response{Status: "ready"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, response{Status: "not ready"})
}

// writeJSON encodes v as JSON and writes it to w with the given HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
