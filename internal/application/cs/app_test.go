package cs

import (
	"context"
	"errors"
	"testing"

	"github.com/moby/moby/api/types/swarm"
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
		})
	}
}
