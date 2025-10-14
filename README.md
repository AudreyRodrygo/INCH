# Sentinel + Herald

> Real-time security event processing platform with intelligent notification delivery.

## Overview

**Sentinel** is a cloud-native SIEM platform that processes security events in real time. It collects events from lightweight agents, correlates them using a custom rule engine with temporal pattern detection, and delivers alerts in under 100ms.

**Herald** is a smart notification gateway with priority-based delivery, SLA enforcement, and alert fatigue prevention through intelligent digest aggregation.

## Architecture

```
                    ┌─────────────┐
                    │   Agents    │  Lightweight Go binaries (<15MB)
                    └──────┬──────┘
                           │ gRPC + Protobuf
                           ▼
┌──────────────────────────────────────────────────────┐
│                     SENTINEL                          │
│                                                       │
│  ┌─────────────┐    Kafka    ┌─────────────────┐     │
│  │  Collector   │───────────▶│    Processor     │     │
│  │  (gRPC srv)  │ raw-events │  (Rule Engine)   │     │
│  └─────────────┘             └────────┬────────┘     │
│                                       │ Kafka         │
│                                       ▼ alerts        │
│                              ┌─────────────────┐     │
│                              │  Alert Manager   │     │
│                              │ (Circuit Breaker)│     │
│                              └────────┬────────┘     │
└───────────────────────────────────────┼──────────────┘
                                        │ NATS JetStream
                                        ▼
┌──────────────────────────────────────────────────────┐
│                      HERALD                           │
│                                                       │
│  ┌─────────────┐    NATS    ┌──────────────────┐     │
│  │ Gateway API  │──────────▶│ Delivery Worker   │     │
│  │ (REST+gRPC)  │           │ Email/Slack/TG/WH │     │
│  └─────────────┘            └──────────────────┘     │
└──────────────────────────────────────────────────────┘
```

## Key Features

- **Sub-100ms latency** from event to alert
- **Temporal correlation** — sliding windows, sequence detection (kill chain)
- **Behavioral baseline** — statistical anomaly detection without ML frameworks
- **Hot rule reload** — update correlation rules without restart
- **Forensics replay** — re-run historical events with new rules
- **Priority Queue with SLA** — CRITICAL <1s, HIGH <10s, NORMAL <60s
- **Digest mode** — batch low-priority alerts to prevent alert fatigue

## Tech Stack

| Component | Technology | Why |
|-----------|-----------|-----|
| Language | Go | Low memory, fast binaries, native concurrency |
| Transport | gRPC + Protobuf | Binary serialization, strict schema, streaming |
| Event Bus | Kafka (franz-go) | High-throughput log with retention for replay |
| Alert Bus | NATS JetStream | Low-latency fan-out, simpler ops than Kafka |
| Database | PostgreSQL (pgx) | JSONB for flexible enrichment data |
| Cache | Redis | Deduplication, rate limiter state, baseline data |
| Tracing | OpenTelemetry + Jaeger | End-to-end distributed tracing |
| Metrics | Prometheus + Grafana | Custom dashboards with SLA tracking |

## Quick Start

```bash
# Start infrastructure
docker-compose -f docker-compose.infra.yml up -d

# Build all services
make build

# Run tests
make test
```

## Project Structure

```
├── pkg/          Shared libraries (circuit breaker, rate limiter, retry)
├── proto/        Protobuf definitions
├── sentinel/     Security event processing platform
├── herald/       Notification gateway
├── loadtest/     k6 performance tests
└── grafana/      Dashboard exports
```

## License

MIT
