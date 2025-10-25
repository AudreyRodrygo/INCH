package redisutil_test

import (
	"testing"

	"github.com/AudreyRodrygo/Sentinel/pkg/redisutil"
)

func TestConfig_DefaultAddr(t *testing.T) {
	cfg := redisutil.Config{}
	if cfg.Addr != "" {
		t.Errorf("default Addr should be empty (set by NewClient), got %q", cfg.Addr)
	}
}

// Integration tests for NewClient require a running Redis instance.
// They are gated behind the "integration" build tag.
