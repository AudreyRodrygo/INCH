package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor/enrichment"
	"github.com/AudreyRodrygo/Inch/inch/internal/processor/rules"
	"github.com/AudreyRodrygo/Inch/pkg/config"
	"github.com/AudreyRodrygo/Inch/pkg/health"
	"github.com/AudreyRodrygo/Inch/pkg/kafkautil"
	"github.com/AudreyRodrygo/Inch/pkg/observability"
	"github.com/AudreyRodrygo/Inch/pkg/postgres"
)

const serviceName = "event-processor"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

func run() error {
	// 1. Config.
	cfg := processor.Defaults()
	if err := config.Load("PROCESSOR", "", &cfg); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// 2. Context with shutdown signal.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. Logger.
	logger := observability.MustLogger(serviceName, cfg.LogLevel, cfg.Development)
	defer func() { _ = logger.Sync() }()

	logger.Info("starting service", zap.Int("workers", cfg.WorkerCount))

	// 4. Tracing.
	shutdownTracer, err := observability.InitTracer(ctx, serviceName, cfg.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() { _ = shutdownTracer(context.Background()) }()

	// 5. PostgreSQL.
	db, err := postgres.NewPool(ctx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer db.Close()

	// Run migrations.
	if migrateErr := postgres.Migrate(ctx, cfg.Postgres.DSN(), "inch/migrations"); migrateErr != nil {
		logger.Warn("migrations failed (may already be applied)", zap.Error(migrateErr))
	}

	// 6. Kafka consumer.
	consumer, err := kafkautil.NewConsumer(cfg.Kafka)
	if err != nil {
		return fmt.Errorf("creating kafka consumer: %w", err)
	}
	defer consumer.Close()

	// 7. Enrichment pipeline.
	enrichPipeline := enrichment.NewPipeline(
		enrichment.NewGeoIP(),
		enrichment.NewThreatIntel(),
	)

	// 8. Rule engine.
	ruleList, ruleErr := rules.LoadFromDir("inch/rules")
	if ruleErr != nil {
		logger.Warn("failed to load rules", zap.Error(ruleErr))
	}
	logger.Info("rules loaded", zap.Int("count", len(ruleList)))
	ruleEngine := rules.New(ruleList, logger)

	// 9. Kafka producer for alerts.
	alertProducer, alertErr := kafkautil.NewProducer(cfg.KafkaAlerts)
	if alertErr != nil {
		return fmt.Errorf("creating alert producer: %w", alertErr)
	}
	defer alertProducer.Close()

	// 10. Worker pool.
	pool := processor.NewPool(cfg.WorkerCount, cfg.ChannelBuffer, db, enrichPipeline, ruleEngine, alertProducer, cfg.KafkaAlerts.Topic, logger)

	// 8. Health check.
	checker := health.New()
	go func() {
		if healthErr := checker.ListenAndServe(ctx, fmt.Sprintf(":%d", cfg.MetricsPort)); healthErr != nil {
			logger.Error("health server error", zap.Error(healthErr))
		}
	}()

	// 9. Start workers in background.
	go func() {
		if poolErr := pool.Run(ctx); poolErr != nil {
			logger.Error("worker pool error", zap.Error(poolErr))
		}
	}()

	// 10. Mark ready and start consuming.
	checker.SetReady(true)
	logger.Info("service ready, consuming from Kafka")

	// Consume loop: read from Kafka → deserialize → submit to worker pool.
	err = kafkautil.ConsumeLoop(ctx, consumer, func(topic string, _, value []byte, _ map[string]string) error {
		var event pb.SecurityEvent
		if unmarshalErr := proto.Unmarshal(value, &event); unmarshalErr != nil {
			logger.Error("failed to unmarshal event",
				zap.String("topic", topic),
				zap.Error(unmarshalErr),
			)
			return unmarshalErr
		}

		return pool.Submit(ctx, &event)
	})

	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("consume loop: %w", err)
	}

	logger.Info("service stopped")
	return nil
}
