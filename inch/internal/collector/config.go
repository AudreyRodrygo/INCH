// Package collector implements the event-collector service.
//
// The collector is the entry point for all security events in Sentinel.
// It receives events from agents via gRPC, validates and deduplicates them,
// then publishes to Kafka for downstream processing.
//
// Flow:
//
//	Agent ──gRPC──▶ Collector ──Kafka──▶ Processor
//	                   │
//	                   ├── Validate (Protobuf schema enforces structure)
//	                   ├── Deduplicate (Redis SET NX with TTL)
//	                   └── Publish (Kafka topic "raw-events")
package collector

import (
	"github.com/AudreyRodrygo/Inch/pkg/kafkautil"
	"github.com/AudreyRodrygo/Inch/pkg/redisutil"
)

// Config holds all event-collector configuration.
type Config struct {
	// GRPCPort is the port for the gRPC server.
	GRPCPort int `mapstructure:"grpc_port"`

	// MetricsPort is the port for Prometheus metrics and health checks.
	MetricsPort int `mapstructure:"metrics_port"`

	// LogLevel controls logging verbosity (debug, info, warn, error).
	LogLevel string `mapstructure:"log_level"`

	// Development enables human-friendly log output.
	Development bool `mapstructure:"development"`

	// Kafka producer configuration.
	Kafka kafkautil.ProducerConfig `mapstructure:"kafka"`

	// Redis configuration for deduplication.
	Redis redisutil.Config `mapstructure:"redis"`

	// DeduplicationTTL is how long to remember event IDs for dedup (seconds).
	// Default: 300 (5 minutes).
	DeduplicationTTL int `mapstructure:"deduplication_ttl"`

	// OTLPEndpoint for distributed tracing (e.g., "localhost:4317").
	// Empty disables tracing.
	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
}

// Defaults returns a Config with sensible development defaults.
func Defaults() Config {
	return Config{
		GRPCPort:    50051,
		MetricsPort: 8081,
		LogLevel:    "info",
		Development: true,
		Kafka: kafkautil.ProducerConfig{
			Brokers: []string{"localhost:9094"},
			Topic:   "raw-events",
		},
		Redis: redisutil.Config{
			Addr: "localhost:6379",
		},
		DeduplicationTTL: 300,
	}
}
