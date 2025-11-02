package dlq_test

import (
	"context"
	"testing"

	"github.com/AudreyRodrygo/Sentinel/pkg/dlq"
)

func TestMemory_PushAndLen(t *testing.T) {
	queue := dlq.NewMemory(10)

	if queue.Len() != 0 {
		t.Fatalf("new queue Len() = %d, want 0", queue.Len())
	}

	err := queue.Push(context.Background(), dlq.Message{
		OriginalTopic: "raw-events",
		Value:         []byte("test message"),
		Error:         "parse error",
		Attempts:      3,
	})
	if err != nil {
		t.Fatalf("Push error: %v", err)
	}

	if queue.Len() != 1 {
		t.Errorf("Len() = %d, want 1", queue.Len())
	}
}

func TestMemory_DropsOldestWhenFull(t *testing.T) {
	queue := dlq.NewMemory(3)

	// Fill the queue.
	for i := range 3 {
		_ = queue.Push(context.Background(), dlq.Message{
			Error: string(rune('A' + i)), // "A", "B", "C"
		})
	}

	// Push one more — should drop "A" (oldest).
	_ = queue.Push(context.Background(), dlq.Message{Error: "D"})

	if queue.Len() != 3 {
		t.Fatalf("Len() = %d, want 3 (capacity)", queue.Len())
	}

	messages := queue.Messages()
	if messages[0].Error != "B" {
		t.Errorf("oldest message = %q, want %q (A should be dropped)", messages[0].Error, "B")
	}
	if messages[2].Error != "D" {
		t.Errorf("newest message = %q, want %q", messages[2].Error, "D")
	}
}

func TestMemory_Drain(t *testing.T) {
	queue := dlq.NewMemory(10)

	_ = queue.Push(context.Background(), dlq.Message{Error: "err1"})
	_ = queue.Push(context.Background(), dlq.Message{Error: "err2"})

	drained := queue.Drain()

	if len(drained) != 2 {
		t.Errorf("Drain() returned %d messages, want 2", len(drained))
	}
	if queue.Len() != 0 {
		t.Errorf("after Drain(), Len() = %d, want 0", queue.Len())
	}
}

func TestMemory_ImplementsQueueInterface(_ *testing.T) {
	// This test verifies that Memory satisfies the Queue interface.
	// The var _ Queue = (*Memory)(nil) line in dlq.go catches this at
	// compile time, but this test makes the intention explicit.
	var q dlq.Queue = dlq.NewMemory(10)
	_ = q.Push(context.Background(), dlq.Message{})
	_ = q.Len()
}

func TestMemory_SetsFailedAtAutomatically(t *testing.T) {
	queue := dlq.NewMemory(10)

	_ = queue.Push(context.Background(), dlq.Message{Error: "test"})

	messages := queue.Messages()
	if messages[0].FailedAt.IsZero() {
		t.Error("FailedAt should be set automatically when zero")
	}
}

func TestFormatError(t *testing.T) {
	got := dlq.FormatError("raw-events", nil, 3)
	if got == "" {
		t.Error("FormatError returned empty string")
	}
}
