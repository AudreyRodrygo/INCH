package natsutil_test

import (
	"testing"

	"github.com/AudreyRodrygo/Inch/pkg/natsutil"
)

func TestConfig_DefaultURL(t *testing.T) {
	cfg := natsutil.Config{}
	if cfg.URL != "" {
		t.Errorf("default URL should be empty (set by Connect), got %q", cfg.URL)
	}
}

// Integration tests for Connect and EnsureStream require a running NATS server.
// They are gated behind the "integration" build tag and run via:
//   make test-integration
