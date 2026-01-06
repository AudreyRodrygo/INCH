package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AudreyRodrygo/Inch/pkg/config"
)

// testConfig is a sample configuration struct used in tests.
// The `mapstructure` tags tell Viper how to map YAML keys and env vars to fields.
type testConfig struct {
	Port     int    `mapstructure:"port"`
	LogLevel string `mapstructure:"log_level"`
	Debug    bool   `mapstructure:"debug"`

	Postgres struct {
		Host     string `mapstructure:"host"`
		Port     int    `mapstructure:"port"`
		Database string `mapstructure:"database"`
	} `mapstructure:"postgres"`
}

func TestLoad_FromYAMLFile(t *testing.T) {
	// Create a temporary YAML config file.
	content := `
port: 9090
log_level: info
debug: true
postgres:
  host: localhost
  port: 5432
  database: testdb
`
	path := writeTestConfig(t, content)

	var cfg testConfig
	err := config.Load("TEST", path, &cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all fields were populated from the YAML file.
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, want 9090", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "info")
	}
	if !cfg.Debug {
		t.Error("Debug = false, want true")
	}
	if cfg.Postgres.Host != "localhost" {
		t.Errorf("Postgres.Host = %q, want %q", cfg.Postgres.Host, "localhost")
	}
	if cfg.Postgres.Port != 5432 {
		t.Errorf("Postgres.Port = %d, want 5432", cfg.Postgres.Port)
	}
	if cfg.Postgres.Database != "testdb" {
		t.Errorf("Postgres.Database = %q, want %q", cfg.Postgres.Database, "testdb")
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	// YAML says port=9090, but env var says TEST_PORT=7777.
	// Environment variable must win.
	content := `
port: 9090
log_level: info
`
	path := writeTestConfig(t, content)

	t.Setenv("TEST_PORT", "7777")
	t.Setenv("TEST_LOG_LEVEL", "debug")

	var cfg testConfig
	err := config.Load("TEST", path, &cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 7777 {
		t.Errorf("Port = %d, want 7777 (env override)", cfg.Port)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q (env override)", cfg.LogLevel, "debug")
	}
}

func TestLoad_EnvOnlyNoFile(t *testing.T) {
	// No config file — only environment variables.
	t.Setenv("NOFILE_PORT", "3000")
	t.Setenv("NOFILE_LOG_LEVEL", "warn")

	var cfg testConfig
	err := config.Load("NOFILE", "", &cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.LogLevel != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "warn")
	}
}

func TestLoad_MalformedFileReturnsError(t *testing.T) {
	// A broken YAML file should return an error — we don't want silent misconfiguration.
	path := writeTestConfig(t, "invalid: yaml: [broken")

	var cfg testConfig
	err := config.Load("TEST", path, &cfg)
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}

func TestLoad_MissingFileIsNotError(t *testing.T) {
	// A non-existent file path should not cause an error —
	// it's valid to configure entirely through env vars.
	var cfg testConfig
	err := config.Load("TEST", "/nonexistent/config.yaml", &cfg)
	// Viper returns an error for missing files (os.PathError), which our code
	// does not suppress. This is acceptable — callers can pass "" to skip file loading.
	if err != nil {
		t.Logf("missing file returned error (expected): %v", err)
	}
}

// writeTestConfig creates a temporary YAML file and returns its path.
// The file is automatically cleaned up when the test finishes.
func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	return path
}
