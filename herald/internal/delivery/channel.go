// Package delivery implements the Herald delivery-worker service.
//
// The worker reads from NATS JetStream and delivers notifications
// through multiple channels (Email, Webhook, Telegram, Slack).
//
// Each channel is independent: failures in Telegram don't affect Email.
// Failed deliveries go to a Dead Letter Queue for later review.
package delivery

import (
	"context"
)

// Notification is the JSON payload received from the queue.
type Notification map[string]any

// ID extracts the notification ID.
func (n Notification) ID() string {
	if s, ok := n["notification_id"].(string); ok {
		return s
	}
	return ""
}

// Priority extracts the priority string.
func (n Notification) Priority() string {
	if s, ok := n["priority"].(string); ok {
		return s
	}
	return "LOW"
}

// Recipient extracts the recipient.
func (n Notification) Recipient() string {
	if s, ok := n["recipient"].(string); ok {
		return s
	}
	return ""
}

// Channel is the interface for notification delivery methods.
//
// Same pattern as Sentinel dispatcher — Strategy Pattern.
// Adding a new channel = one new file implementing this interface.
type Channel interface {
	Send(ctx context.Context, notification Notification) error
	Name() string
}
