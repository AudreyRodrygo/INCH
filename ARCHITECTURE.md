# Architecture & Design Decisions

## System Overview

Sentinel + Herald is a two-project platform for real-time security event processing and intelligent notification delivery.

**Sentinel** processes the event pipeline: collection → enrichment → correlation → alerting.
**Herald** handles notification delivery: prioritization → routing → multi-channel delivery.

## Why Two Separate Projects?

Separation of concerns with different scaling profiles:
- Sentinel processes 10k+ events/sec — CPU-bound, scales horizontally via Kafka partitions
- Herald handles tens of notifications/sec — I/O-bound, scales by adding delivery channels

Both share infrastructure libraries (`pkg/`) but have independent lifecycles.

## Key Design Decisions

### 1. Kafka for Events, NATS for Alerts

**Problem**: Need a message broker between services. One size doesn't fit all.

**Decision**: Kafka for the event pipeline (collector → processor → alert-manager), NATS JetStream for alert delivery (alert-manager → dispatcher, gateway → delivery-worker).

**Rationale**:
- Kafka excels at high-throughput ordered log with retention (needed for forensics replay)
- NATS excels at low-latency fan-out with simpler ops (sufficient for alert volume)
- Using both demonstrates the ability to choose tools by requirements, not by familiarity

### 2. Custom Circuit Breaker & Rate Limiter (No Libraries)

**Problem**: Need fault tolerance patterns for downstream service protection.

**Decision**: Implement from scratch instead of using `sony/gobreaker` or `golang.org/x/time/rate`.

**Rationale**:
- Demonstrates deep understanding of the patterns (interview talking point)
- Zero-allocation implementations: CB 7.7M ops/sec, RL 4M ops/sec
- Tailored to our exact needs (Prometheus metrics integration, specific state callbacks)

### 3. Worker Pool, Not Goroutine-Per-Event

**Problem**: Processing 10k events/sec requires parallelism.

**Decision**: Fixed-size worker pool with bounded channel buffer.

**Rationale**:
- Goroutine-per-event at 10k/sec = 10k goroutines, unpredictable memory, DB connection exhaustion
- Worker pool = bounded N workers, bounded N DB connections, natural backpressure
- When the channel buffer fills, Kafka consumer pauses automatically (built-in backpressure)

### 4. Protobuf + gRPC for Agent-to-Collector

**Problem**: Agent sends 10k events/sec to collector. Need efficient serialization.

**Decision**: Protocol Buffers over gRPC instead of JSON over REST.

**Rationale**:
- Protobuf is 3-10x smaller than JSON (no field names repeated)
- Strict schema catches incompatible changes at compile time
- gRPC over HTTP/2: multiplexing, header compression, bidirectional streaming

### 5. Heap-Based Priority Queue (Not `container/heap`)

**Problem**: Herald needs priority ordering with SLA enforcement.

**Decision**: Custom heap implementation in `herald/internal/gateway/priority/`.

**Rationale**:
- Go's `container/heap` requires implementing a 5-method interface on a slice type — verbose
- Our implementation is cleaner: `Push(Item)`, `Pop()`, `TryPop()`, `Len()`
- Demonstrates algorithm knowledge: 17ns/op for push+pop
- FIFO ordering within same priority (stable sort property)

### 6. YAML Rule DSL with Hot Reload

**Problem**: Security teams need to add/modify detection rules without deploying code.

**Decision**: YAML-based rule language loaded from files, with fsnotify-based hot reload.

**Rationale**:
- YAML is human-readable and version-controllable (Git)
- Three rule types cover most SIEM use cases: single, threshold, sequence
- Hot reload via `sync.RWMutex` swap: zero-downtime rule updates
- Rule validation at load time prevents silent misconfigurations

### 7. Forensics Replay via Kafka Offset Seeking

**Problem**: After an incident, analysts need to check "was this happening 3 days ago?"

**Decision**: API that creates a temporary Kafka consumer, seeks to a timestamp, and replays through the rule engine.

**Rationale**:
- Kafka retains events for 7 days — replay is "free" (no additional storage)
- Replay uses an isolated rule engine instance (doesn't affect production state)
- SSE streaming provides real-time results to the analyst
- This is a key differentiator vs ELK (which requires re-indexing for rule changes)

## Concurrency Model

Each service follows the same pattern:
1. `main()` calls `run()` which returns an error
2. `signal.NotifyContext` creates a context cancelled on SIGINT/SIGTERM
3. Dependencies are initialized in order (config → logger → connections → business logic)
4. Background goroutines are managed via `errgroup`
5. Graceful shutdown: mark not-ready → drain in-flight → close connections

## Error Handling

- All errors are wrapped with context: `fmt.Errorf("operation: %w", err)`
- Infrastructure errors (DB down) → log + continue (don't crash the whole pipeline)
- Business logic errors (bad event) → log + skip event + count in metrics
- No panics in production code — panics only in `Must*` functions called from `main()`

## Testing Strategy

- **Unit tests**: table-driven, in same package (white-box)
- **Benchmarks**: for algorithmic code (rule engine, circuit breaker, rate limiter, priority queue)
- **Integration tests**: testcontainers (behind `//go:build integration` tag)
- **Load tests**: k6 scripts in `loadtest/`
