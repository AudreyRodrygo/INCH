package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsHandler returns an HTTP handler that serves Prometheus metrics
// on the /metrics endpoint.
//
// Prometheus scrapes this endpoint periodically (every 15s by default)
// to collect metrics. Each service exposes its own metrics handler,
// typically on a separate port from the main API.
//
// Usage:
//
//	mux.Handle("/metrics", observability.MetricsHandler())
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

// NewCounter creates a Prometheus counter metric and registers it.
//
// Counters only go up — they track cumulative totals like:
//   - events_processed_total
//   - alerts_fired_total
//   - requests_total
//
// Labels add dimensions: you can count events per severity, per source, etc.
// Example: events_processed_total{severity="high"} = 42
func NewCounter(name, help string, labels []string) *prometheus.CounterVec {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: name,
		Help: help,
	}, labels)

	prometheus.MustRegister(counter)
	return counter
}

// NewHistogram creates a Prometheus histogram for measuring distributions.
//
// Histograms are used for latency measurements. They automatically calculate
// percentiles (P50, P95, P99) from observed values.
//
// The buckets parameter defines the histogram boundaries. For latency:
//   - DefaultLatencyBuckets: 1ms, 5ms, 10ms, 25ms, 50ms, 100ms, 250ms, 500ms, 1s, 5s
//
// Each observed value falls into a bucket, and Prometheus calculates
// approximate percentiles from the bucket counts.
func NewHistogram(name, help string, labels []string, buckets []float64) *prometheus.HistogramVec {
	if buckets == nil {
		buckets = DefaultLatencyBuckets
	}

	histogram := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: buckets,
	}, labels)

	prometheus.MustRegister(histogram)
	return histogram
}

// NewGauge creates a Prometheus gauge metric and registers it.
//
// Gauges can go up and down — they track current values like:
//   - worker_pool_queue_depth
//   - active_connections
//   - circuit_breaker_state (0=closed, 1=open, 2=half-open)
func NewGauge(name, help string, labels []string) *prometheus.GaugeVec {
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, labels)

	prometheus.MustRegister(gauge)
	return gauge
}

// DefaultLatencyBuckets provides histogram buckets tuned for network service latency.
// Values are in seconds: 1ms to 5s.
var DefaultLatencyBuckets = []float64{
	0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 5.0,
}
