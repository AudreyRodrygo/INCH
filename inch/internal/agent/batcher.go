package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/AudreyRodrygo/Inch/gen/sentinel/v1"
)

// Batcher accumulates events and sends them in batches to the collector.
//
// Batching reduces gRPC overhead: instead of one RPC per event (10k RPCs/sec),
// we send one RPC per 100 events (100 RPCs/sec). Each RPC carries a batch,
// amortizing the cost of connection setup, serialization, and network round-trip.
//
// Flush triggers:
//   - Batch is full (batch_size events accumulated)
//   - Timer fires (flush_interval elapsed since last send)
//   - Context cancelled (graceful shutdown — flush remaining)
//
// The two triggers ensure both high-throughput and low-latency delivery:
//   - Under load: batches fill up quickly and flush by size
//   - Under low load: timer ensures events aren't stuck waiting for a full batch
type Batcher struct {
	client   pb.CollectorServiceClient
	conn     *grpc.ClientConn
	agentID  string
	batch    []*pb.SecurityEvent
	batchCap int
	interval time.Duration
	sequence atomic.Uint64
	logger   *zap.Logger
}

// NewBatcher creates an event batcher with a gRPC connection to the collector.
func NewBatcher(collectorAddr, agentID string, batchSize, flushIntervalSec int, logger *zap.Logger) (*Batcher, error) {
	// Connect to collector. In production, use TLS + mTLS.
	conn, err := grpc.NewClient(collectorAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to collector at %s: %w", collectorAddr, err)
	}

	if batchSize <= 0 {
		batchSize = 100
	}
	if flushIntervalSec <= 0 {
		flushIntervalSec = 5
	}

	return &Batcher{
		client:   pb.NewCollectorServiceClient(conn),
		conn:     conn,
		agentID:  agentID,
		batch:    make([]*pb.SecurityEvent, 0, batchSize),
		batchCap: batchSize,
		interval: time.Duration(flushIntervalSec) * time.Second,
		logger:   logger,
	}, nil
}

// Run reads events from the channel and sends batches to the collector.
// Blocks until the context is cancelled, then flushes remaining events.
func (b *Batcher) Run(ctx context.Context, events <-chan *pb.SecurityEvent) error {
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				// Channel closed — flush and exit.
				return b.flush(ctx)
			}

			b.batch = append(b.batch, event)
			if len(b.batch) >= b.batchCap {
				if err := b.flush(ctx); err != nil {
					b.logger.Warn("batch flush failed", zap.Error(err))
				}
			}

		case <-ticker.C:
			// Timer flush — send whatever we have.
			if len(b.batch) > 0 {
				if err := b.flush(ctx); err != nil {
					b.logger.Warn("timer flush failed", zap.Error(err))
				}
			}

		case <-ctx.Done():
			// Graceful shutdown — flush remaining events.
			b.logger.Info("flushing remaining events before shutdown",
				zap.Int("pending", len(b.batch)),
			)
			// Use a fresh context with timeout for the final flush.
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second) //nolint:gosec // need fresh context for shutdown flush
			defer cancel()
			return b.flush(flushCtx)
		}
	}
}

// flush sends the current batch to the collector and resets.
func (b *Batcher) flush(ctx context.Context) error {
	if len(b.batch) == 0 {
		return nil
	}

	seq := b.sequence.Add(1)

	req := &pb.IngestEventsRequest{
		Batch: &pb.EventBatch{
			Events:   b.batch,
			AgentId:  b.agentID,
			Sequence: seq,
		},
	}

	resp, err := b.client.IngestEvents(ctx, req)
	if err != nil {
		// Don't clear the batch — retry on next flush.
		return fmt.Errorf("sending batch (seq=%d, size=%d): %w", seq, len(b.batch), err)
	}

	b.logger.Info("batch sent",
		zap.Uint64("sequence", seq),
		zap.Uint32("accepted", resp.Accepted),
		zap.Uint32("rejected", resp.Rejected),
	)

	// Clear the batch, reuse the underlying array.
	b.batch = b.batch[:0]
	return nil
}

// Close releases the gRPC connection.
func (b *Batcher) Close() error {
	return b.conn.Close()
}
