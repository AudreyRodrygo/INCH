package dispatcher

import (
	"context"

	"go.uber.org/zap"
)

// LogChannel is a simple channel that logs alerts to structured output.
//
// Useful for:
//   - Development (see alerts without setting up external services)
//   - Testing (verify alerts are being generated)
//   - Fallback (always-on channel that never fails)
type LogChannel struct {
	logger *zap.Logger
}

// NewLogChannel creates a logging channel.
func NewLogChannel(logger *zap.Logger) *LogChannel {
	return &LogChannel{logger: logger}
}

// Name returns the channel identifier.
func (l *LogChannel) Name() string { return "log" }

// Send logs the alert at INFO level.
func (l *LogChannel) Send(_ context.Context, alert AlertPayload) error {
	l.logger.Info("ALERT",
		zap.String("alert_id", alert.AlertID()),
		zap.String("severity", alert.Severity()),
		zap.Any("payload", map[string]any(alert)),
	)
	return nil
}
