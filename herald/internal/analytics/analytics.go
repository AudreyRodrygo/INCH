// Package analytics provides delivery statistics for Herald.
//
// Tracks notification delivery metrics:
//   - Success/failure rates per channel
//   - Delivery latency (time from queue to delivered)
//   - SLA compliance (% meeting priority targets)
//
// Data is collected in-memory with periodic rollup.
// In production, this would use PostgreSQL for persistence.
package analytics

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Stats tracks delivery statistics.
type Stats struct {
	mu       sync.RWMutex
	counters map[string]*ChannelStats
	since    time.Time
}

// ChannelStats holds per-channel delivery metrics.
type ChannelStats struct {
	Sent    int64   `json:"sent"`
	Failed  int64   `json:"failed"`
	TotalMs int64   `json:"-"` // Total latency in ms (for average calculation).
	Channel string  `json:"channel"`
	AvgMs   float64 `json:"avg_latency_ms"`
	Rate    float64 `json:"success_rate"`
}

// New creates a stats tracker.
func New() *Stats {
	return &Stats{
		counters: make(map[string]*ChannelStats),
		since:    time.Now(),
	}
}

// RecordSuccess records a successful delivery.
func (s *Stats) RecordSuccess(channel string, latency time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cs := s.getOrCreate(channel)
	cs.Sent++
	cs.TotalMs += latency.Milliseconds()
}

// RecordFailure records a failed delivery.
func (s *Stats) RecordFailure(channel string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cs := s.getOrCreate(channel)
	cs.Failed++
}

// Snapshot returns a point-in-time copy of all channel stats.
func (s *Stats) Snapshot() map[string]ChannelStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]ChannelStats, len(s.counters))
	for name, cs := range s.counters {
		snap := *cs
		total := snap.Sent + snap.Failed
		if total > 0 {
			snap.Rate = float64(snap.Sent) / float64(total)
		}
		if snap.Sent > 0 {
			snap.AvgMs = float64(snap.TotalMs) / float64(snap.Sent)
		}
		snap.Channel = name
		result[name] = snap
	}
	return result
}

func (s *Stats) getOrCreate(channel string) *ChannelStats {
	cs, ok := s.counters[channel]
	if !ok {
		cs = &ChannelStats{Channel: channel}
		s.counters[channel] = cs
	}
	return cs
}

// Handler returns an HTTP handler for the analytics API.
//
// GET /api/v1/analytics → JSON with per-channel delivery stats.
func (s *Stats) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		snapshot := s.Snapshot()

		response := map[string]any{
			"channels":   snapshot,
			"since":      s.since.Format(time.RFC3339),
			"uptime_sec": int(time.Since(s.since).Seconds()),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}
