package dispatcher

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
)

// Webhook delivers alerts via HTTP POST with HMAC-SHA256 signature.
//
// The receiving server can verify the request is authentic by:
//  1. Computing HMAC-SHA256 of the request body with the shared secret
//  2. Comparing it to the X-INCH-Signature header
//
// This is the same pattern used by GitHub webhooks, Stripe, etc.
type Webhook struct {
	url    string
	secret string
	client *http.Client
}

// NewWebhook creates a webhook channel.
func NewWebhook(url, secret string) *Webhook {
	return &Webhook{
		url:    url,
		secret: secret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the channel identifier.
func (w *Webhook) Name() string { return "webhook" }

// Send POSTs the alert as JSON to the webhook URL.
//
// Headers:
//   - Content-Type: application/json
//   - X-INCH-Signature: HMAC-SHA256 hex digest
//   - X-INCH-Event: alert
func (w *Webhook) Send(ctx context.Context, alert AlertPayload) error {
	if w.url == "" {
		return nil // Webhook not configured — skip silently.
	}

	body, err := json.Marshal(alert)
	if err != nil {
		return fmt.Errorf("marshaling webhook body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-INCH-Event", "alert")

	// Sign the payload with HMAC-SHA256.
	if w.secret != "" {
		signature := computeHMAC(body, []byte(w.secret))
		req.Header.Set("X-INCH-Signature", "sha256="+signature)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook POST to %s: %w", w.url, err)
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

// computeHMAC calculates HMAC-SHA256 and returns the hex-encoded digest.
func computeHMAC(message, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return hex.EncodeToString(mac.Sum(nil))
}
