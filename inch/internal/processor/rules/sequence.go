package rules

import (
	"sync"
	"time"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
)

// SequenceTracker detects ordered event chains (kill chain patterns).
//
// A sequence rule defines multiple steps that must occur in order,
// from the same source (group_by), within a time window.
//
// Example: "SSH brute force → successful login → privilege escalation"
//
//	Step 0: auth_failure ×5    (accumulate 5 failures)
//	Step 1: auth_success ×1    (then a successful login)
//	Step 2: privilege_escalation ×1  (then escalation)
//	Window: 10 minutes, group_by: source_ip
//
// Each source IP has its own state machine:
//
//	State 0 (waiting for step 0) ──5 failures──▶ State 1 ──success──▶ State 2 ──escalation──▶ FIRE!
//
// If the window expires before completion, the state resets.
// This is significantly more powerful than simple threshold rules —
// it detects multi-stage attacks that unfold over time.
type SequenceTracker struct {
	mu     sync.Mutex
	states map[string]*sequenceState // Key: "rule_id|group_key"
}

// sequenceState tracks progress through a sequence rule for one group key.
type sequenceState struct {
	currentStep int       // Which step we're waiting for next.
	stepCounts  []int     // How many events matched each step so far.
	startedAt   time.Time // When the first step matched (for window expiration).
}

// NewSequenceTracker creates a tracker for sequence correlation.
func NewSequenceTracker() *SequenceTracker {
	return &SequenceTracker{
		states: make(map[string]*sequenceState),
	}
}

// Evaluate checks if an event advances a sequence rule's state machine.
//
// Returns true if the event completes the final step (all steps matched
// within the time window) — meaning the full attack chain was detected.
//
// Parameters:
//   - key: composite key "rule_id|group_key"
//   - event: the security event to evaluate
//   - rule: the sequence rule definition
func (st *SequenceTracker) Evaluate(key string, event *pb.SecurityEvent, rule *Rule) bool {
	st.mu.Lock()
	defer st.mu.Unlock()

	state, exists := st.states[key]

	// Check if existing state has expired.
	if exists && !state.startedAt.IsZero() && time.Since(state.startedAt) > rule.Window {
		// Window expired — reset and start fresh.
		delete(st.states, key)
		exists = false
	}

	if !exists {
		state = &sequenceState{
			stepCounts: make([]int, len(rule.Steps)),
		}
		st.states[key] = state
	}

	// Check if the event matches the current step's conditions.
	currentStep := state.currentStep
	if currentStep >= len(rule.Steps) {
		return false // All steps already completed (shouldn't happen).
	}

	step := rule.Steps[currentStep]
	if !MatchesConditions(event, step.Conditions) {
		return false // Event doesn't match current step.
	}

	// Event matches — increment the count for this step.
	state.stepCounts[currentStep]++

	// Record start time on first match.
	if state.startedAt.IsZero() {
		state.startedAt = time.Now()
	}

	// Check if this step's required count is met.
	requiredCount := step.Count
	if requiredCount <= 0 {
		requiredCount = 1
	}

	if state.stepCounts[currentStep] < requiredCount {
		return false // Need more events for this step.
	}

	// Step complete — advance to next step.
	state.currentStep++

	// Check if all steps are complete → sequence detected!
	if state.currentStep >= len(rule.Steps) {
		// Reset state for this group key.
		delete(st.states, key)
		return true // Kill chain detected!
	}

	return false
}

// Cleanup removes expired sequence states.
func (st *SequenceTracker) Cleanup(maxAge time.Duration) {
	st.mu.Lock()
	defer st.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for key, state := range st.states {
		if !state.startedAt.IsZero() && state.startedAt.Before(cutoff) {
			delete(st.states, key)
		}
	}
}

// Size returns the number of tracked sequences (for metrics).
func (st *SequenceTracker) Size() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.states)
}
