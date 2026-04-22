package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

type BuildInfo interface {
	subsystem

	Set(version, date string)
}

type prometheusBuildInfo struct {
	info *prometheus.GaugeVec
}

func newPrometheusBuildInfo(namespace string) *prometheusBuildInfo {
	return &prometheusBuildInfo{
		info: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "build_info",
			Help:      "Build information of cloud-secrets",
		}, []string{"version", "date"}),
	}
}

func (i *prometheusBuildInfo) collectors() []prometheus.Collector {
	return []prometheus.Collector{i.info}
}

func (i *prometheusBuildInfo) Set(version, date string) {
	i.info.WithLabelValues(version, date).Set(1)
}
