package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/AudreyRodrygo/Inch/gen/inch/v1"
)

// ProcessWatcher monitors process start/stop events.
//
// It periodically scans the process table and compares with the previous
// snapshot. New processes = EVENT_TYPE_PROCESS_START, gone processes =
// EVENT_TYPE_PROCESS_STOP.
//
// On Linux, it reads /proc/[pid]/cmdline.
// On macOS, it uses os.FindProcess as a simplified approach.
//
// This is a polling approach — simpler and more portable than kernel-level
// monitoring (eBPF, kqueue). For most security use cases, a 10-second
// polling interval is sufficient to detect suspicious process launches.
type ProcessWatcher struct {
	interval time.Duration
	agentID  string
	logger   *zap.Logger
}

// NewProcessWatcher creates a process watcher.
func NewProcessWatcher(intervalSec int, agentID string, logger *zap.Logger) *ProcessWatcher {
	if intervalSec <= 0 {
		intervalSec = 10
	}
	return &ProcessWatcher{
		interval: time.Duration(intervalSec) * time.Second,
		agentID:  agentID,
		logger:   logger,
	}
}

// Name returns the source identifier.
func (pw *ProcessWatcher) Name() string { return "process-watcher" }

// Start begins polling the process table. Blocks until context is cancelled.
func (pw *ProcessWatcher) Start(ctx context.Context, events chan<- *pb.SecurityEvent) error { //nolint:gocognit // sequential poll loop
	ticker := time.NewTicker(pw.interval)
	defer ticker.Stop()

	previousPIDs := pw.scanProcesses()
	pw.logger.Info("process watcher started",
		zap.Int("initial_processes", len(previousPIDs)),
		zap.Duration("interval", pw.interval),
	)

	hostname, _ := os.Hostname()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			currentPIDs := pw.scanProcesses()

			// Detect new processes (in current but not in previous).
			for pid, cmd := range currentPIDs {
				if _, existed := previousPIDs[pid]; !existed {
					event := &pb.SecurityEvent{
						EventId:   uuid.NewString(),
						Type:      pb.EventType_EVENT_TYPE_PROCESS_START,
						Timestamp: timestamppb.Now(),
						Hostname:  hostname,
						Process:   cmd,
						AgentId:   pw.agentID,
						Metadata: map[string]string{
							"pid": strconv.Itoa(pid),
						},
					}
					select {
					case events <- event:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}

			// Detect stopped processes (in previous but not in current).
			for pid, cmd := range previousPIDs {
				if _, exists := currentPIDs[pid]; !exists {
					event := &pb.SecurityEvent{
						EventId:   uuid.NewString(),
						Type:      pb.EventType_EVENT_TYPE_PROCESS_STOP,
						Timestamp: timestamppb.Now(),
						Hostname:  hostname,
						Process:   cmd,
						AgentId:   pw.agentID,
						Metadata: map[string]string{
							"pid": strconv.Itoa(pid),
						},
					}
					select {
					case events <- event:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}

			previousPIDs = currentPIDs
		}
	}
}

// scanProcesses returns a map of PID → command name for all running processes.
func (pw *ProcessWatcher) scanProcesses() map[int]string {
	if runtime.GOOS == "linux" {
		return pw.scanProcFS()
	}
	// macOS / other: return empty (simplified; production would use sysctl).
	return make(map[int]string)
}

// scanProcFS reads /proc to enumerate processes (Linux only).
func (pw *ProcessWatcher) scanProcFS() map[int]string {
	procs := make(map[int]string)

	entries, err := os.ReadDir("/proc")
	if err != nil {
		pw.logger.Debug("cannot read /proc", zap.Error(err))
		return procs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // Not a PID directory.
		}

		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue // Process may have exited between readdir and readfile.
		}

		// cmdline uses null bytes as separators.
		cmd := strings.ReplaceAll(string(cmdline), "\x00", " ")
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			cmd = fmt.Sprintf("[pid:%d]", pid)
		}

		procs[pid] = cmd
	}

	return procs
}
