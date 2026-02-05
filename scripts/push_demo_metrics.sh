#!/usr/bin/env bash
# Push demo metrics to Pushgateway.
# Usage: ./scripts/push_demo_metrics.sh [iterations] [interval_seconds]

set -euo pipefail

GW="${PUSHGATEWAY_URL:-http://localhost:9091}"
ITERATIONS="${1:-80}"
INTERVAL="${2:-2}"

echo "Pushing demo metrics to $GW ($ITERATIONS iterations, every ${INTERVAL}s)"

for i in $(seq 1 "$ITERATIONS"); do
  # --- counters grow over time ---
  ev_total=$((i * 150 + RANDOM % 50))
  notif_total=$((i * 45 + RANDOM % 20))
  notif_fail=$((i / 10 + 1))

  # --- gauges vary each iteration ---
  queue=$((10 + RANDOM % 40))
  kafka_lag=$((RANDOM % 200))
  dlq=$((RANDOM % 4))
  queue_pri=$((5 + RANDOM % 20))

  # ── Sentinel metrics ────────────────────────────────────────────────────────
  curl -s --data-binary @- "${GW}/metrics/job/event-processor" <<EOF
# TYPE events_processed_total counter
events_processed_total{service="event-processor"} $ev_total
# TYPE alerts_fired_total counter
alerts_fired_total{severity="critical",service="event-processor"} $((i / 5 + 1))
alerts_fired_total{severity="warning",service="event-processor"} $((i / 2 + 1))
# TYPE circuit_breaker_state gauge
circuit_breaker_state{service="event-processor"} 0
# TYPE worker_pool_queue_depth gauge
worker_pool_queue_depth{service="event-processor"} $queue
# TYPE kafka_consumer_lag gauge
kafka_consumer_lag{topic="security-events",group="event-processor"} $kafka_lag
# TYPE processing_latency_seconds_bucket gauge
processing_latency_seconds_bucket{le="0.005",service="event-processor"} $((ev_total * 6 / 10))
processing_latency_seconds_bucket{le="0.025",service="event-processor"} $((ev_total * 92 / 100))
processing_latency_seconds_bucket{le="+Inf",service="event-processor"} $ev_total
EOF

  # ── Herald metrics ───────────────────────────────────────────────────────────
  curl -s --data-binary @- "${GW}/metrics/job/notification-dispatcher" <<EOF
# TYPE notifications_sent_total counter
notifications_sent_total{channel="email"} $((notif_total * 50 / 100))
notifications_sent_total{channel="slack"} $((notif_total * 35 / 100))
notifications_sent_total{channel="webhook"} $((notif_total * 15 / 100))
# TYPE notifications_failed_total counter
notifications_failed_total{channel="email"} $((notif_fail * 6 / 10))
notifications_failed_total{channel="slack"} $((notif_fail * 3 / 10))
notifications_failed_total{channel="webhook"} $((notif_fail / 10 + 1))
# TYPE delivery_latency_seconds_bucket gauge
delivery_latency_seconds_bucket{le="0.05",channel="email"} $((notif_total * 30 / 100))
delivery_latency_seconds_bucket{le="0.1",channel="email"} $((notif_total * 70 / 100))
delivery_latency_seconds_bucket{le="+Inf",channel="email"} $notif_total
delivery_latency_seconds_bucket{le="0.02",channel="slack"} $((notif_total * 60 / 100))
delivery_latency_seconds_bucket{le="+Inf",channel="slack"} $notif_total
# TYPE priority_queue_depth gauge
priority_queue_depth $queue_pri
# TYPE sla_compliance_rate gauge
sla_compliance_rate{priority="critical"} 0.97
sla_compliance_rate{priority="high"} 0.95
# TYPE dlq_depth gauge
dlq_depth $dlq
EOF

  echo -ne "\r[$i/$ITERATIONS] events=$ev_total  notifications=$notif_total  "
  sleep "$INTERVAL"
done

echo ""
echo "Done. Open Grafana at http://localhost:3001"
