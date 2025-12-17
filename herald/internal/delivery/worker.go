package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"go.uber.org/zap"

	"github.com/AudreyRodrygo/Sentinel/pkg/dlq"
	"github.com/AudreyRodrygo/Sentinel/pkg/retry"
)

// Worker consumes notifications from NATS and delivers through channels.
type Worker struct {
	js       jetstream.JetStream
	channels []Channel
	dlq      dlq.Queue
	logger   *zap.Logger
}

// NewWorker creates a delivery worker.
func NewWorker(js jetstream.JetStream, channels []Channel, dlqQueue dlq.Queue, logger *zap.Logger) *Worker {
	return &Worker{
		js:       js,
		channels: channels,
		dlq:      dlqQueue,
		logger:   logger,
	}
}

// Run starts consuming and delivering. Blocks until context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	consumer, err := w.js.CreateOrUpdateConsumer(ctx, "HERALD", jetstream.ConsumerConfig{
		Durable:       "delivery-worker",
		AckPolicy:     jetstream.AckExplicitPolicy,
		FilterSubject: "herald.deliver",
	})
	if err != nil {
		return fmt.Errorf("creating NATS consumer: %w", err)
	}

	w.logger.Info("delivery worker running", zap.Int("channels", len(w.channels)))

	for {
		msgs, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
		if fetchErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}

		for msg := range msgs.Messages() {
			w.handleMessage(ctx, msg)
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// handleMessage delivers a notification through all registered channels.
func (w *Worker) handleMessage(ctx context.Context, msg jetstream.Msg) {
	var notif Notification
	if err := json.Unmarshal(msg.Data(), &notif); err != nil {
		w.logger.Error("invalid notification JSON", zap.Error(err))
		_ = msg.Ack()
		return
	}

	for _, ch := range w.channels {
		err := retry.Do(ctx, func() error {
			return ch.Send(ctx, notif)
		}, retry.WithMaxAttempts(3), retry.WithBaseDelay(500*time.Millisecond))
		if err != nil {
			w.logger.Error("delivery failed",
				zap.String("channel", ch.Name()),
				zap.String("notification_id", notif.ID()),
				zap.Error(err),
			)
			_ = w.dlq.Push(ctx, dlq.Message{
				OriginalTopic: "herald.deliver",
				Value:         msg.Data(),
				Error:         fmt.Sprintf("channel %s: %v", ch.Name(), err),
				Attempts:      3,
			})
		}
	}

	_ = msg.Ack()
}
