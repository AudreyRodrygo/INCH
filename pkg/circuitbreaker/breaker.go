// Package circuitbreaker implements the Circuit Breaker pattern to prevent
// cascading failures in distributed systems.
//
// When a downstream service fails repeatedly, the circuit breaker "opens"
// and rejects requests immediately — no more wasted time on timeouts.
// After a cooldown period, it lets one request through ("half-open") to
// test if the service has recovered.
//
// State machine:
//
//	CLOSED ──(threshold failures)──▶ OPEN ──(timeout)──▶ HALF-OPEN
//	  ▲                                                      │
//	  │              success                                 │
//	  └──────────────────────────────────────────────────────┘
//	                        │ failure → back to OPEN
//
// This is a custom implementation without external libraries — intentionally,
// to demonstrate understanding of the pattern on interviews.
//
// Usage:
//
//	cb := circuitbreaker.New(circuitbreaker.Config{
//	    Threshold: 5,              // Open after 5 consecutive failures.
//	    Timeout:   30*time.Second, // Try half-open after 30s.
//	})
//
//	err := cb.Execute(func() error {
//	    return callDownstreamService()
//	})
//	// err may be ErrCircuitOpen if the breaker is open.
package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

// Circuit breaker states.
const (
	StateClosed   State = iota // Normal operation — requests pass through.
	StateOpen                  // Failure threshold reached — requests are rejected.
	StateHalfOpen              // Testing recovery — one request allowed.
)

// String returns a human-readable state name (useful for logging and metrics).
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// ErrCircuitOpen is returned when the circuit breaker is open and
// the request is rejected without calling the downstream service.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Config holds the circuit breaker parameters.
type Config struct {
	// Threshold is the number of consecutive failures before the circuit opens.
	// Default: 5.
	Threshold int

	// Timeout is how long the circuit stays open before transitioning to half-open.
	// Default: 30 seconds.
	Timeout time.Duration
}

// Breaker implements the Circuit Breaker pattern.
//
// It is safe for concurrent use by multiple goroutines.
type Breaker struct {
	mu sync.Mutex // Protects all mutable state below.

	state       State     // Current state of the circuit.
	failures    int       // Consecutive failure count.
	lastFailure time.Time // When the last failure occurred.

	threshold int           // Max consecutive failures before opening.
	timeout   time.Duration // Time in open state before trying half-open.

	// onStateChange is called when the state transitions.
	// Useful for logging and Prometheus metrics.
	onStateChange func(from, to State)
}

// New creates a circuit breaker with the given configuration.
func New(cfg Config) *Breaker {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &Breaker{
		state:     StateClosed,
		threshold: cfg.Threshold,
		timeout:   cfg.Timeout,
	}
}

// OnStateChange registers a callback for state transitions.
// Useful for logging ("circuit opened!") and metrics (circuit_breaker_state gauge).
func (b *Breaker) OnStateChange(fn func(from, to State)) {
	b.mu.Lock()
	b.onStateChange = fn
	b.mu.Unlock()
}

// Execute runs the given function through the circuit breaker.
//
// If the circuit is:
//   - Closed: fn is called. If it fails, failure count increases.
//   - Open: fn is NOT called. Returns ErrCircuitOpen immediately.
//   - Half-Open: fn is called as a test. Success → Closed. Failure → Open.
func (b *Breaker) Execute(fn func() error) error {
	b.mu.Lock()

	switch b.state {
	case StateOpen:
		// Check if enough time has passed to try half-open.
		if time.Since(b.lastFailure) > b.timeout {
			b.setState(StateHalfOpen)
			b.mu.Unlock()
			return b.tryHalfOpen(fn)
		}
		b.mu.Unlock()
		return ErrCircuitOpen

	case StateHalfOpen:
		// Already in half-open — only one request at a time.
		// Additional requests are rejected to prevent overloading
		// a potentially recovering service.
		b.mu.Unlock()
		return ErrCircuitOpen

	default: // StateClosed
		b.mu.Unlock()
		return b.tryClosed(fn)
	}
}

// tryClosed executes fn in the closed state.
// On success: reset failure count. On failure: increment and maybe open.
func (b *Breaker) tryClosed(fn func() error) error {
	err := fn()

	b.mu.Lock()
	defer b.mu.Unlock()

	if err == nil {
		b.failures = 0
		return nil
	}

	b.failures++
	b.lastFailure = time.Now()

	if b.failures >= b.threshold {
		b.setState(StateOpen)
	}

	return err
}

// tryHalfOpen executes fn as a recovery test.
// On success: close the circuit. On failure: reopen it.
func (b *Breaker) tryHalfOpen(fn func() error) error {
	err := fn()

	b.mu.Lock()
	defer b.mu.Unlock()

	if err == nil {
		b.failures = 0
		b.setState(StateClosed)
		return nil
	}

	b.failures++
	b.lastFailure = time.Now()
	b.setState(StateOpen)

	return err
}

// State returns the current circuit breaker state.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Failures returns the current consecutive failure count.
func (b *Breaker) Failures() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failures
}

// setState transitions to a new state and fires the callback.
// Caller must hold b.mu.
func (b *Breaker) setState(to State) {
	from := b.state
	if from == to {
		return
	}

	b.state = to

	if b.onStateChange != nil {
		// Fire callback without holding the lock to prevent deadlocks.
		fn := b.onStateChange
		go fn(from, to)
	}
}
