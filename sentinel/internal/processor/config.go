// Package processor implements the event-processor service.
//
// The processor is the brain of Sentinel — it reads raw events from Kafka,
// enriches them (GeoIP, threat intel), evaluates correlation rules, classifies
// severity, and persists the results to PostgreSQL.
//
// Architecture:
//
//	Kafka (raw-events) ──▶ [Worker Pool] ──▶ Enrichment ──▶ Rule Engine ──▶ PostgreSQL
//	                                                            │
//	                                                            ▼
//	                                                     Kafka (alerts)
package processor

import (
	"github.com/AudreyRodrygo/Sentinel/pkg/kafkautil"
	"github.com/AudreyRodrygo/Sentinel/pkg/postgres"
)

// Config holds all event-processor configuration.
type Config struct {
	MetricsPort int    `mapstructure:"metrics_port"`
	LogLevel    string `mapstructure:"log_level"`
	Development bool   `mapstructure:"development"`

	// WorkerCount is the number of concurrent event processing goroutines.
	// More workers = higher throughput, but more DB connections.
	// Rule of thumb: 2x CPU cores, max = Postgres MaxConns.
	WorkerCount int `mapstructure:"worker_count"`

	// ChannelBuffer is the size of the work channel between Kafka consumer
	// and workers. Larger buffer absorbs traffic spikes but uses more memory.
	ChannelBuffer int `mapstructure:"channel_buffer"`

	// Kafka consumer configuration.
	Kafka kafkautil.ConsumerConfig `mapstructure:"kafka"`

	// KafkaAlerts is the producer config for publishing alerts.
	KafkaAlerts kafkautil.ProducerConfig `mapstructure:"kafka_alerts"`

	// Postgres connection configuration.
	Postgres postgres.Config `mapstructure:"postgres"`

	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
}

// Defaults returns a Config with sensible development defaults.
func Defaults() Config {
	return Config{
		MetricsPort:   8082,
		LogLevel:      "info",
		Development:   true,
		WorkerCount:   4,
		ChannelBuffer: 1000,
		Kafka: kafkautil.ConsumerConfig{
			Brokers: []string{"localhost:9094"},
			Topics:  []string{"raw-events"},
			GroupID: "event-processor",
		},
		KafkaAlerts: kafkautil.ProducerConfig{
			Brokers: []string{"localhost:9094"},
			Topic:   "alerts",
		},
		Postgres: postgres.Config{
			Host:     "localhost",
			Port:     5432,
			Database: "sentinel",
			User:     "sentinel",
			Password: "sentinel",
			MaxConns: 10,
		},
	}
}
