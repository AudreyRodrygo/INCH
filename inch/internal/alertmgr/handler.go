package alertmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
	"github.com/AudreyRodrygo/Inch/pkg/circuitbreaker"
	"github.com/AudreyRodrygo/Inch/pkg/ratelimiter"
)

// Handler processes alerts from Kafka and publishes to NATS.
//
// It composes three pkg/ libraries to create resilient alert delivery:
//   - Deduplicator: suppresses repeated alerts within a time window
//   - Rate limiter: prevents alert storms (token bucket)
//   - Circuit breaker: protects NATS from cascading failures
type Handler struct {
	js      jetstream.JetStream
	subject string
	breaker *circuitbreaker.Breaker
	limiter *ratelimiter.Limiter
	dedup   *alertDedup
	logger  *zap.Logger
}

// NewHandler creates an alert handler.
func NewHandler(
	js jetstream.JetStream,
	subject string,
	suppressWindow time.Duration,
	rateLimitPerSec float64,
	logger *zap.Logger,
) *Handler {
	return &Handler{
		js:      js,
		subject: subject,
		breaker: circuitbreaker.New(circuitbreaker.Config{
			Threshold: 5,
			Timeout:   30 * time.Second,
		}),
		limiter: ratelimiter.New(rateLimitPerSec, int64(rateLimitPerSec)*2),
		dedup:   newAlertDedup(suppressWindow),
		logger:  logger,
	}
}

// Handle processes a single alert from Kafka.
//
// Pipeline: deserialize → dedup → rate limit → circuit breaker → NATS publish.
// Each step can reject the alert, preventing unnecessary downstream work.
func (h *Handler) Handle(ctx context.Context, value []byte) error {
	// 1. Deserialize the Protobuf alert.
	var alert pb.Alert
	if err := proto.Unmarshal(value, &alert); err != nil {
		return fmt.Errorf("unmarshaling alert: %w", err)
	}

	// 2. Deduplication: suppress repeated alerts for same rule+group.
	dedupKey := fmt.Sprintf("%s|%s", alert.RuleId, alert.GroupValues["group_key"])
	if h.dedup.isDuplicate(dedupKey) {
		h.logger.Debug("alert suppressed (dedup)",
			zap.String("rule", alert.RuleId),
			zap.String("key", dedupKey),
		)
		return nil
	}

	// 3. Rate limiting: prevent alert storms.
	if !h.limiter.Allow() {
		h.logger.Warn("alert rate-limited",
			zap.String("rule", alert.RuleId),
		)
		return nil
	}

	// 4. Publish to NATS through circuit breaker.
	err := h.breaker.Execute(func() error {
		return h.publishToNATS(ctx, &alert)
	})
	if err != nil {
		return fmt.Errorf("publishing alert to NATS: %w", err)
	}

	h.logger.Info("alert dispatched",
		zap.String("alert_id", alert.AlertId),
		zap.String("rule", alert.RuleId),
		zap.String("severity", alert.Severity.String()),
	)

	return nil
}

// publishToNATS serializes the alert as JSON and publishes to JetStream.
//
// JSON (not Protobuf) is used for the NATS message because the notification
// dispatcher and channels (email, Telegram) need human-readable data
// for template rendering.
func (h *Handler) publishToNATS(ctx context.Context, alert *pb.Alert) error {
	// Convert to a JSON-friendly structure for notification templates.
	payload := map[string]any{
		"alert_id":    alert.AlertId,
		"rule_id":     alert.RuleId,
		"rule_name":   alert.RuleName,
		"severity":    alert.Severity.String(),
		"description": alert.Description,
		"event_count": alert.EventCount,
		"tags":        alert.Tags,
		"enrichment":  alert.Enrichment,
		"group":       alert.GroupValues,
		"created_at":  alert.CreatedAt.AsTime().Format(time.RFC3339),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling alert JSON: %w", err)
	}

	_, err = h.js.Publish(ctx, h.subject, data)
	if err != nil {
		return fmt.Errorf("publishing to %s: %w", h.subject, err)
	}

	return nil
}

// Stop releases resources (rate limiter goroutine).
func (h *Handler) Stop() {
	h.limiter.Stop()
}

// alertDedup tracks recently seen alert keys to suppress duplicates.
//
// Simple in-memory implementation with TTL-based expiry.
// For multi-instance deployment, replace with Redis SET NX.
type alertDedup struct {
	mu     sync.Mutex
	seen   map[string]time.Time
	window time.Duration
}

func newAlertDedup(window time.Duration) *alertDedup {
	return &alertDedup{
		seen:   make(map[string]time.Time),
		window: window,
	}
}

// isDuplicate returns true if this key was seen within the suppress window.
func (d *alertDedup) isDuplicate(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Lazy cleanup: remove expired entries.
	for k, ts := range d.seen {
		if now.Sub(ts) > d.window {
			delete(d.seen, k)
		}
	}

	if _, exists := d.seen[key]; exists {
		return true
	}

	d.seen[key] = now
	return false
}
