package metrics

import "github.com/prometheus/client_golang/prometheus"

// Group contains all metrics subsystems and exposes a single collector.
type Group struct {
	BuildInfo BuildInfo
	// Docker contains Docker API metrics.
	Docker Docker
	// Provider contains cloud provider API metrics.
	Provider Provider
	// Secrets contains secret synchronization metrics.
	Secrets Secrets
	// Syncs contains sync run metrics.
	Syncs Syncs

	collectors []prometheus.Collector
}

// CreateGroupParams contains metric group initialization settings.
type CreateGroupParams struct {
	// Namespace is a common prefix for all metric names.
	Namespace string
}

type subsystem interface {
	collectors() []prometheus.Collector
}

// NewGroup creates a metrics group with all enabled subsystems.
func NewGroup(params CreateGroupParams) *Group {
	group := &Group{
		collectors: make([]prometheus.Collector, 0),
	}

	group.BuildInfo = newPrometheusBuildInfo(params.Namespace)
	group.register(group.BuildInfo)

	group.Docker = newPrometheusDocker(params.Namespace)
	group.register(group.Docker)

	group.Provider = newPrometheusProvider(params.Namespace)
	group.register(group.Provider)

	group.Secrets = newPrometheusSecrets(params.Namespace)
	group.register(group.Secrets)

	group.Syncs = newPrometheusSyncs(params.Namespace)
	group.register(group.Syncs)

	return group
}

func (g *Group) register(ss subsystem) {
	g.collectors = append(g.collectors, ss.collectors()...)
}

// Describe sends metric descriptors to Prometheus.
func (g *Group) Describe(ch chan<- *prometheus.Desc) {
	for _, collector := range g.collectors {
		collector.Describe(ch)
	}
}

// Collect sends collected metric values to Prometheus.
func (g *Group) Collect(ch chan<- prometheus.Metric) {
	for _, collector := range g.collectors {
		collector.Collect(ch)
	}
}
