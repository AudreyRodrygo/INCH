// Package ratelimiter implements a Token Bucket rate limiter.
//
// The Token Bucket algorithm works like a bucket that holds tokens:
//   - Tokens are added at a fixed rate (e.g., 10 per second)
//   - Each operation consumes one token
//   - The bucket has a maximum capacity (burst size)
//   - If empty, operations are either rejected (Allow) or blocked (Wait)
//
// This is used in INCH's alert-manager (limit alerts per channel)
// and INCH's gateway-api (limit requests per client).
//
// The implementation is lock-free using atomic operations for performance
// under high concurrency — critical for our 10k events/sec target.
//
// Usage:
//
//	limiter := ratelimiter.New(10, 20) // 10 tokens/sec, burst of 20
//	defer limiter.Stop()
//
//	if limiter.Allow() {
//	    sendAlert() // Token consumed.
//	} else {
//	    drop() // Rate limit exceeded.
//	}
package ratelimiter

import (
	"context"
	"sync/atomic"
	"time"
)

// Limiter implements the Token Bucket algorithm.
//
// It is safe for concurrent use by multiple goroutines.
// The zero value is not usable — create with New().
type Limiter struct {
	tokens   atomic.Int64  // Current number of available tokens.
	capacity int64         // Maximum tokens (burst size).
	stopCh   chan struct{} // Signals the refill goroutine to stop.
}

// New creates a rate limiter that allows `rate` operations per second
// with a maximum burst of `burst` operations.
//
// Parameters:
//   - rate: how many tokens are added per second (sustained throughput)
//   - burst: maximum tokens the bucket can hold (peak throughput)
//
// The bucket starts full (at burst capacity), allowing an initial burst
// of traffic before the rate limit kicks in.
//
// Call Stop() when done to release the refill goroutine.
func New(rate float64, burst int64) *Limiter {
	l := &Limiter{
		capacity: burst,
		stopCh:   make(chan struct{}),
	}
	l.tokens.Store(burst) // Start with a full bucket.

	// Start a background goroutine that adds tokens at the specified rate.
	// Using a ticker ensures consistent token addition regardless of usage pattern.
	go l.refill(rate)

	return l
}

// Allow consumes one token and returns true, or returns false if
// no tokens are available (non-blocking).
//
// Use this when you want to drop or reject operations that exceed the rate.
func (l *Limiter) Allow() bool {
	for {
		current := l.tokens.Load()
		if current <= 0 {
			return false // No tokens available.
		}
		// CompareAndSwap: atomically decrement only if the value hasn't changed.
		// If another goroutine consumed a token between Load and CAS, retry.
		if l.tokens.CompareAndSwap(current, current-1) {
			return true
		}
		// CAS failed — another goroutine got the token first. Try again.
	}
}

// Wait blocks until a token is available or the context is cancelled.
//
// Use this when you want to slow down rather than drop operations.
// Returns nil when a token is consumed, or ctx.Err() if cancelled.
func (l *Limiter) Wait(ctx context.Context) error {
	// Fast path: try to get a token immediately.
	if l.Allow() {
		return nil
	}

	// Slow path: poll periodically. The interval is short enough for
	// responsive behavior but not so short that it wastes CPU.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if l.Allow() {
				return nil
			}
		}
	}
}

// Tokens returns the current number of available tokens.
// Useful for monitoring and metrics.
func (l *Limiter) Tokens() int64 {
	return l.tokens.Load()
}

// Stop releases the background refill goroutine.
// The limiter must not be used after Stop is called.
func (l *Limiter) Stop() {
	close(l.stopCh)
}

// refill periodically adds tokens to the bucket at the specified rate.
//
// It runs in a background goroutine and exits when Stop() is called.
// The refill interval is calculated to add exactly `rate` tokens per second.
func (l *Limiter) refill(rate float64) {
	// Calculate how often to add one token.
	// For rate=10/sec: interval = 100ms (add 1 token every 100ms).
	// For rate=1000/sec: interval = 1ms.
	interval := time.Duration(float64(time.Second) / rate)
	if interval < time.Millisecond {
		interval = time.Millisecond // Minimum 1ms granularity.
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopCh:
			return
		case <-ticker.C:
			// Add a token, but don't exceed capacity.
			for {
				current := l.tokens.Load()
				if current >= l.capacity {
					break // Bucket is full.
				}
				if l.tokens.CompareAndSwap(current, current+1) {
					break // Successfully added a token.
				}
				// CAS failed, retry.
			}
		}
	}
}
