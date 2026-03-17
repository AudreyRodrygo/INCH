package collector

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
)

// Server implements the CollectorService gRPC interface.
//
// It is the first service in the INCH pipeline:
//
//	Agent → [Server.IngestEvents] → Redis dedup → Kafka publish
//
// The server is stateless — all state lives in Redis (dedup) and Kafka (events).
// This means we can scale horizontally: multiple collector instances behind
// a load balancer, all writing to the same Kafka topic.
type Server struct {
	pb.UnimplementedCollectorServiceServer

	dedup     *Deduplicator
	publisher *Publisher
	logger    *zap.Logger
}

// NewServer creates a collector gRPC server.
//
// Dependencies are injected through the constructor — this is the standard
// Go pattern for dependency injection (no frameworks needed).
func NewServer(dedup *Deduplicator, publisher *Publisher, logger *zap.Logger) *Server {
	return &Server{
		dedup:     dedup,
		publisher: publisher,
		logger:    logger,
	}
}

// IngestEvents processes a batch of security events from an agent.
//
// For each event in the batch:
//  1. Check for duplicate (Redis SET NX)
//  2. If new: serialize and publish to Kafka
//  3. If duplicate: skip and count as rejected
//
// Returns counts of accepted and rejected events.
func (s *Server) IngestEvents(ctx context.Context, req *pb.IngestEventsRequest) (*pb.IngestEventsResponse, error) {
	if req.Batch == nil || len(req.Batch.Events) == 0 {
		return &pb.IngestEventsResponse{}, nil
	}

	var accepted, rejected uint32
	var rejectedEvents []*pb.RejectedEvent

	for _, event := range req.Batch.Events {
		// Step 1: Deduplication check.
		if event.EventId == "" {
			rejected++
			rejectedEvents = append(rejectedEvents, &pb.RejectedEvent{
				EventId: event.EventId,
				Reason:  "empty event_id",
			})
			continue
		}

		isDup, err := s.dedup.IsDuplicate(ctx, event.EventId)
		if err != nil {
			// Redis error — log and accept the event anyway.
			// It's better to process a potential duplicate than to lose an event.
			s.logger.Warn("dedup check failed, accepting event",
				zap.String("event_id", event.EventId),
				zap.Error(err),
			)
		} else if isDup {
			rejected++
			rejectedEvents = append(rejectedEvents, &pb.RejectedEvent{
				EventId: event.EventId,
				Reason:  "duplicate",
			})
			continue
		}

		// Step 2: Serialize and publish to Kafka.
		data, err := proto.Marshal(event)
		if err != nil {
			s.logger.Error("failed to marshal event",
				zap.String("event_id", event.EventId),
				zap.Error(err),
			)
			rejected++
			rejectedEvents = append(rejectedEvents, &pb.RejectedEvent{
				EventId: event.EventId,
				Reason:  fmt.Sprintf("marshal error: %v", err),
			})
			continue
		}

		// Use hostname as Kafka key → events from same host go to same partition.
		// This preserves per-host ordering for sequence correlation.
		key := []byte(event.Hostname)

		if err := s.publisher.Publish(ctx, key, data); err != nil {
			s.logger.Error("failed to publish event",
				zap.String("event_id", event.EventId),
				zap.Error(err),
			)
			rejected++
			rejectedEvents = append(rejectedEvents, &pb.RejectedEvent{
				EventId: event.EventId,
				Reason:  fmt.Sprintf("publish error: %v", err),
			})
			continue
		}

		accepted++
	}

	s.logger.Info("batch processed",
		zap.String("agent_id", req.Batch.AgentId),
		zap.Uint32("accepted", accepted),
		zap.Uint32("rejected", rejected),
	)

	return &pb.IngestEventsResponse{
		Accepted:       accepted,
		Rejected:       rejected,
		RejectedEvents: rejectedEvents,
	}, nil
}
