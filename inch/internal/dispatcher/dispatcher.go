package dispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/AudreyRodrygo/Inch/pkg/dlq"
	"github.com/AudreyRodrygo/Inch/pkg/retry"
)

// Dispatcher consumes alerts from NATS and delivers through channels.
//
// For each alert, it tries every registered channel. If a channel fails
// after retries, the alert goes to the Dead Letter Queue for later review.
type Dispatcher struct {
	js       jetstream.JetStream
	channels []Channel
	dlq      dlq.Queue
	logger   *zap.Logger
}

// New creates a notification dispatcher.
func New(js jetstream.JetStream, channels []Channel, dlqQueue dlq.Queue, logger *zap.Logger) *Dispatcher {
	return &Dispatcher{
		js:       js,
		channels: channels,
		dlq:      dlqQueue,
		logger:   logger,
	}
}

// Run starts consuming alerts from NATS and dispatching to channels.
// Blocks until the context is cancelled.
func (d *Dispatcher) Run(ctx context.Context) error {
	// Create a durable consumer on the ALERTS stream.
	consumer, err := d.js.CreateOrUpdateConsumer(ctx, "ALERTS", jetstream.ConsumerConfig{
		Durable:       "notification-dispatcher",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: "alerts.dispatch",
	})
	if err != nil {
		return fmt.Errorf("creating NATS consumer: %w", err)
	}

	d.logger.Info("consuming alerts from NATS",
		zap.Int("channels", len(d.channels)),
	)

	for {
		// Fetch one message at a time for simplicity.
		// For higher throughput, use consumer.Messages() for streaming.
		msgs, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if fetchErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// Timeout is normal — no messages available.
			continue
		}

		for msg := range msgs.Messages() {
			d.handleMessage(ctx, msg)
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// handleMessage processes a single NATS message.
func (d *Dispatcher) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var alert AlertPayload
	if err := json.Unmarshal(msg.Data(), &alert); err != nil {
		d.logger.Error("failed to unmarshal alert", zap.Error(err))
		_ = msg.Ack() // Ack bad message to prevent redelivery loop.
		return
	}

	allSuccess := true
	for _, ch := range d.channels {
		// Retry each channel independently with exponential backoff.
		err := retry.Do(ctx, func() error {
			return ch.Send(ctx, alert)
		}, retry.WithMaxAttempts(3), retry.WithBaseDelay(500*time.Millisecond))
		if err != nil {
			allSuccess = false
			d.logger.Error("channel delivery failed after retries",
				zap.String("channel", ch.Name()),
				zap.String("alert_id", alert.AlertID()),
				zap.Error(err),
			)

			// Send to DLQ for later review.
			_ = d.dlq.Push(ctx, dlq.Message{
				OriginalTopic: "alerts.dispatch",
				Value:         msg.Data(),
				Error:         fmt.Sprintf("channel %s: %v", ch.Name(), err),
				Attempts:      3,
			})
		}
	}

	// Acknowledge the message regardless — failed deliveries go to DLQ.
	// If we nack, NATS would redeliver to ALL channels, not just the failed one.
	_ = msg.Ack()

	if allSuccess {
		d.logger.Info("alert delivered to all channels",
			zap.String("alert_id", alert.AlertID()),
			zap.String("severity", alert.Severity()),
		)
	}
}
