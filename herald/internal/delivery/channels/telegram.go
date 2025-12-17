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

// Telegram delivers notifications via the Telegram Bot API.
//
// Requires:
//   - Bot token (from @BotFather)
//   - Chat ID (the user or group to send to)
//
// API: POST https://api.telegram.org/bot<token>/sendMessage
type Telegram struct {
	token  string
	chatID string
	client *http.Client
}

// NewTelegram creates a Telegram delivery channel.
func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Name returns the channel identifier.
func (t *Telegram) Name() string { return "telegram" }

// Send posts a formatted message to Telegram.
func (t *Telegram) Send(ctx context.Context, notif delivery.Notification) error {
	if t.token == "" || t.chatID == "" {
		return nil
	}

	// Format the notification as a readable Telegram message.
	text := fmt.Sprintf(
		"🔔 *%s* — %s\n\n%s\n\n_Recipient: %s_",
		notif.Priority(),
		notif["subject"],
		notif["body"],
		notif.Recipient(),
	)

	payload, _ := json.Marshal(map[string]string{
		"chat_id":    t.chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("creating telegram request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram API call: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status %d", resp.StatusCode)
	}

	return nil
}
