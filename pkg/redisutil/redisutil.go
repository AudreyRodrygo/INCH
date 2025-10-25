// Package redisutil provides a Redis client factory for Sentinel and Herald services.
//
// Redis serves three purposes in our system:
//   - Deduplication: SET NX with TTL to detect duplicate events
//   - Rate limiter state: atomic counters for Token Bucket algorithm
//   - Behavioral baseline: Sorted Sets for time-series data (rolling averages)
//
// Usage:
//
//	client, err := redisutil.NewClient(redisutil.Config{
//	    Addr:     "localhost:6379",
//	    Password: "",
//	    DB:       0,
//	})
//	defer client.Close()
package redisutil

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Config holds Redis connection parameters.
type Config struct {
	Addr     string `mapstructure:"addr"`     // host:port (default: "localhost:6379")
	Password string `mapstructure:"password"` // empty for no auth
	DB       int    `mapstructure:"db"`       // database number (0-15)
}

// NewClient creates a Redis client and verifies the connection.
//
// The client is safe for concurrent use — go-redis handles connection
// pooling internally (default: 10 connections per CPU).
func NewClient(ctx context.Context, cfg Config) (*redis.Client, error) {
	if cfg.Addr == "" {
		cfg.Addr = "localhost:6379"
	}

	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Verify connection works — fail fast at startup.
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pinging redis at %s: %w", cfg.Addr, err)
	}

	return client, nil
}
