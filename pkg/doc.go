// Package pkg contains shared libraries used by all INCH services.
//
// Subpackages:
//   - circuitbreaker: Circuit Breaker pattern implementation
//   - ratelimiter: Token Bucket rate limiter
//   - retry: Exponential backoff with jitter
//   - observability: Logging, metrics, and tracing setup
//   - health: HTTP health check endpoints
//   - config: Configuration loading via Viper
//   - postgres: PostgreSQL connection pool and migrations
//   - redisutil: Redis client factory
//   - kafkautil: Kafka producer/consumer helpers
//   - natsutil: NATS JetStream helpers
//   - dlq: Dead Letter Queue abstraction
package pkg
