package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer sets up OpenTelemetry distributed tracing.
//
// It connects to an OTLP collector (like Jaeger) via gRPC and exports
// trace spans. Every service call creates a span; spans from different
// services are linked by a shared trace ID.
//
// The returned shutdown function must be called during graceful shutdown
// to flush any pending spans.
//
// If otlpEndpoint is empty, a no-op tracer is returned (useful for tests).
//
// How distributed tracing works across our services:
//  1. Agent sends event → Collector creates span, puts trace ID in Kafka headers
//  2. Processor reads from Kafka → extracts trace ID, creates child span
//  3. Alert Manager → extracts trace ID, creates child span, puts in NATS headers
//  4. Dispatcher → extracts trace ID, creates final span
//
// Result: one trace in Jaeger shows the complete event journey.
func InitTracer(ctx context.Context, serviceName, otlpEndpoint string) (shutdown func(context.Context) error, err error) {
	if otlpEndpoint == "" {
		// No endpoint configured — use no-op tracer.
		// This is common in tests and local development without Jaeger.
		return func(context.Context) error { return nil }, nil
	}

	// Create the OTLP exporter that sends spans to Jaeger/collector via gRPC.
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(otlpEndpoint),
		otlptracegrpc.WithInsecure(), // No TLS for local development.
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP exporter: %w", err)
	}

	// Resource describes this service — appears in Jaeger UI.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// TracerProvider manages span creation and export.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter), // Batch spans for efficiency.
		sdktrace.WithResource(res),
	)

	// Set as global tracer provider — any code can create spans via otel.Tracer().
	otel.SetTracerProvider(tp)

	// Set up context propagation — this is how trace IDs cross service boundaries.
	// W3C TraceContext is the standard format used in HTTP headers, gRPC metadata,
	// and message broker headers.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}

// Tracer returns a named tracer for creating spans in application code.
//
// Usage:
//
//	tracer := observability.Tracer("collector")
//	ctx, span := tracer.Start(ctx, "process-event")
//	defer span.End()
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}
