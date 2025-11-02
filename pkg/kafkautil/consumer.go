package kafkautil

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ConsumerConfig holds Kafka consumer settings.
type ConsumerConfig struct {
	// Brokers is a list of Kafka broker addresses.
	Brokers []string `mapstructure:"brokers"`

	// Topics to subscribe to.
	Topics []string `mapstructure:"topics"`

	// GroupID is the consumer group identifier.
	//
	// All consumers with the same GroupID share the partitions of a topic.
	// Kafka ensures each partition is assigned to exactly one consumer
	// in the group — this is how we achieve parallel processing.
	//
	// Example: topic "raw-events" has 4 partitions, 2 processor instances
	// with GroupID "event-processor" → each gets 2 partitions.
	GroupID string `mapstructure:"group_id"`
}

// NewConsumer creates a Kafka consumer client.
//
// The consumer automatically:
//   - Joins the consumer group
//   - Gets assigned partitions
//   - Tracks offsets (resumes after restart)
//   - Rebalances when group members join/leave
//
// Call Close() during graceful shutdown to commit final offsets
// and leave the group cleanly.
func NewConsumer(cfg ConsumerConfig, opts ...kgo.Opt) (*kgo.Client, error) {
	if len(cfg.Brokers) == 0 {
		return nil, fmt.Errorf("kafkautil: at least one broker address is required")
	}
	if cfg.GroupID == "" {
		return nil, fmt.Errorf("kafkautil: consumer group ID is required")
	}

	allOpts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.GroupID),
		kgo.ConsumeTopics(cfg.Topics...),

		// Start from the beginning of the topic if no committed offset exists.
		// This means a new consumer group will process all existing events.
		// For our use case this is correct — we don't want to miss events.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	}

	allOpts = append(allOpts, opts...)

	client, err := kgo.NewClient(allOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating kafka consumer: %w", err)
	}

	return client, nil
}

// ConsumeLoop reads messages from Kafka and calls handler for each one.
//
// It blocks until the context is cancelled (graceful shutdown).
// After processing each batch of messages, offsets are committed
// automatically by franz-go.
//
// The handler receives:
//   - topic: which topic the message came from
//   - key: message key (used for partitioning)
//   - value: message payload (our Protobuf-encoded events)
//   - headers: metadata (trace IDs for distributed tracing)
//
// If handler returns an error, it is logged but consumption continues.
// Dead letter queue handling should be implemented by the caller.
func ConsumeLoop(ctx context.Context, client *kgo.Client, handler func(topic string, key, value []byte, headers map[string]string) error) error {
	for {
		// PollFetches blocks until messages are available or context is cancelled.
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			return ctx.Err() // Graceful shutdown.
		}

		// Check for errors (broker disconnects, auth failures, etc.).
		if errs := fetches.Errors(); len(errs) > 0 {
			// Return the first error — the caller decides whether to retry.
			return fmt.Errorf("kafka fetch error: %w", errs[0].Err)
		}

		// Process each message.
		fetches.EachRecord(func(record *kgo.Record) {
			headers := make(map[string]string, len(record.Headers))
			for _, h := range record.Headers {
				headers[h.Key] = string(h.Value)
			}

			if err := handler(record.Topic, record.Key, record.Value, headers); err != nil {
				// Log error but continue — one bad message shouldn't stop the consumer.
				// In production, failed messages should go to a DLQ (dead letter queue).
				_ = err // Will be replaced with proper logging in Phase 2.
			}
		})

		// franz-go auto-commits offsets after PollFetches returns.
		// This means "at-least-once" delivery: if the process crashes
		// between PollFetches and commit, messages will be re-delivered.
		// Our deduplication layer (Redis SET NX) handles this.
	}
}
