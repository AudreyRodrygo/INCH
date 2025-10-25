// Package retry provides a retry mechanism with exponential backoff and jitter.
//
// Exponential backoff: each retry waits 2x longer than the previous one.
// Jitter: adds randomness to prevent thundering herd (many clients retrying simultaneously).
//
// Usage:
//
//	err := retry.Do(ctx, func() error {
//	    return sendRequest()
//	}, retry.WithMaxAttempts(5), retry.WithBaseDelay(100*time.Millisecond))
//
// The function respects context cancellation — if the context is done
// (timeout, shutdown), the retry loop exits immediately.
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

// ErrMaxAttemptsReached is returned when all retry attempts are exhausted.
var ErrMaxAttemptsReached = errors.New("max retry attempts reached")

// config holds the retry parameters. It's unexported — users configure
// it through functional options (WithMaxAttempts, WithBaseDelay, etc.).
type config struct {
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	multiplier  float64
}

// defaults returns a config with sensible production values.
func defaults() config {
	return config{
		maxAttempts: 3,
		baseDelay:   100 * time.Millisecond,
		maxDelay:    10 * time.Second,
		multiplier:  2.0,
	}
}

// Option configures the retry behavior. This is the Functional Options Pattern —
// a Go idiom for optional configuration without constructors with many parameters.
//
// Instead of: New(3, 100ms, 10s, 2.0)    ← unclear what each number means
// We write:   Do(fn, WithMaxAttempts(3))  ← self-documenting
type Option func(*config)

// WithMaxAttempts sets the maximum number of attempts (including the first call).
// Default: 3. Set to 1 for no retries.
func WithMaxAttempts(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxAttempts = n
		}
	}
}

// WithBaseDelay sets the initial delay before the first retry.
// Default: 100ms. Subsequent delays are multiplied by the multiplier.
func WithBaseDelay(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.baseDelay = d
		}
	}
}

// WithMaxDelay caps the maximum delay between retries.
// Default: 10s. Without a cap, exponential growth could produce absurd waits.
func WithMaxDelay(d time.Duration) Option {
	return func(c *config) {
		if d > 0 {
			c.maxDelay = d
		}
	}
}

// WithMultiplier sets the exponential growth factor.
// Default: 2.0 (each delay is 2x the previous).
func WithMultiplier(m float64) Option {
	return func(c *config) {
		if m > 0 {
			c.multiplier = m
		}
	}
}

// Do executes fn and retries it with exponential backoff if it returns an error.
//
// The retry loop exits when:
//   - fn returns nil (success)
//   - All attempts are exhausted (returns ErrMaxAttemptsReached wrapping the last error)
//   - The context is cancelled (returns ctx.Err())
//
// Jitter: each delay is randomized to ±25% to prevent thundering herd.
// For example, a 400ms delay becomes something between 300ms and 500ms.
func Do(ctx context.Context, fn func() error, opts ...Option) error {
	cfg := defaults()
	for _, opt := range opts {
		opt(&cfg)
	}

	var lastErr error
	delay := cfg.baseDelay

	for attempt := range cfg.maxAttempts {
		// Execute the function.
		lastErr = fn()
		if lastErr == nil {
			return nil // Success — no more retries needed.
		}

		// Don't sleep after the last attempt — we're done.
		if attempt == cfg.maxAttempts-1 {
			break
		}

		// Apply jitter: randomize ±25% to spread out retries.
		jittered := addJitter(delay)

		// Wait, but respect context cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(jittered):
		}

		// Grow the delay exponentially, but cap it.
		delay = time.Duration(float64(delay) * cfg.multiplier)
		if delay > cfg.maxDelay {
			delay = cfg.maxDelay
		}
	}

	return fmt.Errorf("%w: %v", ErrMaxAttemptsReached, lastErr)
}

// addJitter adds ±25% randomness to a duration.
// This prevents multiple clients from retrying at exactly the same time.
//
// Example: 400ms → random value between 300ms (75%) and 500ms (125%).
func addJitter(d time.Duration) time.Duration {
	// jitter is in range [0.75, 1.25)
	jitter := 0.75 + rand.Float64()*0.5 //nolint:gosec // Not crypto, just delay randomization.
	return time.Duration(float64(d) * jitter)
}
