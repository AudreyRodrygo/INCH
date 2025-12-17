package gateway

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/AudreyRodrygo/Sentinel/herald/internal/gateway/priority"
)

// SendRequest is the JSON body for POST /api/v1/notifications.
type SendRequest struct {
	Priority  string            `json:"priority"` // CRITICAL, HIGH, NORMAL, LOW
	Recipient string            `json:"recipient"`
	Subject   string            `json:"subject"`
	Body      string            `json:"body"`
	Channel   string            `json:"channel"` // email, webhook, telegram, slack
	Source    string            `json:"source"`  // sentinel, manual, etc.
	Metadata  map[string]string `json:"metadata"`
}

// SendResponse is returned by POST /api/v1/notifications.
type SendResponse struct {
	NotificationID string `json:"notification_id"`
	Accepted       bool   `json:"accepted"`
	Priority       string `json:"priority"`
}

// Server handles HTTP requests for the Herald gateway.
type Server struct {
	queue  *priority.Queue
	logger *zap.Logger
}

// NewServer creates a gateway REST server.
func NewServer(queue *priority.Queue, logger *zap.Logger) *Server {
	return &Server{
		queue:  queue,
		logger: logger,
	}
}

// Router returns the chi HTTP router with all endpoints.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Middleware stack.
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// API v1.
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/notifications", s.handleSend)
		r.Get("/health", s.handleHealth)
		r.Get("/queue/stats", s.handleQueueStats)
	})

	return r
}

// handleSend queues a notification for delivery.
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Body == "" && req.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject or body is required")
		return
	}

	notifID := uuid.NewString()
	level := priority.ParseLevel(req.Priority)

	// Serialize the full request as the queue payload.
	payload, _ := json.Marshal(map[string]any{
		"notification_id": notifID,
		"priority":        level.String(),
		"recipient":       req.Recipient,
		"subject":         req.Subject,
		"body":            req.Body,
		"channel":         req.Channel,
		"source":          req.Source,
		"metadata":        req.Metadata,
	})

	s.queue.Push(priority.Item{
		ID:        notifID,
		Priority:  level,
		Payload:   payload,
		CreatedAt: time.Now(),
	})

	s.logger.Info("notification queued",
		zap.String("id", notifID),
		zap.String("priority", level.String()),
		zap.String("recipient", req.Recipient),
	)

	writeJSON(w, http.StatusAccepted, SendResponse{
		NotificationID: notifID,
		Accepted:       true,
		Priority:       level.String(),
	})
}

// handleHealth returns service status.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleQueueStats returns current queue metrics.
func (s *Server) handleQueueStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"queue_depth": s.queue.Len(),
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
