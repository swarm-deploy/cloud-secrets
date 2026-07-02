package metrics

import "github.com/prometheus/client_golang/prometheus"

// Secrets contains metrics for synchronized secrets lifecycle.
type Secrets interface {
	subsystem

	// IncCreated increments the number of created secrets.
	IncCreated()
	// IncRemovedSecrets increments the number of removed secrets.
	IncRemovedSecrets()
	// IncRemovedVersions increments the number of removed secret versions.
	IncRemovedVersions()
	// IncUpdated increments the number of updated secrets.
	IncUpdated()
}

type prometheusSecrets struct {
	created         prometheus.Counter
	removedSecrets  prometheus.Counter
	removedVersions prometheus.Counter
	updated         prometheus.Counter
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
		removedSecrets: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "secrets",
				Name:      "removed_total",
				Help:      "Number of removed secrets.",
			},
		),
		removedVersions: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "secrets",
				Name:      "removed_versions_total",
				Help:      "Number of removed secret versions.",
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

func (m *prometheusSecrets) IncRemovedSecrets() {
	m.removedSecrets.Inc()
}

func (m *prometheusSecrets) IncRemovedVersions() {
	m.removedVersions.Inc()
}

func (m *prometheusSecrets) IncUpdated() {
	m.updated.Inc()
}

func (m *prometheusSecrets) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.created, m.removedSecrets, m.removedVersions, m.updated}
}
