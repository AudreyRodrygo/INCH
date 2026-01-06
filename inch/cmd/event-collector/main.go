package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	pb "github.com/AudreyRodrygo/Inch/gen/sentinel/v1"
	"github.com/AudreyRodrygo/Inch/pkg/config"
	"github.com/AudreyRodrygo/Inch/pkg/health"
	"github.com/AudreyRodrygo/Inch/pkg/kafkautil"
	"github.com/AudreyRodrygo/Inch/pkg/observability"
	"github.com/AudreyRodrygo/Inch/pkg/redisutil"
	"github.com/AudreyRodrygo/Inch/inch/internal/collector"
)

const serviceName = "event-collector"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

// run is the real entrypoint. Returning an error lets deferred cleanup execute.
//
// Initialization order matters:
//  1. Config (everything depends on it)
//  2. Logger (need logging for everything after)
//  3. Tracing (spans start appearing from here)
//  4. External connections (Redis, Kafka)
//  5. Business logic (deduplicator, publisher, gRPC server)
//  6. Health check (mark ready only after everything is connected)
func run() error {
	// 1. Load configuration.
	cfg := collector.Defaults()
	if err := config.Load("COLLECTOR", "", &cfg); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// 2. Create a context cancelled on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 3. Initialize structured logger.
	logger := observability.MustLogger(serviceName, cfg.LogLevel, cfg.Development)
	defer func() { _ = logger.Sync() }()

	logger.Info("starting service",
		zap.Int("grpc_port", cfg.GRPCPort),
		zap.Int("metrics_port", cfg.MetricsPort),
	)

	// 4. Initialize distributed tracing.
	shutdownTracer, err := observability.InitTracer(ctx, serviceName, cfg.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	defer func() { _ = shutdownTracer(context.Background()) }()

	// 5. Connect to Redis for deduplication.
	redisClient, err := redisutil.NewClient(ctx, cfg.Redis)
	if err != nil {
		logger.Warn("redis unavailable, dedup disabled", zap.Error(err))
		// We continue without Redis — dedup is best-effort.
		// In production, you might want to fail hard here.
	}
	if redisClient != nil {
		defer func() { _ = redisClient.Close() }()
	}

	dedup := collector.NewDeduplicator(redisClient, cfg.DeduplicationTTL)

	// 6. Create Kafka producer.
	kafkaClient, err := kafkautil.NewProducer(cfg.Kafka)
	if err != nil {
		return fmt.Errorf("creating kafka producer: %w", err)
	}
	defer kafkaClient.Close()

	publisher := collector.NewPublisher(kafkaClient, cfg.Kafka.Topic)

	// 7. Create the gRPC server with the collector service.
	grpcServer := grpc.NewServer()
	collectorServer := collector.NewServer(dedup, publisher, logger)
	pb.RegisterCollectorServiceServer(grpcServer, collectorServer)

	// 8. Start health check server.
	checker := health.New()
	go func() {
		metricsAddr := fmt.Sprintf(":%d", cfg.MetricsPort)

		mux := http.NewServeMux()
		mux.Handle("/healthz", checker.Handler())
		mux.Handle("/readyz", checker.Handler())
		mux.Handle("/metrics", observability.MetricsHandler())

		srv := &http.Server{
			Addr:              metricsAddr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}

		logger.Info("metrics server started", zap.String("addr", metricsAddr))
		if srvErr := srv.ListenAndServe(); srvErr != nil && srvErr != http.ErrServerClosed {
			logger.Error("metrics server error", zap.Error(srvErr))
		}
	}()

	// 9. Start gRPC server.
	grpcAddr := fmt.Sprintf(":%d", cfg.GRPCPort)
	listener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", grpcAddr, err)
	}

	go func() {
		logger.Info("gRPC server started", zap.String("addr", grpcAddr))
		if err := grpcServer.Serve(listener); err != nil {
			logger.Error("gRPC server error", zap.Error(err))
		}
	}()

	// 10. Mark as ready — Kubernetes can now send traffic.
	checker.SetReady(true)
	logger.Info("service ready")

	// 11. Wait for shutdown signal.
	<-ctx.Done()
	logger.Info("shutting down...")

	// Graceful shutdown: stop accepting new connections, finish in-flight.
	checker.SetReady(false)
	grpcServer.GracefulStop()

	logger.Info("service stopped")
	return nil
}
