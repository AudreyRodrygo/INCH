package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/AudreyRodrygo/Inch/inch/internal/dispatcher"
	"github.com/AudreyRodrygo/Inch/pkg/config"
	"github.com/AudreyRodrygo/Inch/pkg/dlq"
	"github.com/AudreyRodrygo/Inch/pkg/health"
	"github.com/AudreyRodrygo/Inch/pkg/natsutil"
	"github.com/AudreyRodrygo/Inch/pkg/observability"
)

const serviceName = "notification-dispatcher"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

func run() error {
	cfg := dispatcher.Defaults()
	if err := config.Load("DISPATCHER", "", &cfg); err != nil {
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

	// Register notification channels.
	channels := []dispatcher.Channel{
		// Log channel — always active, useful for development.
		dispatcher.NewLogChannel(logger),
	}

	// Webhook channel — if configured.
	if cfg.Webhook.URL != "" {
		channels = append(channels, dispatcher.NewWebhook(cfg.Webhook.URL, cfg.Webhook.Secret))
		logger.Info("webhook channel enabled", zap.String("url", cfg.Webhook.URL))
	}

	// Dead Letter Queue (in-memory for now, could be NATS/Kafka).
	deadLetters := dlq.NewMemory(10000)

	// Create dispatcher.
	d := dispatcher.New(js, channels, deadLetters, logger)

	// Health check.
	checker := health.New()
	go func() {
		if healthErr := checker.ListenAndServe(ctx, fmt.Sprintf(":%d", cfg.MetricsPort)); healthErr != nil {
			logger.Error("health server error", zap.Error(healthErr))
		}
	}()

	checker.SetReady(true)
	logger.Info("service ready, consuming from NATS")

	// Run dispatcher (blocks until context cancelled).
	if runErr := d.Run(ctx); runErr != nil && ctx.Err() == nil {
		return fmt.Errorf("dispatcher: %w", runErr)
	}

	logger.Info("service stopped",
		zap.Int("dlq_size", deadLetters.Len()),
	)
	return nil
}
