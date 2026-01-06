// Package dispatcher implements the notification-dispatcher service.
//
// It consumes alerts from NATS JetStream and delivers them through
// multiple channels (Webhook, Email, WebSocket). Each channel has
// independent retry logic, circuit breaker, and delivery tracking.
//
// Flow:
//
//	NATS "alerts.dispatch" ──▶ [Dispatcher] ──▶ Webhook
//	                                         ──▶ Email
//	                                         ──▶ WebSocket
//	                                         ──▶ DLQ (on failure)
package dispatcher

import (
	"github.com/AudreyRodrygo/Inch/pkg/natsutil"
)

// Config holds notification-dispatcher configuration.
type Config struct {
	MetricsPort int    `mapstructure:"metrics_port"`
	LogLevel    string `mapstructure:"log_level"`
	Development bool   `mapstructure:"development"`

	NATS natsutil.Config `mapstructure:"nats"`

	// Webhook delivery configuration.
	Webhook WebhookConfig `mapstructure:"webhook"`

	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
}

// WebhookConfig holds webhook channel settings.
type WebhookConfig struct {
	// URL is the webhook endpoint to POST alerts to.
	URL string `mapstructure:"url"`

	// Secret is the HMAC-SHA256 signing key for webhook payloads.
	// The receiver can verify authenticity by checking the signature header.
	Secret string `mapstructure:"secret"`
}

// Defaults returns development defaults.
func Defaults() Config {
	return Config{
		MetricsPort: 8084,
		LogLevel:    "info",
		Development: true,
		NATS: natsutil.Config{
			URL: "nats://localhost:4222",
		},
	}
}
