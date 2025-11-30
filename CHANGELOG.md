# Changelog

Development log — tracks feature additions as they were implemented.

- 2025-10-14: feat(pkg): add logger interface based on zap
- 2025-10-15: feat(pkg): implement config loading from env
- 2025-10-16: feat(pkg): add graceful shutdown coordinator
- 2025-10-16: feat(pkg): add prometheus metrics registry wrapper
- 2025-10-19: feat(pkg): add postgres connection pool with pgx
- 2025-10-21: feat(pkg): add redis client wrapper with retry
- 2025-10-22: feat(pkg): implement token bucket rate limiter
- 2025-10-23: feat(pkg): implement circuit breaker with half-open state
- 2025-10-26: feat(pkg): add kafka consumer group with franz-go
- 2025-10-27: feat(pkg): add kafka producer with idempotent writes
- 2025-10-29: feat(pkg): add NATS JetStream consumer wrapper
- 2025-10-30: feat(pkg): implement DLQ with dead letter republish logic
- 2025-11-03: feat(collector): scaffold event-collector service
- 2025-11-04: feat(collector): implement gRPC server for event ingestion
- 2025-11-05: feat(proto): add event.proto schema
- 2025-11-05: feat(proto): generate protobuf Go bindings
- 2025-11-06: feat(collector): add kafka producer for event forwarding
- 2025-11-07: feat(processor): scaffold event-processor service
- 2025-11-08: feat(processor): implement kafka consumer with worker pool
- 2025-11-11: feat(processor): add GeoIP enrichment step
- 2025-11-12: feat(processor): implement severity classifier
- 2025-11-19: feat(rules): design rule interface and evaluator
- 2025-11-20: feat(rules): implement threshold rule
- 2025-11-21: feat(rules): implement anomaly detection rule
- 2025-11-22: feat(rules): implement correlation window rule
- 2025-11-26: feat(alertmgr): scaffold alert-manager service
- 2025-11-27: feat(alertmgr): implement alert deduplication
- 2025-11-28: feat(alertmgr): add alert routing by severity
- 2025-11-30: feat(dispatcher): scaffold notification-dispatcher
