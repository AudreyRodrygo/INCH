package agent

import (
	"context"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
)

// Source is an interface for event collection backends.
//
// Each source (log tailer, process watcher, network monitor) implements
// this interface. The agent starts all sources and merges their events
// into a single stream for batching.
//
// Sources must:
//   - Write events to the provided channel
//   - Stop cleanly when the context is cancelled
//   - Not block if the channel is full (drop or buffer internally)
type Source interface {
	// Start begins collecting events and writing them to the channel.
	// Blocks until the context is cancelled.
	Start(ctx context.Context, events chan<- *pb.SecurityEvent) error

	// Name returns the source identifier for logging.
	Name() string
}
