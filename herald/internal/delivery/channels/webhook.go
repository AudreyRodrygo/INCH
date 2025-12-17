package channels

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/delivery"
)

// Webhook delivers notifications via HTTP POST with HMAC-SHA256 signature.
type Webhook struct {
	url    string
	secret string
	client *http.Client
}

// NewWebhook creates a webhook delivery channel.
func NewWebhook(url, secret string) *Webhook {
	return &Webhook{
		url:    url,
		secret: secret,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the channel identifier.
func (w *Webhook) Name() string { return "webhook" }

// Send POSTs the notification as JSON.
func (w *Webhook) Send(ctx context.Context, notif delivery.Notification) error {
	if w.url == "" {
		return nil
	}

	body, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("marshaling webhook body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Herald-Event", "notification")

	if w.secret != "" {
		mac := hmac.New(sha256.New, []byte(w.secret))
		mac.Write(body)
		req.Header.Set("X-Herald-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}
