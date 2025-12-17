// Package gateway implements the Herald gateway-api service.
//
// The gateway accepts notification requests via REST and gRPC,
// prioritizes them, and publishes to NATS for the delivery worker.
//
// Key features:
//   - Priority Queue with SLA enforcement (CRITICAL <1s, HIGH <10s)
//   - Per-client rate limiting (token bucket)
//   - Smart deduplication (content fingerprint + TTL window)
//   - Template rendering (Go templates stored in PostgreSQL)
package gateway

import (
	"github.com/AudreyRodrygo/Sentinel/pkg/natsutil"
	"github.com/AudreyRodrygo/Sentinel/pkg/postgres"
)

// Config holds gateway-api configuration.
type Config struct {
	HTTPPort    int    `mapstructure:"http_port"`
	MetricsPort int    `mapstructure:"metrics_port"`
	LogLevel    string `mapstructure:"log_level"`
	Development bool   `mapstructure:"development"`

	Postgres     postgres.Config `mapstructure:"postgres"`
	NATS         natsutil.Config `mapstructure:"nats"`
	OTLPEndpoint string          `mapstructure:"otlp_endpoint"`
}

// Defaults returns development defaults.
func Defaults() Config {
	return Config{
		HTTPPort:    8090,
		MetricsPort: 8091,
		LogLevel:    "info",
		Development: true,
		Postgres: postgres.Config{
			Host:     "localhost",
			Port:     5432,
			Database: "herald",
			User:     "sentinel",
			Password: "sentinel",
			MaxConns: 10,
		},
		NATS: natsutil.Config{
			URL: "nats://localhost:4222",
		},
	}
}
