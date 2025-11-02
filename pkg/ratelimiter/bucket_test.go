package ratelimiter_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AudreyRodrygo/Sentinel/pkg/ratelimiter"
)

func TestAllow_ConsumesTokens(t *testing.T) {
	limiter := ratelimiter.New(100, 5) // 100/sec, burst 5
	defer limiter.Stop()

	// Should allow 5 calls (burst capacity).
	for i := range 5 {
		if !limiter.Allow() {
			t.Fatalf("Allow() returned false on call %d, want true", i+1)
		}
	}

	// 6th call should be rejected — bucket is empty.
	if limiter.Allow() {
		t.Error("Allow() returned true when bucket should be empty")
	}
}

func TestAllow_RefillsOverTime(t *testing.T) {
	limiter := ratelimiter.New(100, 5) // 100 tokens/sec
	defer limiter.Stop()

	// Drain all tokens.
	for range 5 {
		limiter.Allow()
	}

	// Wait for refill (at 100/sec, 1 token every 10ms).
	time.Sleep(50 * time.Millisecond)

	// Should have some tokens now.
	if !limiter.Allow() {
		t.Error("Allow() returned false after refill period")
	}
}

func TestWait_BlocksUntilToken(t *testing.T) {
	limiter := ratelimiter.New(100, 1) // 100/sec, burst 1
	defer limiter.Stop()

	// Consume the only token.
	limiter.Allow()

	// Wait should block briefly and then succeed after refill.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait() returned error: %v", err)
	}
}

func TestWait_RespectsContextCancellation(t *testing.T) {
	limiter := ratelimiter.New(0.1, 1) // Very slow: 1 token per 10 seconds
	defer limiter.Stop()

	// Drain the bucket.
	limiter.Allow()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := limiter.Wait(ctx)
	if err == nil {
		t.Fatal("Wait() should have returned context error")
	}
}

func TestTokens_ReportsCurrentCount(t *testing.T) {
	limiter := ratelimiter.New(100, 10)
	defer limiter.Stop()

	initial := limiter.Tokens()
	if initial != 10 {
		t.Errorf("initial tokens = %d, want 10", initial)
	}

	limiter.Allow()
	after := limiter.Tokens()
	if after != 9 {
		t.Errorf("after one Allow, tokens = %d, want 9", after)
	}
}

func TestAllow_ConcurrentSafety(t *testing.T) {
	// Use a very low rate so refill doesn't interfere during the test.
	// 0.1 tokens/sec = 1 token every 10 seconds — effectively no refill.
	limiter := ratelimiter.New(0.1, 100)
	defer limiter.Stop()

	// Run 200 goroutines trying to get tokens simultaneously.
	// Only 100 should succeed (burst capacity, no refill during test).
	var allowed atomic.Int64
	var wg sync.WaitGroup

	for range 200 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.Allow() {
				allowed.Add(1)
			}
		}()
	}

	wg.Wait()

	got := allowed.Load()
	if got != 100 {
		t.Errorf("allowed = %d, want exactly 100 (burst capacity)", got)
	}
}

func TestBucketDoesNotExceedCapacity(t *testing.T) {
	limiter := ratelimiter.New(1000, 5) // Fast refill, small bucket
	defer limiter.Stop()

	// Wait for many refill cycles.
	time.Sleep(50 * time.Millisecond)

	// Tokens should not exceed capacity.
	tokens := limiter.Tokens()
	if tokens > 5 {
		t.Errorf("tokens = %d, exceeds capacity 5", tokens)
	}
}

// BenchmarkAllow measures the performance of Allow() under contention.
// This is important because the rate limiter is in the hot path of event processing.
func BenchmarkAllow(b *testing.B) {
	limiter := ratelimiter.New(1_000_000, int64(b.N)) // Large burst for benchmark.
	defer limiter.Stop()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow()
		}
	})
}
