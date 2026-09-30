package cs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/moby/moby/api/types/swarm"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
	cloudsecretssync "github.com/swarm-deploy/cloud-secrets/internal/sync"
	"go.uber.org/mock/gomock"
)

func TestApplication_Sync_LastSyncAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*engine.MockClient, *contracts.MockProvider)
		wantError bool
		wantTime  bool
	}{
		{
			name: "records successful sync",
			setup: func(engineClient *engine.MockClient, provider *contracts.MockProvider) {
				engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{}, nil)
				engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{}, nil)
				provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{}, nil)
			},
			wantTime: true,
		},
		{
			name: "does not record failed sync",
			setup: func(engineClient *engine.MockClient, _ *contracts.MockProvider) {
				engineClient.EXPECT().ListServices(gomock.Any()).Return(nil, errors.New("docker unavailable"))
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			engineClient := engine.NewMockClient(ctrl)
			provider := contracts.NewMockProvider(ctrl)
			tt.setup(engineClient, provider)

			metricsGroup := metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test"})
			registry := prometheus.NewRegistry()
			require.NoError(t, registry.Register(metricsGroup))
			app := &Application{
				metrics: metricsGroup,
				synchronizer: cloudsecretssync.NewSynchronizer(
					engineClient,
					provider,
					metricsGroup.Secrets,
					false,
					secretname.FolderDelimiter('-'),
				),
			}

			_, err := app.Sync(context.Background(), "test")
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			lastSyncAt, ok := app.LastSyncAt()
			assert.Equal(t, tt.wantTime, ok)
			assert.Equal(t, tt.wantTime, !lastSyncAt.IsZero())
			assert.Greater(t, prometheusGaugeValue(t, registry, "test_syncs_last_sync_at_unix"), float64(0))
		})
	}
}

func TestApplication_Run_SetsNextSyncAt(t *testing.T) {
	t.Parallel()

	const refreshInterval = time.Hour
	app := &Application{}
	app.cfg.CloudSecrets.RefreshInterval = refreshInterval

	ctx, cancel := context.WithCancel(context.Background())
	startedAt := time.Now()
	runResult := make(chan error, 1)
	go func() {
		runResult <- app.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		_, ok := app.NextSyncAt()
		return ok
	}, time.Second, 10*time.Millisecond)

	nextSyncAt, ok := app.NextSyncAt()
	require.True(t, ok)
	assert.WithinDuration(t, startedAt.Add(refreshInterval), nextSyncAt, time.Second)

	cancel()
	require.NoError(t, <-runResult)
	require.NoError(t, app.Close())
}

func TestApplication_NextSyncAt_AdvancesPastSchedule(t *testing.T) {
	t.Parallel()

	const refreshInterval = time.Hour
	initialNextSyncAt := time.Now().Add(-30 * time.Minute)
	app := &Application{}
	app.cfg.CloudSecrets.RefreshInterval = refreshInterval
	app.setNextSyncAt(initialNextSyncAt)

	nextSyncAt, ok := app.NextSyncAt()

	require.True(t, ok)
	assert.Equal(t, initialNextSyncAt.Add(refreshInterval), nextSyncAt)
	assert.True(t, nextSyncAt.After(time.Now()))
}

func TestApplication_NextSyncAt_PreservesFutureSchedule(t *testing.T) {
	t.Parallel()

	const refreshInterval = time.Hour
	initialNextSyncAt := time.Now().Add(30 * time.Minute)
	app := &Application{}
	app.cfg.CloudSecrets.RefreshInterval = refreshInterval
	app.setNextSyncAt(initialNextSyncAt)

	nextSyncAt, ok := app.NextSyncAt()

	require.True(t, ok)
	assert.Equal(t, initialNextSyncAt, nextSyncAt)
}

func prometheusGaugeValue(t *testing.T, gatherer prometheus.Gatherer, name string) float64 {
	t.Helper()

	metricFamilies, err := gatherer.Gather()
	require.NoError(t, err)

	for _, family := range metricFamilies {
		if family.GetName() != name {
			continue
		}

		require.Len(t, family.GetMetric(), 1)
		return family.GetMetric()[0].GetGauge().GetValue()
	}

	require.Failf(t, "metric not found", "metric %q was not gathered", name)
	return 0
}
