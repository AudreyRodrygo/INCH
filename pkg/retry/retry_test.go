package retry_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AudreyRodrygo/Sentinel/pkg/retry"
)

func TestDo_SuccessOnFirstAttempt(t *testing.T) {
	var calls atomic.Int32

	err := retry.Do(context.Background(), func() error {
		calls.Add(1)
		return nil // Success immediately.
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestDo_SuccessOnThirdAttempt(t *testing.T) {
	var calls atomic.Int32

	err := retry.Do(context.Background(), func() error {
		n := calls.Add(1)
		if n < 3 {
			return errors.New("temporary failure")
		}
		return nil // Succeed on 3rd attempt.
	},
		retry.WithMaxAttempts(5),
		retry.WithBaseDelay(1*time.Millisecond), // Fast for tests.
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestDo_AllAttemptsFail(t *testing.T) {
	errFail := errors.New("permanent failure")

	err := retry.Do(context.Background(), func() error {
		return errFail
	},
		retry.WithMaxAttempts(3),
		retry.WithBaseDelay(1*time.Millisecond),
	)

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, retry.ErrMaxAttemptsReached) {
		t.Errorf("error should wrap ErrMaxAttemptsReached, got: %v", err)
	}
}

func TestDo_RespectsContextCancellation(t *testing.T) {
	// Cancel the context before retry can finish.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()

	err := retry.Do(ctx, func() error {
		return errors.New("always fails")
	},
		retry.WithMaxAttempts(100),
		retry.WithBaseDelay(1*time.Second), // Long delay — context should cancel first.
	)

	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got: %v", err)
	}
	// Should have exited quickly, not waited for many retries.
	if elapsed > 500*time.Millisecond {
		t.Errorf("retry took %v, should have been cancelled quickly", elapsed)
	}
}

func TestDo_ExponentialGrowth(t *testing.T) {
	// Verify that delays grow exponentially by measuring total duration.
	// With base=10ms, multiplier=2: delays are ~10ms, ~20ms, ~40ms ≈ 70ms total.
	var calls atomic.Int32

	start := time.Now()
	_ = retry.Do(context.Background(), func() error {
		calls.Add(1)
		return errors.New("fail")
	},
		retry.WithMaxAttempts(4),
		retry.WithBaseDelay(10*time.Millisecond),
		retry.WithMultiplier(2.0),
	)
	elapsed := time.Since(start)

	// Total delay should be at least 10+20+40 = 70ms (minus jitter).
	// With ±25% jitter, minimum is about 52ms.
	if elapsed < 40*time.Millisecond {
		t.Errorf("elapsed = %v, expected at least ~50ms of backoff", elapsed)
	}
	if calls.Load() != 4 {
		t.Errorf("calls = %d, want 4", calls.Load())
	}
}

func TestDo_MaxDelayCap(t *testing.T) {
	// With maxDelay=15ms and base=10ms, multiplier=2:
	// delays should be 10ms, 15ms (capped), 15ms (capped).
	start := time.Now()
	_ = retry.Do(context.Background(), func() error {
		return errors.New("fail")
	},
		retry.WithMaxAttempts(4),
		retry.WithBaseDelay(10*time.Millisecond),
		retry.WithMaxDelay(15*time.Millisecond),
		retry.WithMultiplier(2.0),
	)
	elapsed := time.Since(start)

	// Total delay should be around 10+15+15 = 40ms (±jitter).
	// Should NOT be 10+20+40 = 70ms (uncapped).
	if elapsed > 150*time.Millisecond {
		t.Errorf("elapsed = %v, maxDelay cap may not be working", elapsed)
	}
}
