package metrics

import "github.com/prometheus/client_golang/prometheus"

// Secrets contains metrics for synchronized secrets lifecycle.
type Secrets interface {
	subsystem

	// IncCreated increments the number of created secrets.
	IncCreated()
	// IncRemoved increments the number of removed secrets.
	IncRemoved()
	// IncUpdated increments the number of updated secrets.
	IncUpdated()
}

type prometheusSecrets struct {
	created prometheus.Counter
	removed prometheus.Counter
	updated prometheus.Counter
}

func newPrometheusSecrets(namespace string) *prometheusSecrets {
	return &prometheusSecrets{
		created: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "secrets",
				Name:      "created_total",
				Help:      "Number of created secrets.",
			},
		),
		removed: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "secrets",
				Name:      "removed_total",
				Help:      "Number of removed secrets.",
			},
		),
		updated: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "secrets",
				Name:      "updated_total",
				Help:      "Number of updated secrets.",
			},
		),
	}
}

func (m *prometheusSecrets) IncCreated() {
	m.created.Inc()
}

func (m *prometheusSecrets) IncRemoved() {
	m.removed.Inc()
}

func (m *prometheusSecrets) IncUpdated() {
	m.updated.Inc()
}

func (m *prometheusSecrets) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.created, m.removed, m.updated}
}
