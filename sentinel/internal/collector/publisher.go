package collector

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel/propagation"

	"github.com/AudreyRodrygo/Sentinel/pkg/observability"
)

// Publisher sends validated events to Kafka.
//
// Each event is published to the "raw-events" topic with:
//   - Key: source hostname (ensures events from the same host go to the same
//     partition, preserving per-host ordering)
//   - Value: Protobuf-encoded SecurityEvent
//   - Headers: OpenTelemetry trace context (for distributed tracing)
//
// The processor reads from this topic and enriches/correlates events.
type Publisher struct {
	client *kgo.Client
	topic  string
}

// NewPublisher creates a Kafka event publisher.
func NewPublisher(client *kgo.Client, topic string) *Publisher {
	return &Publisher{
		client: client,
		topic:  topic,
	}
}

// Publish sends a single event to Kafka.
//
// The key determines partition assignment — events with the same key
// always go to the same partition. We use hostname as key so all events
// from one machine are processed in order (important for sequence correlation).
//
// Trace context is propagated via Kafka headers so the processor's span
// becomes a child of the collector's span in Jaeger.
func (p *Publisher) Publish(ctx context.Context, key, value []byte) error {
	record := &kgo.Record{
		Topic: p.topic,
		Key:   key,
		Value: value,
	}

	// Inject OpenTelemetry trace context into Kafka headers.
	// This is how distributed tracing crosses service boundaries:
	// collector creates a span → puts trace ID in Kafka headers →
	// processor reads headers → creates child span.
	injectTraceHeaders(ctx, record)

	results := p.client.ProduceSync(ctx, record)
	if err := results.FirstErr(); err != nil {
		return fmt.Errorf("publishing to kafka topic %s: %w", p.topic, err)
	}

	return nil
}

// kafkaHeaderCarrier adapts Kafka record headers to the OpenTelemetry
// TextMapCarrier interface, allowing trace context injection.
//
// This is the bridge between OTel's propagation API and Kafka's header format.
type kafkaHeaderCarrier struct {
	record *kgo.Record
}

func (c *kafkaHeaderCarrier) Get(key string) string {
	for _, h := range c.record.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c *kafkaHeaderCarrier) Set(key, value string) {
	// Remove existing header with the same key.
	for i, h := range c.record.Headers {
		if h.Key == key {
			c.record.Headers[i].Value = []byte(value)
			return
		}
	}
	// Add new header.
	c.record.Headers = append(c.record.Headers, kgo.RecordHeader{
		Key:   key,
		Value: []byte(value),
	})
}

func (c *kafkaHeaderCarrier) Keys() []string {
	keys := make([]string, len(c.record.Headers))
	for i, h := range c.record.Headers {
		keys[i] = h.Key
	}
	return keys
}

// injectTraceHeaders writes OpenTelemetry trace context into Kafka headers.
func injectTraceHeaders(ctx context.Context, record *kgo.Record) {
	propagator := propagation.TraceContext{}
	carrier := &kafkaHeaderCarrier{record: record}
	propagator.Inject(ctx, carrier)

	// Also add a human-readable trace ID header for debugging.
	tracer := observability.Tracer("collector")
	_, span := tracer.Start(ctx, "kafka.produce")
	defer span.End()
}
