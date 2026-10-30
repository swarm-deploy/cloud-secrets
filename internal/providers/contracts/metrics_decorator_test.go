package contracts

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"go.uber.org/mock/gomock"
)

func TestMetricsDecorator_CreateSecret(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := NewMockProvider(ctrl)
	request := CreateSecretRequest{Path: "prod/db", Key: "password", Value: []byte("sensitive")}
	wantSecret := Secret{Path: "prod/db"}
	provider.EXPECT().CreateSecret(gomock.Any(), request).Return(wantSecret, nil)

	metricsGroup := metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test"})
	registry := prometheus.NewRegistry()
	require.NoError(t, registry.Register(metricsGroup))
	decorated := WithMetrics(provider, metricsGroup.Provider)

	secret, err := decorated.CreateSecret(context.Background(), request)

	require.NoError(t, err)
	assert.Equal(t, wantSecret, secret)
	metricFamilies, err := registry.Gather()
	require.NoError(t, err)

	var found bool
	for _, family := range metricFamilies {
		if family.GetName() != "test_provider_requests_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if len(metric.GetLabel()) == 1 && metric.GetLabel()[0].GetValue() == operationCreateSecret {
				assert.Equal(t, float64(1), metric.GetCounter().GetValue())
				found = true
			}
		}
	}
	assert.True(t, found, "create_secret provider metric was not recorded")
}
