package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Docker contains metrics for Docker API operations.
type Docker interface {
	subsystem

	// RecordRequest records one Docker API request by operation.
	RecordRequest(operation string, duration time.Duration)
}

type prometheusDocker struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newPrometheusDocker(namespace string) *prometheusDocker {
	return &prometheusDocker{
		requests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "docker",
				Name:      "requests_total",
				Help:      "Number of Docker requests grouped by operation.",
			},
			[]string{"operation"},
		),
		duration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: namespace,
				Subsystem: "docker",
				Name:      "request_duration_seconds",
				Help:      "Duration of Docker requests in seconds grouped by operation.",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
			},
			[]string{"operation"},
		),
	}
}

func (m *prometheusDocker) RecordRequest(operation string, duration time.Duration) {
	m.requests.WithLabelValues(operation).Inc()
	m.duration.WithLabelValues(operation).Observe(duration.Seconds())
}

func (m *prometheusDocker) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.requests, m.duration}
}
