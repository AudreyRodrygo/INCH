// Package natsutil provides NATS JetStream connection and stream helpers.
//
// NATS is used for the alert delivery pipeline (alert-manager → notification-dispatcher)
// where we need low-latency fan-out rather than Kafka's heavy log retention.
//
// Why NATS instead of Kafka for this path:
//   - Alert delivery is low-volume (tens/sec vs 10k/sec for events)
//   - We need fast fan-out to multiple channels (email, Slack, Telegram)
//   - NATS is operationally simpler (single binary vs Kafka cluster)
//   - Built-in DLQ support via consumer retry policies
//
// JetStream adds persistence to NATS:
//   - Messages survive broker restarts
//   - Consumer ack/nack for at-least-once delivery
//   - Configurable retention (time, count, or size)
//
// Usage:
//
//	conn, js, err := natsutil.Connect(ctx, natsutil.Config{URL: "nats://localhost:4222"})
//	defer conn.Close()
//
//	err = natsutil.EnsureStream(ctx, js, natsutil.StreamConfig{
//	    Name:     "ALERTS",
//	    Subjects: []string{"alerts.>"},
//	})
package natsutil

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// Config holds NATS connection parameters.
type Config struct {
	// URL is the NATS server address (e.g., "nats://localhost:4222").
	URL string `mapstructure:"url"`
}

// Connect establishes a connection to NATS and returns a JetStream context.
//
// The connection automatically reconnects if the server is temporarily unavailable.
// This is important for Kubernetes environments where pods restart.
func Connect(_ context.Context, cfg Config) (*nats.Conn, jetstream.JetStream, error) {
	if cfg.URL == "" {
		cfg.URL = nats.DefaultURL // "nats://localhost:4222"
	}

	conn, err := nats.Connect(cfg.URL,
		// Reconnect automatically if the connection drops.
		nats.MaxReconnects(-1), // Retry forever.
		nats.ReconnectWait(1*time.Second),

		// Give the server time to start (useful in docker-compose).
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("connecting to NATS at %s: %w", cfg.URL, err)
	}

	// Create a JetStream context for persistent messaging.
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("creating JetStream context: %w", err)
	}

	return conn, js, nil
}

// StreamConfig defines a JetStream stream.
type StreamConfig struct {
	// Name is the stream identifier (e.g., "ALERTS").
	Name string

	// Subjects are the NATS subjects this stream captures.
	// Supports wildcards: "alerts.>" captures "alerts.dispatch", "alerts.dlq", etc.
	Subjects []string

	// MaxAge is how long messages are retained. Default: 24 hours.
	MaxAge time.Duration
}

// EnsureStream creates a JetStream stream if it doesn't exist,
// or updates it if the configuration has changed.
//
// This is idempotent — safe to call on every startup.
// Streams define WHERE messages are stored; consumers define WHO reads them.
func EnsureStream(ctx context.Context, js jetstream.JetStream, cfg StreamConfig) error {
	maxAge := cfg.MaxAge
	if maxAge == 0 {
		maxAge = 24 * time.Hour
	}

	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      cfg.Name,
		Subjects:  cfg.Subjects,
		Retention: jetstream.WorkQueuePolicy, // Messages deleted after ack.
		MaxAge:    maxAge,
	})
	if err != nil {
		return fmt.Errorf("creating stream %s: %w", cfg.Name, err)
	}

	return nil
}
