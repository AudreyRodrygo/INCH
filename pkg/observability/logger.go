// Package observability provides a unified setup for logging, metrics, and tracing
// across all Sentinel and Herald services.
//
// Every service initializes observability once at startup:
//
//	obs, err := observability.New(ctx, observability.Config{
//	    ServiceName: "event-collector",
//	    LogLevel:    "info",
//	    OTLPEndpoint: "localhost:4317",
//	})
//	defer obs.Shutdown(ctx)
//
// The package ensures consistent instrumentation: same log format, same metric
// naming conventions, same trace propagation across all services.
package observability

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// NewLogger creates a structured zap logger configured for the given environment.
//
// The logger outputs JSON in production (machine-readable, for log aggregation)
// and a human-friendly console format in development.
//
// Every log entry automatically includes the service name, making it easy to
// filter logs when multiple services write to the same output.
func NewLogger(serviceName, level string, development bool) (*zap.Logger, error) {
	// Parse the log level string into a zap level.
	// Valid levels: "debug", "info", "warn", "error"
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parsing log level %q: %w", level, err)
	}

	var cfg zap.Config
	if development {
		// Development: colored console output, human-readable timestamps.
		cfg = zap.NewDevelopmentConfig()
	} else {
		// Production: JSON output, ISO8601 timestamps, sampling under load.
		// Sampling: after 100 identical messages, log only every 100th.
		// This prevents a single noisy error from filling disk.
		cfg = zap.NewProductionConfig()
	}

	cfg.Level = zap.NewAtomicLevelAt(lvl)

	logger, err := cfg.Build(
		// Add service name to every log entry.
		zap.Fields(zap.String("service", serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("building logger: %w", err)
	}

	return logger, nil
}

// MustLogger is like NewLogger but panics on error. Use in main() only.
func MustLogger(serviceName, level string, development bool) *zap.Logger {
	logger, err := NewLogger(serviceName, level, development)
	if err != nil {
		panic(fmt.Sprintf("observability: %v", err))
	}
	return logger
}
