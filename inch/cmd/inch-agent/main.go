package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
	"github.com/AudreyRodrygo/Inch/inch/internal/agent"
	"github.com/AudreyRodrygo/Inch/pkg/config"
	"github.com/AudreyRodrygo/Inch/pkg/observability"
)

const serviceName = "inch-agent"

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

func run() error {
	cfg := agent.Defaults()
	if err := config.Load("AGENT", "", &cfg); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := observability.MustLogger(serviceName, cfg.LogLevel, cfg.Development)
	defer func() { _ = logger.Sync() }()

	logger.Info("starting agent",
		zap.String("agent_id", cfg.AgentID),
		zap.String("collector", cfg.CollectorAddr),
		zap.Int("batch_size", cfg.BatchSize),
	)

	// Event channel — all sources write here, batcher reads.
	events := make(chan *pb.SecurityEvent, 1000)

	// Create batcher (gRPC connection to collector).
	batcher, err := agent.NewBatcher(
		cfg.CollectorAddr, cfg.AgentID,
		cfg.BatchSize, cfg.FlushIntervalSec,
		logger,
	)
	if err != nil {
		return fmt.Errorf("creating batcher: %w", err)
	}
	defer func() { _ = batcher.Close() }()

	// Start all sources + batcher in an errgroup.
	g, ctx := errgroup.WithContext(ctx)

	// Start log tailers.
	for _, path := range cfg.LogFiles {
		tailer := agent.NewLogTailer(path, cfg.AgentID, logger)
		g.Go(func() error {
			if sourceErr := tailer.Start(ctx, events); sourceErr != nil && ctx.Err() == nil {
				logger.Warn("log tailer stopped", zap.String("source", tailer.Name()), zap.Error(sourceErr))
			}
			return nil // Don't kill other sources if one file disappears.
		})
	}

	// Start process watcher (if enabled).
	if cfg.EnableProcessWatch {
		pw := agent.NewProcessWatcher(cfg.ProcessWatchIntervalSec, cfg.AgentID, logger)
		g.Go(func() error {
			return pw.Start(ctx, events)
		})
	}

	// Start batcher (reads from events channel, sends to collector).
	g.Go(func() error {
		return batcher.Run(ctx, events)
	})

	logger.Info("agent running")

	// Wait for all goroutines to finish (triggered by ctx cancel).
	if err := g.Wait(); err != nil && ctx.Err() == nil {
		return fmt.Errorf("agent error: %w", err)
	}

	logger.Info("agent stopped")
	return nil
}
