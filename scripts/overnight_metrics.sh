#!/usr/bin/env bash
# Overnight demo metrics — runs for ~7 hours pushing realistic traffic patterns.
# Simulates: traffic waves, latency spikes, occasional errors, circuit breaker events.
#
# Usage: ./scripts/overnight_metrics.sh
# Stop:  Ctrl+C

set -euo pipefail

GW="${PUSHGATEWAY_URL:-http://localhost:9091}"
INTERVAL=15          # seconds between pushes (matches Prometheus scrape interval)
DURATION=25200       # 7 hours in seconds
ITERATIONS=$((DURATION / INTERVAL))

echo "========================================"
echo "  INCH overnight metrics generator"
echo "  Duration : 7 hours (~$ITERATIONS pushes)"
echo "  Interval : ${INTERVAL}s"
echo "  Target   : $GW"
echo "========================================"
echo "Started at: $(date)"
echo "Press Ctrl+C to stop early."
echo ""

# Counters (monotonically increasing)
ev_total=0
alerts_crit=0
alerts_warn=0
notif_email=0
notif_slack=0
notif_webhook=0
notif_fail_email=0
notif_fail_slack=0
notif_fail_webhook=0

START_TIME=$SECONDS

for i in $(seq 1 "$ITERATIONS"); do
  ELAPSED=$(( SECONDS - START_TIME ))
  HOUR_FRAC=$(echo "$ELAPSED 3600" | awk '{printf "%.4f", $1/$2}')

  # ── Traffic wave: slow sine over 7 hours, peaks at hours 1, 4, 7 ────────────
  # Base load oscillates between 60 and 200 events/interval
  WAVE=$(echo "$i $ITERATIONS" | awk '{
    pi = 3.14159265
    # two full sine cycles over the run
    x = 2 * pi * 2 * ($1 / $2)
    wave = sin(x)
    base = 130 + wave * 70
    printf "%d", base
  }')

  # Add random jitter ±20%
  JITTER=$(( RANDOM % 40 - 20 ))
  LOAD=$(( WAVE + JITTER ))
  if [ "$LOAD" -lt 10 ]; then LOAD=10; fi

  # ── Occasional traffic spike (roughly once every 30 minutes) ─────────────────
  SPIKE_CHANCE=$(( RANDOM % 120 ))
  if [ "$SPIKE_CHANCE" -eq 0 ]; then
    LOAD=$(( LOAD * 3 ))
    echo "  [SPIKE] iteration $i — load x3"
  fi

  # ── Counters grow by LOAD each iteration ─────────────────────────────────────
  ev_total=$(( ev_total + LOAD ))
  notif_email=$(( notif_email + LOAD * 50 / 100 ))
  notif_slack=$(( notif_slack + LOAD * 35 / 100 ))
  notif_webhook=$(( notif_webhook + LOAD * 15 / 100 ))

  # Failures: ~1.5% of notifications
  FAIL=$(( LOAD * 15 / 1000 + 1 ))
  notif_fail_email=$(( notif_fail_email + FAIL * 6 / 10 ))
  notif_fail_slack=$(( notif_fail_slack + FAIL * 3 / 10 ))
  notif_fail_webhook=$(( notif_fail_webhook + 1 ))

  # Alerts: roughly 1 critical per 50 events, 1 warning per 20
  alerts_crit=$(( alerts_crit + LOAD / 50 + (RANDOM % 2) ))
  alerts_warn=$(( alerts_warn + LOAD / 20 + (RANDOM % 3) ))

  # ── Gauges (vary per iteration) ───────────────────────────────────────────────
  QUEUE=$(( LOAD / 5 + RANDOM % 15 ))
  KAFKA_LAG=$(( LOAD * 2 + RANDOM % 100 ))
  QUEUE_PRI=$(( LOAD / 10 + RANDOM % 10 ))
  DLQ=$(( RANDOM % 5 ))

  # Latency scales with load: higher load → higher latency
  LAT_BASE=$(echo "$LOAD" | awk '{printf "%.4f", 0.002 + $1 * 0.00008}')
  LAT_P95=$(echo "$LOAD"  | awk '{printf "%.4f", 0.010 + $1 * 0.00025}')
  LAT_P99=$(echo "$LOAD"  | awk '{printf "%.4f", 0.035 + $1 * 0.00060}')

  # Circuit breaker: opens briefly during spikes (LOAD > 350)
  CB_STATE=0
  if [ "$LOAD" -gt 350 ]; then CB_STATE=1; fi

  # SLA compliance: degrades slightly under high load
  SLA_CRIT=$(echo "$LOAD" | awk '{v = 0.99 - $1*0.00015; if(v<0.90) v=0.90; printf "%.4f", v}')
  SLA_HIGH=$(echo "$LOAD" | awk '{v = 0.97 - $1*0.00010; if(v<0.88) v=0.88; printf "%.4f", v}')

  # Histogram bucket approximations (cumulative counts)
  B_005=$(( ev_total * 55 / 100 ))
  B_025=$(( ev_total * 89 / 100 ))
  B_100=$(( ev_total * 98 / 100 ))

  DL_05_EMAIL=$(( notif_email * 28 / 100 ))
  DL_10_EMAIL=$(( notif_email * 68 / 100 ))
  DL_02_SLACK=$(( notif_slack * 55 / 100 ))

  # ── Push Sentinel metrics ─────────────────────────────────────────────────────
  curl -s --data-binary @- "${GW}/metrics/job/event-processor" <<EOF
# TYPE events_processed_total counter
events_processed_total{service="event-processor"} $ev_total
# TYPE alerts_fired_total counter
alerts_fired_total{severity="critical",service="event-processor"} $alerts_crit
alerts_fired_total{severity="warning",service="event-processor"} $alerts_warn
# TYPE circuit_breaker_state gauge
circuit_breaker_state{service="event-processor"} $CB_STATE
# TYPE worker_pool_queue_depth gauge
worker_pool_queue_depth{service="event-processor"} $QUEUE
# TYPE kafka_consumer_lag gauge
kafka_consumer_lag{topic="security-events",group="event-processor"} $KAFKA_LAG
# TYPE processing_latency_seconds_bucket gauge
processing_latency_seconds_bucket{le="0.005",service="event-processor"} $B_005
processing_latency_seconds_bucket{le="0.025",service="event-processor"} $B_025
processing_latency_seconds_bucket{le="0.1",service="event-processor"} $B_100
processing_latency_seconds_bucket{le="+Inf",service="event-processor"} $ev_total
# TYPE current_load gauge
current_load{service="event-processor"} $LOAD
EOF

  # ── Push Herald metrics ───────────────────────────────────────────────────────
  curl -s --data-binary @- "${GW}/metrics/job/notification-dispatcher" <<EOF
# TYPE notifications_sent_total counter
notifications_sent_total{channel="email"} $notif_email
notifications_sent_total{channel="slack"} $notif_slack
notifications_sent_total{channel="webhook"} $notif_webhook
# TYPE notifications_failed_total counter
notifications_failed_total{channel="email"} $notif_fail_email
notifications_failed_total{channel="slack"} $notif_fail_slack
notifications_failed_total{channel="webhook"} $notif_fail_webhook
# TYPE delivery_latency_seconds_bucket gauge
delivery_latency_seconds_bucket{le="0.05",channel="email"} $DL_05_EMAIL
delivery_latency_seconds_bucket{le="0.1",channel="email"} $DL_10_EMAIL
delivery_latency_seconds_bucket{le="+Inf",channel="email"} $notif_email
delivery_latency_seconds_bucket{le="0.02",channel="slack"} $DL_02_SLACK
delivery_latency_seconds_bucket{le="+Inf",channel="slack"} $notif_slack
# TYPE priority_queue_depth gauge
priority_queue_depth $QUEUE_PRI
# TYPE sla_compliance_rate gauge
sla_compliance_rate{priority="critical"} $SLA_CRIT
sla_compliance_rate{priority="high"} $SLA_HIGH
# TYPE dlq_depth gauge
dlq_depth $DLQ
EOF

  # ── Progress line ─────────────────────────────────────────────────────────────
  ELAPSED_MIN=$(( ELAPSED / 60 ))
  REMAINING_MIN=$(( (DURATION - ELAPSED) / 60 ))
  echo -ne "\r[$(date +%H:%M)] iter=$i/$ITERATIONS  load=$LOAD  events=$ev_total  notif=$((notif_email+notif_slack+notif_webhook))  remaining=${REMAINING_MIN}m  "

  sleep "$INTERVAL"
done

echo ""
echo "========================================"
echo "Finished at: $(date)"
echo "Open Grafana: http://localhost:3001"
echo "========================================"
