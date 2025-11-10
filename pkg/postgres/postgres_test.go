package postgres_test

import (
	"testing"

	"github.com/AudreyRodrygo/Sentinel/pkg/postgres"
)

func TestConfig_DSN(t *testing.T) { //nolint:gosec // G101: test credentials, not real
	tests := []struct {
		name string
		cfg  postgres.Config
		want string
	}{
		{
			name: "default local config",
			cfg: postgres.Config{
				Host:     "localhost",
				Port:     5432,
				Database: "sentinel",
				User:     "sentinel",
				Password: "sentinel",
			},
			want: "postgres://sentinel:sentinel@localhost:5432/sentinel",
		},
		{
			name: "custom host and port",
			cfg: postgres.Config{
				Host:     "db.example.com",
				Port:     5433,
				Database: "mydb",
				User:     "admin",
				Password: "secret",
			},
			want: "postgres://admin:secret@db.example.com:5433/mydb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.DSN()
			if got != tt.want {
				t.Errorf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Integration tests for NewPool and Migrate require a running PostgreSQL instance.
// They are gated behind the "integration" build tag and run via:
//   make test-integration
//
// See postgres_integration_test.go (will be added when testcontainers is set up).
