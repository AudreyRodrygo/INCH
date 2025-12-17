package channels

import (
	"context"

	"go.uber.org/zap"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/delivery"
)

// Log is a channel that writes notifications to structured logs.
// Always-on fallback channel for development and debugging.
type Log struct {
	logger *zap.Logger
}

// NewLog creates a log delivery channel.
func NewLog(logger *zap.Logger) *Log {
	return &Log{logger: logger}
}

// Name returns the channel identifier.
func (l *Log) Name() string { return "log" }

// Send logs the notification.
func (l *Log) Send(_ context.Context, notif delivery.Notification) error {
	l.logger.Info("NOTIFICATION",
		zap.String("id", notif.ID()),
		zap.String("priority", notif.Priority()),
		zap.String("recipient", notif.Recipient()),
		zap.Any("payload", map[string]any(notif)),
	)
	return nil
}
