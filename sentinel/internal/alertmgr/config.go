// Package alertmgr implements the alert-manager service.
//
// The alert manager sits between the event processor (Kafka) and the
// notification dispatcher (NATS). It provides:
//
//   - Alert deduplication: suppress repeated alerts for the same rule+group
//   - Rate limiting: prevent alert storms from overwhelming channels
//   - Circuit breaker: protect NATS from cascading failures
//   - Routing: direct alerts to appropriate channels based on severity
//
// Flow:
//
//	Kafka "alerts" ──▶ [Alert Manager] ──▶ NATS JetStream "alerts.dispatch"
//	                        │
//	                        ├── Dedup (suppress window)
//	                        ├── Rate limit (per-rule)
//	                        ├── Circuit breaker (NATS protection)
//	                        └── Route (severity → channels)
package alertmgr

import (
	"github.com/AudreyRodrygo/Sentinel/pkg/kafkautil"
	"github.com/AudreyRodrygo/Sentinel/pkg/natsutil"
)

// Config holds alert-manager configuration.
type Config struct {
	MetricsPort int    `mapstructure:"metrics_port"`
	LogLevel    string `mapstructure:"log_level"`
	Development bool   `mapstructure:"development"`

	// Kafka consumer for reading alerts from processor.
	Kafka kafkautil.ConsumerConfig `mapstructure:"kafka"`

	// NATS for publishing to dispatcher.
	NATS natsutil.Config `mapstructure:"nats"`

	// SuppressWindowSec is the dedup window: how many seconds to suppress
	// repeated alerts for the same rule+group combination.
	SuppressWindowSec int `mapstructure:"suppress_window_sec"`

	// RateLimit is the maximum alerts per second published to NATS.
	RateLimit float64 `mapstructure:"rate_limit"`

	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
}

// Defaults returns development defaults.
func Defaults() Config {
	return Config{
		MetricsPort: 8083,
		LogLevel:    "info",
		Development: true,
		Kafka: kafkautil.ConsumerConfig{
			Brokers: []string{"localhost:9094"},
			Topics:  []string{"alerts"},
			GroupID: "alert-manager",
		},
		NATS: natsutil.Config{
			URL: "nats://localhost:4222",
		},
		SuppressWindowSec: 300,
		RateLimit:         100,
	}
}
