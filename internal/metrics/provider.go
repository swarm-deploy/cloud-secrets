package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Provider contains metrics for provider API operations.
type Provider interface {
	subsystem

	// RecordRequest records one provider API request by operation.
	RecordRequest(operation string, duration time.Duration)
}

type prometheusProvider struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newPrometheusProvider(namespace string) *prometheusProvider {
	return &prometheusProvider{
		requests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "provider",
				Name:      "requests_total",
				Help:      "Number of provider requests grouped by operation.",
			},
			[]string{"operation"},
		),
		duration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "provider",
				Name:      "request_duration_seconds",
				Help:      "Duration of provider requests in seconds grouped by operation.",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
			},
			[]string{"operation"},
		),
	}
}

func (m *prometheusProvider) RecordRequest(operation string, duration time.Duration) {
	m.requests.WithLabelValues(operation).Inc()
	m.duration.WithLabelValues(operation).Observe(duration.Seconds())
}

func (m *prometheusProvider) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.requests, m.duration}
}
