// k6 load test for Herald gateway-api.
//
// Tests the POST /api/v1/notifications endpoint under load.
// Measures latency percentiles and throughput.
//
// Run: k6 run --vus 50 --duration 30s loadtest/herald_send.js
//
// Expected results (single instance):
//   - P50: <5ms
//   - P95: <20ms
//   - P99: <50ms
//   - Throughput: 5k+ req/sec

import http from "k6/http";
import { check, sleep } from "k6";
import { Rate, Trend } from "k6/metrics";

const errorRate = new Rate("errors");
const latency = new Trend("notification_latency", true);

const BASE_URL = __ENV.HERALD_URL || "http://localhost:8090";

const PRIORITIES = ["CRITICAL", "HIGH", "NORMAL", "LOW"];

export const options = {
  stages: [
    { duration: "10s", target: 20 }, // Ramp up.
    { duration: "30s", target: 50 }, // Sustained load.
    { duration: "10s", target: 0 }, // Ramp down.
  ],
  thresholds: {
    http_req_duration: ["p(95)<100", "p(99)<200"],
    errors: ["rate<0.01"],
  },
};

export default function () {
  const priority = PRIORITIES[Math.floor(Math.random() * PRIORITIES.length)];

  const payload = JSON.stringify({
    priority: priority,
    recipient: `user-${__VU}@example.com`,
    subject: `[${priority}] Security Alert #${__ITER}`,
    body: `Automated alert from load test. VU=${__VU}, Iter=${__ITER}`,
    channel: "webhook",
    source: "k6-loadtest",
    metadata: {
      test_run: "true",
      vu: String(__VU),
    },
  });

  const params = {
    headers: { "Content-Type": "application/json" },
  };

  const res = http.post(`${BASE_URL}/api/v1/notifications`, payload, params);

  const success = check(res, {
    "status is 202": (r) => r.status === 202,
    "response has notification_id": (r) => {
      const body = JSON.parse(r.body);
      return body.notification_id !== undefined;
    },
  });

  errorRate.add(!success);
  latency.add(res.timings.duration);

  sleep(0.01); // Small pause between requests.
}
