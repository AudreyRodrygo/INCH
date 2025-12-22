package analytics_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/analytics"
)

func TestRecordSuccess(t *testing.T) {
	s := analytics.New()

	s.RecordSuccess("webhook", 100*time.Millisecond)
	s.RecordSuccess("webhook", 200*time.Millisecond)
	s.RecordSuccess("telegram", 50*time.Millisecond)

	snap := s.Snapshot()

	wh := snap["webhook"]
	if wh.Sent != 2 {
		t.Errorf("webhook sent = %d, want 2", wh.Sent)
	}
	if wh.AvgMs != 150 {
		t.Errorf("webhook avg latency = %.0f ms, want 150", wh.AvgMs)
	}
	if wh.Rate != 1.0 {
		t.Errorf("webhook success rate = %.2f, want 1.0", wh.Rate)
	}

	tg := snap["telegram"]
	if tg.Sent != 1 {
		t.Errorf("telegram sent = %d, want 1", tg.Sent)
	}
}

func TestRecordFailure(t *testing.T) {
	s := analytics.New()

	s.RecordSuccess("email", 100*time.Millisecond)
	s.RecordFailure("email")
	s.RecordFailure("email")

	snap := s.Snapshot()
	email := snap["email"]

	if email.Sent != 1 {
		t.Errorf("sent = %d, want 1", email.Sent)
	}
	if email.Failed != 2 {
		t.Errorf("failed = %d, want 2", email.Failed)
	}

	// Success rate: 1 / (1+2) = 0.333...
	expectedRate := 1.0 / 3.0
	if email.Rate < expectedRate-0.01 || email.Rate > expectedRate+0.01 {
		t.Errorf("success rate = %.3f, want ~%.3f", email.Rate, expectedRate)
	}
}

func TestHandler_ReturnsJSON(t *testing.T) {
	s := analytics.New()
	s.RecordSuccess("webhook", 50*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics", nil)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := body["channels"]; !ok {
		t.Error("response missing 'channels' field")
	}
	if _, ok := body["uptime_sec"]; !ok {
		t.Error("response missing 'uptime_sec' field")
	}
}
