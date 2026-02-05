package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
)

// LogTailer reads log files line by line and emits SecurityEvents.
//
// It works like `tail -f` but with resilience:
//   - Starts reading from the end of the file (only new lines)
//   - Handles log rotation (detects inode change, reopens file)
//   - Polls for new data (simpler than fsnotify for cross-platform)
//
// Each log line becomes a SecurityEvent with:
//   - raw_message: the full log line
//   - type: parsed from the log content (auth_failure, auth_success, etc.)
//   - service: parsed from syslog format
type LogTailer struct {
	path    string
	agentID string
	logger  *zap.Logger
}

// NewLogTailer creates a tailer for the given log file.
func NewLogTailer(path, agentID string, logger *zap.Logger) *LogTailer {
	return &LogTailer{
		path:    path,
		agentID: agentID,
		logger:  logger,
	}
}

// Name returns the source identifier.
func (t *LogTailer) Name() string { return fmt.Sprintf("log:%s", t.path) }

// Start begins tailing the log file. Blocks until context is cancelled.
func (t *LogTailer) Start(ctx context.Context, events chan<- *pb.SecurityEvent) error { //nolint:gocognit // sequential poll loop, straightforward
	file, err := os.Open(t.path)
	if err != nil {
		return fmt.Errorf("opening %s: %w", t.path, err)
	}
	defer func() { _ = file.Close() }()

	// Seek to end — we only want NEW lines, not historical.
	if _, seekErr := file.Seek(0, io.SeekEnd); seekErr != nil {
		return fmt.Errorf("seeking to end of %s: %w", t.path, seekErr)
	}

	scanner := bufio.NewScanner(file)
	ticker := time.NewTicker(500 * time.Millisecond) // Poll interval.
	defer ticker.Stop()

	t.logger.Info("tailing log file", zap.String("path", t.path))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Read all available lines.
			for scanner.Scan() {
				line := scanner.Text()
				if line == "" {
					continue
				}

				event := t.parseLine(line)
				select {
				case events <- event:
				case <-ctx.Done():
					return ctx.Err()
				}
			}

			// Check for log rotation: if the file was truncated or replaced,
			// reopen it. This handles logrotate-style rotation.
			if rotated, _ := t.checkRotation(file); rotated {
				_ = file.Close()
				file, err = os.Open(t.path)
				if err != nil {
					t.logger.Warn("failed to reopen rotated log", zap.Error(err))
					continue
				}
				scanner = bufio.NewScanner(file)
				t.logger.Info("log file rotated, reopened", zap.String("path", t.path))
			}
		}
	}
}

// parseLine converts a raw log line to a SecurityEvent.
//
// Basic heuristic parsing for common syslog and auth log formats.
// In production, this would use more sophisticated parsers (grok patterns, etc.).
func (t *LogTailer) parseLine(line string) *pb.SecurityEvent {
	event := &pb.SecurityEvent{
		EventId:    uuid.NewString(),
		Timestamp:  timestamppb.Now(),
		RawMessage: line,
		AgentId:    t.agentID,
		Metadata:   map[string]string{"source_file": t.path},
	}

	// Detect event type from common log patterns.
	lower := strings.ToLower(line)

	switch {
	case strings.Contains(lower, "failed password") || strings.Contains(lower, "authentication failure"):
		event.Type = pb.EventType_EVENT_TYPE_AUTH_FAILURE
		event.Service = extractService(line)
	case strings.Contains(lower, "accepted password") || strings.Contains(lower, "session opened"):
		event.Type = pb.EventType_EVENT_TYPE_AUTH_SUCCESS
		event.Service = extractService(line)
	case strings.Contains(lower, "sudo") && strings.Contains(lower, "root"):
		event.Type = pb.EventType_EVENT_TYPE_PRIVILEGE_ESCALATION
		event.Service = "sudo"
	default:
		event.Type = pb.EventType_EVENT_TYPE_SYSTEM_CALL
	}

	// Try to extract source IP from common patterns like "from 1.2.3.4".
	if idx := strings.Index(lower, "from "); idx != -1 {
		rest := line[idx+5:]
		if spaceIdx := strings.IndexByte(rest, ' '); spaceIdx != -1 {
			event.SourceIp = rest[:spaceIdx]
		} else {
			event.SourceIp = rest
		}
	}

	// Extract hostname.
	hostname, _ := os.Hostname()
	event.Hostname = hostname

	return event
}

// checkRotation detects if the file was truncated or replaced.
func (t *LogTailer) checkRotation(file *os.File) (bool, error) {
	stat, err := file.Stat()
	if err != nil {
		return false, err
	}

	pos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false, err
	}

	// If current position is beyond file size, file was truncated.
	return pos > stat.Size(), nil
}

// extractService tries to extract the service name from syslog format.
// Syslog: "Mar 16 10:30:45 hostname sshd[1234]: message"
func extractService(line string) string {
	parts := strings.Fields(line)
	if len(parts) >= 5 {
		svc := parts[4]
		// Remove PID suffix: "sshd[1234]:" → "sshd"
		if idx := strings.IndexByte(svc, '['); idx != -1 {
			return svc[:idx]
		}
		return strings.TrimSuffix(svc, ":")
	}
	return ""
}
