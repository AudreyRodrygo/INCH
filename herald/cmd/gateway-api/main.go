package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/gateway"
	"github.com/AudreyRodrygo/Sentinel/herald/internal/gateway/priority"
	"github.com/AudreyRodrygo/Sentinel/pkg/config"
	"github.com/AudreyRodrygo/Sentinel/pkg/health"
	"github.com/AudreyRodrygo/Sentinel/pkg/natsutil"
	"github.com/AudreyRodrygo/Sentinel/pkg/observability"
)

const serviceName = "gateway-api"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

func run() error {
	cfg := gateway.Defaults()
	if err := config.Load("GATEWAY", "", &cfg); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := observability.MustLogger(serviceName, cfg.LogLevel, cfg.Development)
	defer func() { _ = logger.Sync() }()

	logger.Info("starting service", zap.Int("http_port", cfg.HTTPPort))

	// Connect to NATS.
	natsConn, js, err := natsutil.Connect(ctx, cfg.NATS)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer natsConn.Close()

	// Ensure HERALD stream exists.
	if streamErr := natsutil.EnsureStream(ctx, js, natsutil.StreamConfig{
		Name:     "HERALD",
		Subjects: []string{"herald.>"},
	}); streamErr != nil {
		return fmt.Errorf("ensuring NATS stream: %w", streamErr)
	}

	// Priority queue.
	queue := priority.New()

	// Start queue drainer: reads from priority queue → publishes to NATS.
	go drainQueue(ctx, queue, js, logger)

	// REST server.
	srv := gateway.NewServer(queue, logger)
	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Health check.
	checker := health.New()
	go func() {
		if healthErr := checker.ListenAndServe(ctx, fmt.Sprintf(":%d", cfg.MetricsPort)); healthErr != nil {
			logger.Error("health server error", zap.Error(healthErr))
		}
	}()

	// Start HTTP server.
	go func() {
		logger.Info("HTTP server started", zap.String("addr", httpServer.Addr))
		if srvErr := httpServer.ListenAndServe(); srvErr != nil && srvErr != http.ErrServerClosed {
			logger.Error("HTTP server error", zap.Error(srvErr))
		}
	}()

	checker.SetReady(true)
	logger.Info("service ready")

	<-ctx.Done()
	logger.Info("shutting down...")

	checker.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second) //nolint:gosec // need fresh context
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)

	logger.Info("service stopped")
	return nil
}

// drainQueue continuously reads from the priority queue and publishes to NATS.
// Higher-priority items are dequeued first (heap ordering).
func drainQueue(ctx context.Context, queue *priority.Queue, js jetstream.JetStream, logger *zap.Logger) {
	done := ctx.Done()
	for {
		item, ok := queue.Pop(done)
		if !ok {
			return // Context cancelled.
		}

		_, err := js.Publish(ctx, "herald.deliver", item.Payload)
		if err != nil {
			logger.Error("failed to publish to NATS",
				zap.String("id", item.ID),
				zap.Error(err),
			)
			// Re-queue on failure.
			queue.Push(item)
			time.Sleep(100 * time.Millisecond) // Brief backoff.
			continue
		}

		logger.Debug("notification published to NATS",
			zap.String("id", item.ID),
			zap.String("priority", item.Priority.String()),
		)
	}
}
