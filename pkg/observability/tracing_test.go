package observability_test

import (
	"context"
	"testing"

	"github.com/yourusername/sentinel/pkg/observability"
)

func TestInitTracer_NoOpWhenEmpty(t *testing.T) {
	// When no OTLP endpoint is provided, InitTracer returns a no-op tracer.
	// This is the expected behavior for tests and local development.
	shutdown, err := observability.InitTracer(context.Background(), "test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Shutdown should succeed even for no-op.
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
}

func TestTracer_ReturnsNonNil(t *testing.T) {
	tracer := observability.Tracer("test-component")
	if tracer == nil {
		t.Fatal("Tracer returned nil")
	}
}
