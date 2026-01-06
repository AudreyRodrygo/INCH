package health_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AudreyRodrygo/Inch/pkg/health"
)

// httptest.NewRecorder creates a fake HTTP response writer.
// This lets us test HTTP handlers without starting a real server.
// It's a standard Go testing pattern for HTTP code.

func TestLiveness_AlwaysReturns200(t *testing.T) {
	checker := health.New()
	handler := checker.Handler()

	// Create a fake GET /healthz request.
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	if body["status"] != "alive" {
		t.Errorf("status = %q, want %q", body["status"], "alive")
	}
}

func TestReadiness_NotReadyByDefault(t *testing.T) {
	checker := health.New()
	handler := checker.Handler()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// A new Checker is not ready — should return 503.
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestReadiness_ReturnsOKWhenReady(t *testing.T) {
	checker := health.New()
	checker.SetReady(true) // Simulate: all dependencies connected.
	handler := checker.Handler()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["status"] != "ready" {
		t.Errorf("status = %q, want %q", body["status"], "ready")
	}
}

func TestReadiness_CanToggle(t *testing.T) {
	checker := health.New()
	handler := checker.Handler()

	// Start: not ready.
	assertReadiness(t, handler, http.StatusServiceUnavailable)

	// Mark ready.
	checker.SetReady(true)
	assertReadiness(t, handler, http.StatusOK)

	// Graceful shutdown: mark not ready again.
	checker.SetReady(false)
	assertReadiness(t, handler, http.StatusServiceUnavailable)
}

func TestIsReady(t *testing.T) {
	checker := health.New()

	if checker.IsReady() {
		t.Error("new checker should not be ready")
	}

	checker.SetReady(true)
	if !checker.IsReady() {
		t.Error("checker should be ready after SetReady(true)")
	}
}

// assertReadiness is a test helper that checks the /readyz response code.
func assertReadiness(t *testing.T, handler http.Handler, wantCode int) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != wantCode {
		t.Errorf("readiness status = %d, want %d", rec.Code, wantCode)
	}
}
