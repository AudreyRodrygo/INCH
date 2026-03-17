# INCH — Architecture & Design Decisions

## System Flow

```
  ┌─────────────────────────────────────────────────────────────────────────────┐
  │  HOST LAYER                                                                  │
  │  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐                    │
  │  │  inch-agent  │   │  inch-agent  │   │  inch-agent  │   (DaemonSet)       │
  │  │  tail logs   │   │  tail logs   │   │  tail logs   │                    │
  │  │  watch procs │   │  watch procs │   │  watch procs │                    │
  └──┴──────┬───────┴───┴──────┬───────┴───┴──────┬───────┴────────────────────┘
            │  gRPC / proto3   │                  │
            └──────────────────┼──────────────────┘
                               │  StreamEvents RPC
                               ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │  INGESTION                                                                  │
  │  ┌──────────────────────────────┐                                           │
  │  │       event-collector        │  normalize → validate → enrich (basic)   │
  │  └──────────────┬───────────────┘                                           │
  └─────────────────┼──────────────────────────────────────────────────────────┘
                    │  Kafka  →  topic: raw-events
                    ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │  PROCESSING                                                                 │
  │  ┌──────────────────────────────┐                                           │
  │  │       event-processor        │  GeoIP enrichment → severity scoring     │
  │  │                              │  rule engine (single/threshold/sequence)  │
  │  │   worker pool ──▶ rule DSL   │  hot reload via fsnotify                 │
  │  └──────────────┬───────────────┘                                           │
  └─────────────────┼──────────────────────────────────────────────────────────┘
                    │  Kafka  →  topic: alerts
                    ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │  ALERT MANAGEMENT                                                           │
  │  ┌──────────────────────────────┐                                           │
  │  │        alert-manager         │  deduplication (Redis TTL windows)       │
  │  │                              │  route by severity → NATS subjects       │
  │  └──────────────┬───────────────┘                                           │
  └─────────────────┼──────────────────────────────────────────────────────────┘
                    │  NATS JetStream  →  subjects: alerts.critical / alerts.high / …
                    ▼
  ┌────────────────────────────────────────────────────────────────────────────┐
  │  DELIVERY                                                                   │
  │  ┌──────────────────────────────┐    ┌─────────────────────────────────┐   │
  │  │  notification-dispatcher     │    │         gateway-api              │   │
  │  │                              │    │  REST forensics replay           │   │
  │  │  Email · Slack · Webhook     │    │  Kafka offset seek → SSE stream  │   │
  │  │  Telegram                    │    │  delivery analytics              │   │
  │  └──────────────────────────────┘    └─────────────────────────────────┘   │
  └────────────────────────────────────────────────────────────────────────────┘
```

```
  INFRASTRUCTURE
  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐
  │  PostgreSQL  │  │    Redis     │  │  Prometheus  │  │  Jaeger (OTLP)       │
  │  alerts DB   │  │  dedup TTL   │  │  + Grafana   │  │  distributed traces  │
  └──────────────┘  └──────────────┘  └──────────────┘  └──────────────────────┘
```

---

## pkg/ — Shared Libraries

```
  pkg/
  ├── config          ─  Viper wrapper, env override, structured config structs
  ├── observability   ─  zap logger + Prometheus registerer + OpenTelemetry tracer
  ├── postgres        ─  pgx/v5 pool factory, migration runner (goose)
  ├── redisutil       ─  go-redis/v9 client factory, health ping
  ├── kafkautil       ─  franz-go producer/consumer builders, offset helpers
  ├── natsutil        ─  NATS JetStream connect, stream/consumer provisioning
  ├── dlq             ─  dead-letter queue: failed events → Kafka DLQ topic
  ├── retry           ─  exponential backoff with jitter, context-aware
  ├── ratelimiter     ─  token-bucket, zero-alloc, 4 M ops/sec
  ├── circuitbreaker  ─  three-state CB, zero-alloc, 7.2 M ops/sec
  └── health          ─  HTTP /healthz + /readyz handlers, checker registry
```

Every service depends only on the `pkg/` packages it needs. No service imports another service.

---

## Key Design Decisions

### 1. Why Two Brokers? Kafka for Events, NATS for Alerts

```
  ┌─────────────────────────────────┬──────────────────────────────────────┐
  │           KAFKA                 │           NATS JetStream             │
  ├─────────────────────────────────┼──────────────────────────────────────┤
  │  10 k+ events/sec ingestion     │  tens of alerts/sec                  │
  │  7-day retention for replay     │  ephemeral — ack and done            │
  │  ordered, partitioned log       │  low-latency fan-out                 │
  │  horizontal scale via partitions│  simpler ops (no ZooKeeper/KRaft)    │
  │  consumer groups for processing │  per-consumer subject filtering      │
  └─────────────────────────────────┴──────────────────────────────────────┘

  Rule: use the right tool for the requirement, not the familiar one.
  Kafka's retention makes forensics replay "free" — no extra storage.
  NATS's simplicity reduces operational toil for the low-volume delivery layer.
```

### 2. Custom Circuit Breaker & Rate Limiter

```
  Closed ──── failures < threshold ───▶ Closed
    │                                      ▲
    │ failures ≥ threshold                 │ probe success
    ▼                                      │
  Open ──────── timeout expires ─────▶ Half-Open
    │                                      │
    └──── probe fails ◀────────────────────┘

  ┌─────────────────────────────────────────────────────┐
  │  circuit breaker   7.2 M ops/sec   zero allocation  │
  │  rate limiter      4.0 M ops/sec   zero allocation  │
  │  both integrate with Prometheus state/count metrics │
  └─────────────────────────────────────────────────────┘

  Rationale: sony/gobreaker allocates on every call; golang.org/x/time/rate
  lacks Prometheus hooks. Implementing from scratch demonstrates the pattern
  deeply and removes a dependency with no added value.
```

### 3. Worker Pool, Not Goroutine-Per-Event

```
  Kafka partition
       │
       ▼
  ┌─────────────────────────────────────────┐
  │  consumer loop                          │
  │       │                                 │
  │       ▼                                 │
  │  ┌─────────┐  bounded channel (N=256)   │
  │  │ channel │◀── backpressure point      │
  │  └────┬────┘                            │
  │       │ dispatch                        │
  │  ┌────▼──────────────────────────┐      │
  │  │  worker  worker  worker  ...  │  ×W  │
  │  └───────────────────────────────┘      │
  └─────────────────────────────────────────┘
       │ channel full → Kafka consumer.Pause()
       ▼ channel drains → Kafka consumer.Resume()

  Goroutine-per-event at 10 k/sec → 10 k goroutines → DB pool exhaustion.
  Fixed pool → bounded memory, bounded DB connections, natural backpressure.
```

### 4. Protobuf + gRPC for Agent → Collector

```
  inch-agent                         event-collector
  ┌──────────────────────┐           ┌─────────────────────────┐
  │  proto3 SecurityEvent│──────────▶│  StreamEvents(stream)   │
  │  binary wire format  │  HTTP/2   │  server-side streaming  │
  └──────────────────────┘           └─────────────────────────┘

  JSON 1 event ≈ 850 B   →   Protobuf ≈ 120 B   (≈ 7× smaller)
  Strict schema: breaking changes caught at proto compile time, not runtime.
  HTTP/2 multiplexing: one TCP connection per agent, no head-of-line blocking.
```

### 5. Custom Heap Priority Queue

```
  inch/internal/gateway/priority/

  Our API          vs.    container/heap
  ────────────────────────────────────────
  Push(item)              heap.Push(&h, item)
  Pop() Item              heap.Pop(&h).(Item)
  TryPop() (Item, bool)   — not available —
  Len() int               len(h)

  121 ns/op push+pop   ·   FIFO ordering within equal priority (stable)
  Used by gateway-api to prioritize forensics replay requests by severity.
```

### 6. YAML Rule DSL with fsnotify Hot Reload

```yaml
  # rules/brute_force.yaml
  - id: brute-force-ssh
    type: threshold        # single | threshold | sequence
    condition: event.type == "auth_failure" && event.dst_port == 22
    threshold: 5
    window: 60s
    severity: high
```

```
  fsnotify WRITE event
        │
        ▼
  ┌─────────────────────────────┐
  │  parse + validate rules     │
  │  compile condition AST      │
  │         │ success           │
  │  sync.RWMutex.Lock()        │
  │  swap active rule set       │
  │  sync.RWMutex.Unlock()      │
  └─────────────────────────────┘
        │ zero-downtime, in-flight events use old snapshot
```

### 7. Forensics Replay via Kafka Offset Seeking

```
  analyst                gateway-api              Kafka cluster
     │                       │                         │
     │  GET /replay?from=…    │                         │
     │ ──────────────────────▶│                         │
     │                        │  seek to timestamp      │
     │                        │ ───────────────────────▶│
     │                        │  replay consumer        │
     │                        │◀─── raw-events ─────────│
     │                        │  isolated rule engine   │
     │  SSE stream            │  (no production state)  │
     │◀──────────────────────-│                         │

  7-day Kafka retention = free historical window.
  Isolated consumer group = replay never affects live processing lag.
  SSE = results stream to the browser as they are produced.
```

---

## Concurrency Model

```
  main()
    └── run(ctx)
          ├── signal.NotifyContext(SIGINT, SIGTERM)  ← ctx cancelled on signal
          ├── init: config → logger → DB/Redis/Kafka/NATS connections
          ├── errgroup.WithContext(ctx)
          │     ├── goroutine: Kafka consumer loop
          │     ├── goroutine: worker pool supervisor
          │     ├── goroutine: HTTP health server
          │     └── goroutine: metrics server
          └── g.Wait() → first non-nil error propagates, ctx cancelled for peers

  Graceful shutdown order:
    1. mark /readyz → 503  (stop receiving new traffic)
    2. Kafka consumer stops polling
    3. drain in-flight channel (workers finish current items)
    4. flush Kafka producer (collector)
    5. close DB/Redis/NATS connections
    6. logger.Sync()
```

---

## Error Handling

```
  ┌───────────────────────────────────────────────────────────────────────┐
  │  Layer              Strategy                                           │
  ├───────────────────────────────────────────────────────────────────────┤
  │  wrapping           fmt.Errorf("operation: %w", err)  — always        │
  │  infrastructure     DB/broker down → log + metric + continue          │
  │  business logic     bad event → log + skip + DLQ publish              │
  │  fatal startup      Must*() helpers in main() — panic is acceptable   │
  │  production code    never panic — return errors up the stack          │
  └───────────────────────────────────────────────────────────────────────┘

  Dead-letter queue (pkg/dlq): events that fail after retry exhaustion are
  published to a Kafka DLQ topic for offline inspection, never silently dropped.
```

---

## Testing Strategy

```
  ┌─────────────────┬────────────────────────────────────────────────────┐
  │  Layer          │  Approach                                           │
  ├─────────────────┼────────────────────────────────────────────────────┤
  │  Unit           │  table-driven, same package (white-box)             │
  │                 │  testify/assert + require                           │
  ├─────────────────┼────────────────────────────────────────────────────┤
  │  Benchmarks     │  rule engine, CB, RL, priority queue                │
  │                 │  track ns/op and allocs/op in CI                    │
  ├─────────────────┼────────────────────────────────────────────────────┤
  │  Integration    │  testcontainers (Postgres, Redis, Kafka, NATS)      │
  │                 │  //go:build integration  — skipped in normal CI     │
  ├─────────────────┼────────────────────────────────────────────────────┤
  │  Load           │  k6 scripts in loadtest/                            │
  │                 │  target: 10 k events/sec sustained, p99 < 50 ms     │
  └─────────────────┴────────────────────────────────────────────────────┘
```
