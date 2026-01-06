package observability_test

import (
	"testing"

	"github.com/AudreyRodrygo/Inch/pkg/observability"
)

func TestNewLogger_Development(t *testing.T) {
	logger, err := observability.NewLogger("test-service", "debug", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the logger works by writing a test message.
	// In development mode, this outputs colored console text.
	logger.Info("test message from development logger")
	_ = logger.Sync()
}

func TestNewLogger_Production(t *testing.T) {
	logger, err := observability.NewLogger("test-service", "info", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Production mode outputs JSON.
	logger.Info("test message from production logger")
	_ = logger.Sync()
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	_, err := observability.NewLogger("test-service", "invalid", false)
	if err == nil {
		t.Fatal("expected error for invalid log level, got nil")
	}
}

func TestNewLogger_AllLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := observability.NewLogger("test", level, true)
			if err != nil {
				t.Fatalf("level %q: unexpected error: %v", level, err)
			}
			_ = logger.Sync()
		})
	}
}
