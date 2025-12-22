// Package replay implements the Forensics Replay API for Sentinel.
//
// Replay allows security analysts to re-run correlation rules against
// historical events stored in Kafka. This is critical for incident
// investigation: "Were there signs of this attack 3 days ago?"
//
// How it works:
//  1. Analyst specifies a time range and optionally custom rules
//  2. Replay reads events from Kafka starting at the given timestamp
//  3. Each event is evaluated against the rule engine
//  4. Matches are streamed back via Server-Sent Events (SSE)
//
// This is possible because Kafka retains events (7 days by default).
// Unlike a database query, replay applies the FULL rule engine including
// temporal correlation and sequence detection — catching patterns that
// a simple grep would miss.
//
// For the diploma: this demonstrates forensic analysis capability —
// the ability to retrospectively analyze security events with new
// detection rules.
package replay

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"

	pb "github.com/AudreyRodrygo/Sentinel/gen/sentinel/v1"
	"github.com/AudreyRodrygo/Sentinel/sentinel/internal/processor/rules"
)

// Request defines the replay parameters.
type Request struct {
	// StartTime is the beginning of the replay window.
	StartTime time.Time `json:"start_time"`

	// EndTime is the end of the replay window.
	EndTime time.Time `json:"end_time"`

	// RulesDir overrides the default rules directory (optional).
	// If empty, uses the currently loaded rules.
	RulesDir string `json:"rules_dir"`
}

// Result is a single match found during replay.
type Result struct {
	RuleID     string    `json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	Severity   string    `json:"severity"`
	GroupKey   string    `json:"group_key"`
	EventCount int       `json:"event_count"`
	Timestamp  time.Time `json:"timestamp"`
}

// Handler serves the replay HTTP API.
type Handler struct {
	brokers []string
	topic   string
	engine  *rules.Engine
	logger  *zap.Logger
}

// NewHandler creates a replay API handler.
func NewHandler(brokers []string, topic string, engine *rules.Engine, logger *zap.Logger) *Handler {
	return &Handler{
		brokers: brokers,
		topic:   topic,
		engine:  engine,
		logger:  logger,
	}
}

// HandleReplay processes a replay request and streams results via SSE.
//
// SSE (Server-Sent Events) is a simple one-way streaming protocol:
// the server sends events as they're found, and the client receives
// them in real-time. Simpler than WebSocket for this use case.
//
// GET /api/v1/replay?start=2024-03-10T00:00:00Z&end=2024-03-13T00:00:00Z
func (h *Handler) HandleReplay(w http.ResponseWriter, r *http.Request) { //nolint:gocognit,cyclop // SSE handler has inherent complexity
	startStr := r.URL.Query().Get("start")
	endStr := r.URL.Query().Get("end")

	if startStr == "" || endStr == "" {
		http.Error(w, `{"error":"start and end query params required"}`, http.StatusBadRequest)
		return
	}

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		http.Error(w, `{"error":"invalid start time format, use RFC3339"}`, http.StatusBadRequest)
		return
	}

	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		http.Error(w, `{"error":"invalid end time format, use RFC3339"}`, http.StatusBadRequest)
		return
	}

	h.logger.Info("starting replay",
		zap.Time("start", startTime),
		zap.Time("end", endTime),
	)

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create a temporary Kafka consumer that reads from the start timestamp.
	// This consumer is NOT part of a consumer group — it reads independently
	// without affecting the production consumer's offsets.
	client, err := kgo.NewClient(
		kgo.SeedBrokers(h.brokers...),
		kgo.ConsumeTopics(h.topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		h.logger.Error("failed to create replay consumer", zap.Error(err))
		_, _ = fmt.Fprintf(w, "data: {\"error\":\"failed to create consumer\"}\n\n")
		flusher.Flush()
		return
	}
	defer client.Close()

	// Create a fresh rule engine for replay (isolated state).
	replayEngine := rules.New(h.engine.Rules(), zap.NewNop())

	ctx := r.Context()
	var totalEvents, totalMatches int

	// Send start event.
	_, _ = fmt.Fprintf(w, "data: {\"status\":\"started\",\"start\":%q,\"end\":%q}\n\n", //nolint:gosec // SSE data, not HTML
		startTime.Format(time.RFC3339), endTime.Format(time.RFC3339))
	flusher.Flush()

	for {
		fetches := client.PollFetches(ctx)
		if ctx.Err() != nil {
			break
		}

		if errs := fetches.Errors(); len(errs) > 0 {
			break
		}

		var done bool
		fetches.EachRecord(func(record *kgo.Record) {
			if done {
				return
			}

			// Skip events outside our time window.
			if record.Timestamp.Before(startTime) {
				return
			}
			if record.Timestamp.After(endTime) {
				done = true
				return
			}

			var event pb.SecurityEvent
			if unmarshalErr := proto.Unmarshal(record.Value, &event); unmarshalErr != nil {
				return
			}

			totalEvents++

			// Evaluate against replay rule engine.
			results := replayEngine.Evaluate(&event)
			for _, result := range results {
				totalMatches++

				match := Result{
					RuleID:     result.Rule.ID,
					RuleName:   result.Rule.Name,
					Severity:   result.Rule.Severity,
					GroupKey:   result.GroupKey,
					EventCount: result.EventCount,
					Timestamp:  record.Timestamp,
				}

				data, _ := json.Marshal(match)
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		})

		if done {
			break
		}
	}

	// Send completion event.
	_, _ = fmt.Fprintf(w, "data: {\"status\":\"completed\",\"total_events\":%d,\"total_matches\":%d}\n\n",
		totalEvents, totalMatches)
	flusher.Flush()

	h.logger.Info("replay completed",
		zap.Int("events_scanned", totalEvents),
		zap.Int("matches_found", totalMatches),
	)
}
