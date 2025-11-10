// Package kafkautil provides Kafka producer and consumer factories using franz-go.
//
// franz-go is a pure-Go Kafka client — no CGO, no librdkafka dependency.
// This makes cross-compilation trivial and Docker images smaller.
//
// In our architecture, Kafka serves as the transport between:
//   - event-collector → event-processor (topic: "raw-events")
//   - event-processor → alert-manager (topic: "alerts")
//
// Kafka retains messages on disk (7 days by default), enabling:
//   - Replay for forensics analysis
//   - Independent consumer groups reading the same data
//   - Decoupling of producer and consumer speeds (backpressure via partitions)
package kafkautil

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ProducerConfig holds Kafka producer settings.
type ProducerConfig struct {
	// Brokers is a list of Kafka broker addresses.
	// At least one is required; the client discovers others automatically.
	Brokers []string `mapstructure:"brokers"`

	// Topic is the default topic to produce to.
	Topic string `mapstructure:"topic"`
}

// NewProducer creates a Kafka producer client.
//
// The producer batches messages internally for throughput.
// Call Close() during graceful shutdown to flush pending messages.
//
// franz-go handles:
//   - Automatic partition assignment (round-robin or key-based)
//   - Connection management and reconnection
//   - Compression (configured via options)
func NewProducer(cfg ProducerConfig, opts ...kgo.Opt) (*kgo.Client, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafkautil: at least one broker address is required")
	}

	allOpts := make([]kgo.Opt, 0, 3+len(opts))
	allOpts = append(allOpts,
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.DefaultProduceTopic(cfg.Topic),

		// RequiredAcks: wait for the leader to acknowledge the write.
		// This is the balance between durability and latency:
		//   - NoAck: fastest, but messages can be lost
		//   - LeaderAck: leader confirms write (our choice)
		//   - AllISRAcks: all replicas confirm (most durable, slowest)
		kgo.RequiredAcks(kgo.LeaderAck()),
	)

	allOpts = append(allOpts, opts...)

	client, err := kgo.NewClient(allOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating kafka producer: %w", err)
	}

	return client, nil
}

// Produce sends a single message to Kafka synchronously.
//
// The key determines which partition receives the message.
// Messages with the same key always go to the same partition,
// preserving order per key (e.g., all events from one host
// go to the same partition and are processed in order).
//
// If key is nil, messages are distributed round-robin across partitions.
func Produce(ctx context.Context, client *kgo.Client, topic string, key, value []byte) error {
	record := &kgo.Record{
		Topic: topic,
		Key:   key,
		Value: value,
	}

	// ProduceSync blocks until the broker acknowledges the write.
	// For higher throughput, use client.Produce() (async) with a callback.
	results := client.ProduceSync(ctx, record)
	return results.FirstErr()
}
