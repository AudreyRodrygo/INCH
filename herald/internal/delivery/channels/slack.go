package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/delivery"
)

// Slack delivers notifications via Slack Incoming Webhooks.
//
// Requires a webhook URL from Slack app configuration.
// API: POST to the webhook URL with a JSON payload.
type Slack struct {
	webhookURL string
	client     *http.Client
}

// NewSlack creates a Slack delivery channel.
func NewSlack(webhookURL string) *Slack {
	return &Slack{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the channel identifier.
func (s *Slack) Name() string { return "slack" }

// Send posts a formatted message to Slack.
func (s *Slack) Send(ctx context.Context, notif delivery.Notification) error {
	if s.webhookURL == "" {
		return nil
	}

	// Slack Block Kit message format.
	text := fmt.Sprintf("*[%s]* %s\n%s\n_Recipient: %s_",
		notif.Priority(),
		notif["subject"],
		notif["body"],
		notif.Recipient(),
	)

	payload, _ := json.Marshal(map[string]string{
		"text": text,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("slack webhook call: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}
