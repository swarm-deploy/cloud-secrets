package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Syncs contains metrics for synchronization runs.
type Syncs interface {
	subsystem

	// RecordRun increments sync run counter by trigger.
	RecordRun(trigger string)
	// SetLastSyncAt records unix timestamp of the latest completed sync.
	SetLastSyncAt(t time.Time)
}

type prometheusSyncs struct {
	runs       *prometheus.CounterVec
	lastSyncAt prometheus.Gauge
}

func newPrometheusSyncs(namespace string) *prometheusSyncs {
	return &prometheusSyncs{
		runs: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: "syncs",
				Name:      "runs_total",
				Help:      "Number of sync runs grouped by trigger.",
			},
			[]string{"trigger"},
		),
		lastSyncAt: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Subsystem: "syncs",
				Name:      "last_sync_at_unix",
				Help:      "Unix timestamp of the latest completed sync run.",
			},
		),
	}
}

func (m *prometheusSyncs) RecordRun(trigger string) {
	m.runs.WithLabelValues(trigger).Inc()
}

func (m *prometheusSyncs) SetLastSyncAt(t time.Time) {
	m.lastSyncAt.Set(float64(t.Unix()))
}

func (m *prometheusSyncs) collectors() []prometheus.Collector {
	return []prometheus.Collector{m.runs, m.lastSyncAt}
}
