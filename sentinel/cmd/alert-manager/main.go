package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/AudreyRodrygo/Sentinel/pkg/config"
	"github.com/AudreyRodrygo/Sentinel/pkg/health"
	"github.com/AudreyRodrygo/Sentinel/pkg/kafkautil"
	"github.com/AudreyRodrygo/Sentinel/pkg/natsutil"
	"github.com/AudreyRodrygo/Sentinel/pkg/observability"
	"github.com/AudreyRodrygo/Sentinel/sentinel/internal/alertmgr"
)

const serviceName = "alert-manager"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

func run() error {
	cfg := alertmgr.Defaults()
	if err := config.Load("ALERTMGR", "", &cfg); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := observability.MustLogger(serviceName, cfg.LogLevel, cfg.Development)
	defer func() { _ = logger.Sync() }()

	logger.Info("starting service")

	shutdownTracer, err := observability.InitTracer(ctx, serviceName, cfg.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() { _ = shutdownTracer(context.Background()) }() //nolint:gosec // need fresh context for shutdown

	// Connect to NATS.
	natsConn, js, err := natsutil.Connect(ctx, cfg.NATS)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer natsConn.Close()

	// Ensure the alerts stream exists.
	if streamErr := natsutil.EnsureStream(ctx, js, natsutil.StreamConfig{
		Name:     "ALERTS",
		Subjects: []string{"alerts.>"},
	}); streamErr != nil {
		return fmt.Errorf("ensuring NATS stream: %w", streamErr)
	}

	// Create Kafka consumer for alerts topic.
	consumer, err := kafkautil.NewConsumer(cfg.Kafka)
	if err != nil {
		return fmt.Errorf("creating kafka consumer: %w", err)
	}
	defer consumer.Close()

	// Create alert handler with dedup, rate limiter, circuit breaker.
	suppressWindow := time.Duration(cfg.SuppressWindowSec) * time.Second
	handler := alertmgr.NewHandler(js, "alerts.dispatch", suppressWindow, cfg.RateLimit, logger)
	defer handler.Stop()

	// Health check.
	checker := health.New()
	go func() {
		if healthErr := checker.ListenAndServe(ctx, fmt.Sprintf(":%d", cfg.MetricsPort)); healthErr != nil {
			logger.Error("health server error", zap.Error(healthErr))
		}
	}()

	checker.SetReady(true)
	logger.Info("service ready, consuming alerts from Kafka")

	// Consume loop: read alerts from Kafka → process through handler → publish to NATS.
	err = kafkautil.ConsumeLoop(ctx, consumer, func(_ string, _, value []byte, _ map[string]string) error {
		return handler.Handle(ctx, value)
	})

	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("consume loop: %w", err)
	}

	logger.Info("service stopped")
	return nil
}
