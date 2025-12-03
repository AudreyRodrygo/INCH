package dispatcher

import (
	"context"
	"time"
)

// AlertPayload is the JSON-decoded alert from NATS.
// Using a map instead of a struct for flexibility — templates can access any field.
type AlertPayload map[string]any

// Severity extracts the severity string from the payload.
func (a AlertPayload) Severity() string {
	if s, ok := a["severity"].(string); ok {
		return s
	}
	return "UNKNOWN"
}

// AlertID extracts the alert ID from the payload.
func (a AlertPayload) AlertID() string {
	if s, ok := a["alert_id"].(string); ok {
		return s
	}
	return ""
}

// Channel is the interface for notification delivery methods.
//
// Each channel (Webhook, Email, Telegram, etc.) implements this interface.
// The dispatcher iterates over all registered channels and calls Send
// for each alert.
//
// Implementations must be safe for concurrent use — multiple dispatcher
// goroutines may call Send simultaneously.
//
// This is the Strategy Pattern: the dispatcher doesn't know HOW to
// send a notification, only that each channel can Send(). Adding a new
// channel requires zero changes to existing code.
type Channel interface {
	// Send delivers an alert through this channel.
	// Returns an error if delivery fails (triggers retry/DLQ).
	Send(ctx context.Context, alert AlertPayload) error

	// Name returns the channel identifier (for logging and metrics).
	Name() string
}

// DeliveryReceipt records the result of a delivery attempt.
type DeliveryReceipt struct {
	AlertID     string
	Channel     string
	Status      string // "sent", "failed", "dlq"
	AttemptedAt time.Time
	Error       string
}
