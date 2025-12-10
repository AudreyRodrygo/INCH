// Package agent implements the sentinel-agent — a lightweight event collector
// that runs on monitored hosts.
//
// The agent collects security events from multiple sources (log files,
// process table, network connections), batches them, and sends to the
// event-collector via gRPC.
//
// Design goals:
//   - Minimal resource footprint (<15MB binary, <50MB RAM)
//   - Resilient: disk buffer when collector is unavailable
//   - Cross-platform: works on Linux and macOS
//
// Architecture:
//
//	[Log Tailer] ──┐
//	[Proc Watch] ──┼──▶ [Batcher] ──gRPC──▶ Collector
//	[Net Monitor]──┘        │
//	                   flush every 5s
//	                   or every 100 events
package agent

// Config holds sentinel-agent configuration.
type Config struct {
	// CollectorAddr is the gRPC address of the event-collector.
	CollectorAddr string `mapstructure:"collector_addr"`

	// AgentID uniquely identifies this agent instance.
	AgentID string `mapstructure:"agent_id"`

	// LogFiles is a list of log files to tail.
	LogFiles []string `mapstructure:"log_files"`

	// BatchSize is the max events per batch before sending.
	BatchSize int `mapstructure:"batch_size"`

	// FlushInterval is the max time between batch sends (seconds).
	FlushIntervalSec int `mapstructure:"flush_interval_sec"`

	// EnableProcessWatch enables process start/stop monitoring.
	EnableProcessWatch bool `mapstructure:"enable_process_watch"`

	// ProcessWatchIntervalSec is the polling interval for process changes.
	ProcessWatchIntervalSec int `mapstructure:"process_watch_interval_sec"`

	LogLevel    string `mapstructure:"log_level"`
	Development bool   `mapstructure:"development"`
}

// Defaults returns development defaults.
func Defaults() Config {
	return Config{
		CollectorAddr:           "localhost:50051",
		AgentID:                 "agent-01",
		LogFiles:                []string{"/var/log/auth.log"},
		BatchSize:               100,
		FlushIntervalSec:        5,
		EnableProcessWatch:      true,
		ProcessWatchIntervalSec: 10,
		LogLevel:                "info",
		Development:             true,
	}
}
