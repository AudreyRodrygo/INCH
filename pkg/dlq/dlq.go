// Package dlq provides a Dead Letter Queue abstraction for messages that
// cannot be processed after all retry attempts are exhausted.
//
// A DLQ is essential for reliable message processing:
//   - Without DLQ: a poison message blocks the entire queue or is silently lost
//   - With DLQ: the message is safely stored for later inspection and replay
//
// This package defines the interface (contract) that both Sentinel and Herald
// use. The actual implementation depends on the underlying transport:
//   - NATS JetStream: for alert delivery pipeline
//   - Kafka: for event processing pipeline
//   - In-memory: for testing
//
// This is a key Go design principle: "Accept interfaces, return structs."
// The consumer (alert-manager, dispatcher) depends on the DLQ interface,
// not a specific implementation. The main() function wires the concrete
// implementation at startup.
//
// Usage:
//
//	var queue dlq.Queue = dlq.NewMemory(1000) // or dlq.NewNATS(js, "alerts.dlq")
//
//	// In the processing loop:
//	if err := process(msg); err != nil {
//	    queue.Push(ctx, dlq.Message{...})
//	}
package dlq

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Message represents a message that failed processing.
type Message struct {
	// OriginalTopic is where the message was originally consumed from.
	OriginalTopic string

	// Key is the message key (for partitioning context).
	Key []byte

	// Value is the message payload.
	Value []byte

	// Error describes why processing failed.
	Error string

	// Attempts is how many times processing was attempted.
	Attempts int

	// FailedAt records when the message was moved to DLQ.
	FailedAt time.Time

	// Headers carries metadata (trace IDs, etc.).
	Headers map[string]string
}

// Queue is the interface for Dead Letter Queue operations.
//
// Implementations must be safe for concurrent use.
type Queue interface {
	// Push adds a failed message to the dead letter queue.
	Push(ctx context.Context, msg Message) error

	// Len returns the number of messages currently in the queue.
	// Useful for monitoring (Prometheus gauge).
	Len() int
}

// Memory is an in-memory DLQ implementation, primarily for testing
// and development. In production, use a persistent implementation
// (NATS JetStream or Kafka).
//
// It has a bounded capacity — when full, oldest messages are dropped.
type Memory struct {
	mu       sync.Mutex
	messages []Message
	capacity int
}

// NewMemory creates an in-memory DLQ with the given capacity.
func NewMemory(capacity int) *Memory {
	return &Memory{
		messages: make([]Message, 0, capacity),
		capacity: capacity,
	}
}

// Push adds a message to the in-memory queue.
// If the queue is full, the oldest message is dropped (ring buffer behavior).
func (m *Memory) Push(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msg.FailedAt.IsZero() {
		msg.FailedAt = time.Now()
	}

	if len(m.messages) >= m.capacity {
		// Drop oldest — shift left by 1.
		copy(m.messages, m.messages[1:])
		m.messages = m.messages[:len(m.messages)-1]
	}

	m.messages = append(m.messages, msg)
	return nil
}

// Len returns the number of messages in the queue.
func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

// Messages returns a copy of all messages in the queue.
// Useful for testing and the replay API.
func (m *Memory) Messages() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]Message, len(m.messages))
	copy(result, m.messages)
	return result
}

// Drain removes and returns all messages from the queue.
// Used by the replay API to re-process dead letters.
func (m *Memory) Drain() []Message {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := m.messages
	m.messages = make([]Message, 0, m.capacity)
	return result
}

// Verify that Memory implements Queue at compile time.
// This is a Go idiom: if Memory doesn't implement all Queue methods,
// the compiler will catch it here rather than at runtime.
var _ Queue = (*Memory)(nil)

// FormatError creates a human-readable error description for DLQ messages.
func FormatError(topic string, err error, attempts int) string {
	return fmt.Sprintf("topic=%s attempts=%d error=%v", topic, attempts, err)
}
