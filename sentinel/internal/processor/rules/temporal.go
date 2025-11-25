package rules

import (
	"sync"
	"time"
)

// WindowTracker maintains sliding windows for threshold-based correlation.
//
// For each rule + group_by combination (e.g., brute_force + source_ip=1.2.3.4),
// it tracks event timestamps in a sliding window. When the count exceeds the
// threshold within the window duration, it fires.
//
// Example: rule "5 auth failures from same IP in 60s"
//
//	Key: "brute_force|1.2.3.4"
//	Window: [t-60s ... t]
//	Events arrive: t=0s, t=15s, t=30s, t=45s, t=50s ← 5th event → FIRE!
//
// Memory management:
//   - Old timestamps are pruned on every check (lazy cleanup)
//   - Groups with no recent activity are periodically garbage collected
//
// Thread safety: uses sync.Mutex per group to minimize contention.
// Different IPs can be processed concurrently without blocking each other.
type WindowTracker struct {
	mu      sync.Mutex
	windows map[string]*slidingWindow // Key: "rule_id|group_key"
}

// slidingWindow holds timestamps of events within the window period.
type slidingWindow struct {
	timestamps []time.Time
}

// NewWindowTracker creates a tracker for temporal correlation.
func NewWindowTracker() *WindowTracker {
	return &WindowTracker{
		windows: make(map[string]*slidingWindow),
	}
}

// AddAndCheck records an event timestamp and returns true if the threshold
// has been reached within the window duration.
//
// Parameters:
//   - key: composite key "rule_id|group_key" (e.g., "brute_force|1.2.3.4")
//   - now: timestamp of the current event
//   - window: duration of the sliding window (e.g., 60s)
//   - threshold: minimum count to trigger (e.g., 5)
//
// Returns:
//   - fired: true if threshold was reached (alert should be generated)
//   - count: current number of events in the window
func (wt *WindowTracker) AddAndCheck(key string, now time.Time, window time.Duration, threshold int) (fired bool, count int) {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	sw, exists := wt.windows[key]
	if !exists {
		sw = &slidingWindow{}
		wt.windows[key] = sw
	}

	// Prune timestamps outside the window.
	// This is O(n) but n is small (bounded by threshold × constant).
	cutoff := now.Add(-window)
	pruned := sw.timestamps[:0] // Reuse underlying array.
	for _, ts := range sw.timestamps {
		if ts.After(cutoff) {
			pruned = append(pruned, ts)
		}
	}
	sw.timestamps = pruned

	// Add the current event.
	sw.timestamps = append(sw.timestamps, now)

	count = len(sw.timestamps)
	fired = count >= threshold

	// If fired, reset the window to prevent repeated alerts
	// for the same burst. The next alert requires a fresh set of events.
	if fired {
		sw.timestamps = sw.timestamps[:0]
	}

	return fired, count
}

// Cleanup removes windows with no recent activity.
// Call periodically (e.g., every minute) to prevent memory leaks
// from abandoned group keys (IPs that stopped sending events).
func (wt *WindowTracker) Cleanup(maxAge time.Duration) {
	wt.mu.Lock()
	defer wt.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for key, sw := range wt.windows {
		if len(sw.timestamps) == 0 {
			delete(wt.windows, key)
			continue
		}
		// Check if the newest timestamp is older than maxAge.
		newest := sw.timestamps[len(sw.timestamps)-1]
		if newest.Before(cutoff) {
			delete(wt.windows, key)
		}
	}
}

// Size returns the number of tracked windows (for metrics).
func (wt *WindowTracker) Size() int {
	wt.mu.Lock()
	defer wt.mu.Unlock()
	return len(wt.windows)
}
