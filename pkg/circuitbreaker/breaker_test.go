package circuitbreaker_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/AudreyRodrygo/Sentinel/pkg/circuitbreaker"
)

var errService = errors.New("service unavailable")

func TestClosed_PassesThrough(t *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.Config{Threshold: 5})

	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("function was not called")
	}
	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("state = %v, want Closed", cb.State())
	}
}

func TestClosed_OpensAfterThreshold(t *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.Config{
		Threshold: 3,
		Timeout:   1 * time.Second,
	})

	// 3 consecutive failures should open the circuit.
	for range 3 {
		_ = cb.Execute(func() error { return errService })
	}

	if cb.State() != circuitbreaker.StateOpen {
		t.Errorf("state = %v, want Open after %d failures", cb.State(), 3)
	}

	// Next call should return ErrCircuitOpen without calling fn.
	called := false
	err := cb.Execute(func() error {
		called = true
		return nil
	})

	if !errors.Is(err, circuitbreaker.ErrCircuitOpen) {
		t.Errorf("err = %v, want ErrCircuitOpen", err)
	}
	if called {
		t.Error("function should NOT be called when circuit is open")
	}
}

func TestClosed_ResetsOnSuccess(t *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.Config{Threshold: 3})

	// 2 failures (below threshold).
	for range 2 {
		_ = cb.Execute(func() error { return errService })
	}

	// 1 success — should reset failure count.
	_ = cb.Execute(func() error { return nil })

	if cb.Failures() != 0 {
		t.Errorf("failures = %d after success, want 0", cb.Failures())
	}

	// 2 more failures should NOT open (counter was reset).
	for range 2 {
		_ = cb.Execute(func() error { return errService })
	}

	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("state = %v, want Closed (failure count was reset)", cb.State())
	}
}

func TestOpen_TransitionsToHalfOpen(t *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.Config{
		Threshold: 1,
		Timeout:   50 * time.Millisecond, // Short timeout for testing.
	})

	// Trip the breaker.
	_ = cb.Execute(func() error { return errService })
	if cb.State() != circuitbreaker.StateOpen {
		t.Fatalf("state = %v, want Open", cb.State())
	}

	// Wait for timeout.
	time.Sleep(60 * time.Millisecond)

	// Next call should transition to half-open and try the function.
	err := cb.Execute(func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected error in half-open: %v", err)
	}

	// Success in half-open → should close.
	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("state = %v, want Closed after successful half-open", cb.State())
	}
}

func TestHalfOpen_ReopensOnFailure(t *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.Config{
		Threshold: 1,
		Timeout:   50 * time.Millisecond,
	})

	// Trip the breaker.
	_ = cb.Execute(func() error { return errService })

	// Wait for timeout → half-open.
	time.Sleep(60 * time.Millisecond)

	// Fail in half-open → should reopen.
	_ = cb.Execute(func() error { return errService })

	if cb.State() != circuitbreaker.StateOpen {
		t.Errorf("state = %v, want Open after half-open failure", cb.State())
	}
}

func TestOnStateChange_IsCalled(t *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.Config{
		Threshold: 2,
		Timeout:   50 * time.Millisecond,
	})

	var mu sync.Mutex
	transitions := make([]string, 0)

	cb.OnStateChange(func(from, to circuitbreaker.State) {
		mu.Lock()
		transitions = append(transitions, from.String()+"->"+to.String())
		mu.Unlock()
	})

	// Trip the breaker (closed → open).
	_ = cb.Execute(func() error { return errService })
	_ = cb.Execute(func() error { return errService })

	// Wait for callback goroutine + timeout.
	time.Sleep(70 * time.Millisecond)

	// Half-open → closed (success).
	_ = cb.Execute(func() error { return nil })

	time.Sleep(20 * time.Millisecond) // Wait for async callbacks.

	mu.Lock()
	defer mu.Unlock()

	if len(transitions) < 2 {
		t.Fatalf("expected at least 2 transitions, got %d: %v", len(transitions), transitions)
	}
	if transitions[0] != "closed->open" {
		t.Errorf("transition[0] = %q, want %q", transitions[0], "closed->open")
	}
}

func TestExecute_ConcurrentSafety(_ *testing.T) {
	cb := circuitbreaker.New(circuitbreaker.Config{Threshold: 100})

	var wg sync.WaitGroup
	for range 500 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cb.Execute(func() error { return nil })
		}()
	}

	wg.Wait() // Should not panic or race.
}

func BenchmarkExecute_Closed(b *testing.B) {
	cb := circuitbreaker.New(circuitbreaker.Config{Threshold: 100})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Execute(func() error { return nil })
		}
	})
}

func BenchmarkExecute_Open(b *testing.B) {
	cb := circuitbreaker.New(circuitbreaker.Config{
		Threshold: 1,
		Timeout:   1 * time.Hour, // Stay open for the benchmark.
	})
	_ = cb.Execute(func() error { return errService }) // Trip it.

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = cb.Execute(func() error { return nil })
		}
	})
}
