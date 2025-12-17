package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/delivery"
	"github.com/AudreyRodrygo/Sentinel/herald/internal/delivery/channels"
	"github.com/AudreyRodrygo/Sentinel/pkg/config"
	"github.com/AudreyRodrygo/Sentinel/pkg/dlq"
	"github.com/AudreyRodrygo/Sentinel/pkg/health"
	"github.com/AudreyRodrygo/Sentinel/pkg/natsutil"
	"github.com/AudreyRodrygo/Sentinel/pkg/observability"
)

const serviceName = "delivery-worker"

// workerConfig holds delivery-worker configuration.
type workerConfig struct {
	MetricsPort int    `mapstructure:"metrics_port"`
	LogLevel    string `mapstructure:"log_level"`
	Development bool   `mapstructure:"development"`

	NATS natsutil.Config `mapstructure:"nats"`

	// Channel configurations.
	WebhookURL    string `mapstructure:"webhook_url"`
	WebhookSecret string `mapstructure:"webhook_secret"`
	TelegramToken string `mapstructure:"telegram_token"`
	TelegramChat  string `mapstructure:"telegram_chat"`
	SlackWebhook  string `mapstructure:"slack_webhook"`

	OTLPEndpoint string `mapstructure:"otlp_endpoint"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%s: %v", serviceName, err)
	}
}

func run() error {
	cfg := workerConfig{
		MetricsPort: 8092,
		LogLevel:    "info",
		Development: true,
		NATS:        natsutil.Config{URL: "nats://localhost:4222"},
	}
	if err := config.Load("DELIVERY", "", &cfg); err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := observability.MustLogger(serviceName, cfg.LogLevel, cfg.Development)
	defer func() { _ = logger.Sync() }()

	logger.Info("starting service")

	// Connect to NATS.
	natsConn, js, err := natsutil.Connect(ctx, cfg.NATS)
	if err != nil {
		return fmt.Errorf("connecting to NATS: %w", err)
	}
	defer natsConn.Close()

	// Register delivery channels.
	var deliveryChannels []delivery.Channel

	// Log channel — always active.
	deliveryChannels = append(deliveryChannels, channels.NewLog(logger))

	// Optional channels — only if configured.
	if cfg.WebhookURL != "" {
		deliveryChannels = append(deliveryChannels, channels.NewWebhook(cfg.WebhookURL, cfg.WebhookSecret))
		logger.Info("webhook channel enabled")
	}
	if cfg.TelegramToken != "" {
		deliveryChannels = append(deliveryChannels, channels.NewTelegram(cfg.TelegramToken, cfg.TelegramChat))
		logger.Info("telegram channel enabled")
	}
	if cfg.SlackWebhook != "" {
		deliveryChannels = append(deliveryChannels, channels.NewSlack(cfg.SlackWebhook))
		logger.Info("slack channel enabled")
	}

	// Dead Letter Queue.
	deadLetters := dlq.NewMemory(10000)

	// Create and run worker.
	worker := delivery.NewWorker(js, deliveryChannels, deadLetters, logger)

	// Health check.
	checker := health.New()
	go func() {
		if healthErr := checker.ListenAndServe(ctx, fmt.Sprintf(":%d", cfg.MetricsPort)); healthErr != nil {
			logger.Error("health server error", zap.Error(healthErr))
		}
	}()

	checker.SetReady(true)
	logger.Info("service ready")

	if runErr := worker.Run(ctx); runErr != nil && ctx.Err() == nil {
		return fmt.Errorf("worker: %w", runErr)
	}

	logger.Info("service stopped", zap.Int("dlq_size", deadLetters.Len()))
	return nil
}
