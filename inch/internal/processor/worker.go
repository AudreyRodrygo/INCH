package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"

	"github.com/google/uuid"
	"github.com/twmb/franz-go/pkg/kgo"

	pb "github.com/AudreyRodrygo/Inch/gen/sentinel/v1"
	"github.com/AudreyRodrygo/Inch/pkg/observability"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor/enrichment"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor/rules"
)

// Pool manages a fixed number of worker goroutines that process events
// from a shared channel.
//
// This is the Worker Pool pattern — a standard Go concurrency pattern:
//
//	                         ┌──▶ Worker 1 ──▶ enrich ──▶ persist
//	Kafka ──▶ [channel] ────┼──▶ Worker 2 ──▶ enrich ──▶ persist
//	                         └──▶ Worker N ──▶ enrich ──▶ persist
//
// Why a fixed pool instead of goroutine-per-event:
//   - Bounded resource usage (memory, DB connections)
//   - Backpressure: when channel is full, Kafka consumer pauses
//   - Predictable performance under load
type Pool struct {
	workCh      chan *pb.SecurityEvent // Buffered channel for backpressure.
	db          *pgxpool.Pool
	enricher    *enrichment.Pipeline
	ruleEngine  *rules.Engine
	alertClient *kgo.Client // Kafka producer for alerts topic.
	alertTopic  string
	logger      *zap.Logger
	count       int // Number of workers.
}

// NewPool creates a worker pool.
func NewPool(count, bufferSize int, db *pgxpool.Pool, enricher *enrichment.Pipeline, ruleEngine *rules.Engine, alertClient *kgo.Client, alertTopic string, logger *zap.Logger) *Pool {
	return &Pool{
		workCh:      make(chan *pb.SecurityEvent, bufferSize),
		db:          db,
		enricher:    enricher,
		ruleEngine:  ruleEngine,
		alertClient: alertClient,
		alertTopic:  alertTopic,
		logger:      logger,
		count:       count,
	}
}

// Submit sends an event to the worker pool for processing.
//
// This is called by the Kafka consumer for each message.
// If the channel is full (workers can't keep up), this blocks —
// which causes the Kafka consumer to pause reading. This is
// intentional backpressure: the system slows down rather than crashes.
func (p *Pool) Submit(ctx context.Context, event *pb.SecurityEvent) error {
	select {
	case p.workCh <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run starts all worker goroutines and blocks until the context is cancelled.
//
// Uses errgroup from golang.org/x/sync — it manages a group of goroutines
// that share a context. If any goroutine returns an error, the context is
// cancelled and all others stop.
func (p *Pool) Run(ctx context.Context) error {
	g, ctx := errgroup.WithContext(ctx)

	// Launch N workers.
	for i := range p.count {
		workerID := i
		g.Go(func() error {
			return p.worker(ctx, workerID)
		})
	}

	// Close the work channel when context is cancelled.
	// This signals workers to drain remaining events and exit.
	g.Go(func() error {
		<-ctx.Done()
		close(p.workCh)
		return nil
	})

	return g.Wait()
}

// worker processes events from the channel until it's closed.
func (p *Pool) worker(ctx context.Context, id int) error {
	p.logger.Info("worker started", zap.Int("worker_id", id))

	for event := range p.workCh {
		if err := p.processEvent(ctx, event); err != nil {
			p.logger.Error("failed to process event",
				zap.Int("worker_id", id),
				zap.String("event_id", event.EventId),
				zap.Error(err),
			)
			// Continue processing — one bad event shouldn't stop the worker.
			continue
		}
	}

	p.logger.Info("worker stopped", zap.Int("worker_id", id))
	return nil
}

// processEvent handles a single event: enrich, classify, persist.
//
// This is where the core business logic lives. Currently a skeleton
// that will be expanded with enrichment and rule engine in Phase 3.
func (p *Pool) processEvent(ctx context.Context, event *pb.SecurityEvent) error {
	tracer := observability.Tracer("processor")
	ctx, span := tracer.Start(ctx, "process-event")
	defer span.End()

	// Enrich the event (GeoIP, threat intel, etc.).
	if err := p.enricher.Enrich(ctx, event); err != nil {
		p.logger.Warn("enrichment partially failed",
			zap.String("event_id", event.EventId),
			zap.Error(err),
		)
		// Continue — partial enrichment is better than no processing.
	}

	// Classify severity based on event type + enrichment data.
	event.Severity = ClassifySeverity(event)

	// Evaluate correlation rules.
	results := p.ruleEngine.Evaluate(event)
	for _, result := range results {
		if alertErr := p.publishAlert(ctx, event, result); alertErr != nil {
			p.logger.Error("failed to publish alert",
				zap.String("rule", result.Rule.ID),
				zap.Error(alertErr),
			)
		}
	}

	// Persist to PostgreSQL.
	if err := p.persistEvent(ctx, event); err != nil {
		return fmt.Errorf("persisting event: %w", err)
	}

	return nil
}

// persistEvent stores an enriched event in PostgreSQL.
func (p *Pool) persistEvent(ctx context.Context, event *pb.SecurityEvent) error {
	// Serialize the full event as JSONB for flexible querying.
	data, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshaling event: %w", err)
	}

	var ts time.Time
	if event.Timestamp != nil {
		ts = event.Timestamp.AsTime()
	} else {
		ts = time.Now()
	}

	_, err = p.db.Exec(ctx, `
		INSERT INTO events (
			event_id, event_type, severity, hostname,
			source_ip, destination_ip, service,
			timestamp, raw_data, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (event_id) DO NOTHING`,
		event.EventId,
		event.Type.String(),
		event.Severity.String(),
		event.Hostname,
		event.SourceIp,
		event.DestinationIp,
		event.Service,
		ts,
		data,
	)
	if err != nil {
		return fmt.Errorf("inserting event: %w", err)
	}

	return nil
}

// publishAlert creates a Protobuf Alert from a rule result and publishes to Kafka.
func (p *Pool) publishAlert(ctx context.Context, event *pb.SecurityEvent, result rules.Result) error {
	if p.alertClient == nil {
		return nil // Alerts publishing not configured.
	}

	alert := &pb.Alert{
		AlertId:    uuid.NewString(),
		RuleId:     result.Rule.ID,
		RuleName:   result.Rule.Name,
		Severity:   result.Rule.SeverityProto(),
		CreatedAt:  event.Timestamp,
		EventCount: uint32(result.EventCount), //nolint:gosec // EventCount is always small positive
		EventIds:   result.EventIDs,
		Tags:       result.Rule.Tags,
		GroupValues: map[string]string{
			"group_key": result.GroupKey,
		},
		Description: fmt.Sprintf("Rule '%s' fired for group '%s' (%d events)",
			result.Rule.Name, result.GroupKey, result.EventCount),
	}

	// Copy event enrichment data to alert.
	if event.Metadata != nil {
		alert.Enrichment = make(map[string]string, len(event.Metadata))
		for k, v := range event.Metadata {
			alert.Enrichment[k] = v
		}
	}

	data, err := proto.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshaling alert: %w", err)
	}

	record := &kgo.Record{
		Topic: p.alertTopic,
		Key:   []byte(result.Rule.ID),
		Value: data,
	}

	results := p.alertClient.ProduceSync(ctx, record)
	return results.FirstErr()
}
