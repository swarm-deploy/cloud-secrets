package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/application/cs"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	cloudsecretssync "github.com/swarm-deploy/cloud-secrets/internal/sync"
	cloudsecretspb "github.com/swarm-deploy/cloud-secrets/pkg/api/cloudsecrets"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestService_GetInfo(t *testing.T) {
	t.Parallel()

	lastSyncAt := time.Date(2026, time.September, 29, 21, 0, 0, 0, time.UTC)
	nextSyncAt := lastSyncAt.Add(5 * time.Minute)
	tests := []struct {
		name       string
		lastSyncAt time.Time
		hasSync    bool
		nextSyncAt time.Time
		hasNext    bool
	}{
		{name: "without successful sync"},
		{
			name:       "with successful sync and schedule",
			lastSyncAt: lastSyncAt,
			hasSync:    true,
			nextSyncAt: nextSyncAt,
			hasNext:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			syncer := NewMockSyncer(ctrl)
			syncer.EXPECT().LastSyncAt().Return(tt.lastSyncAt, tt.hasSync)
			syncer.EXPECT().NextSyncAt().Return(tt.nextSyncAt, tt.hasNext)

			service := NewService("v1.2.3", contracts.ProviderDefinition{
				Name: "HashiCorp Vault",
				URL:  "https://vault.example.com",
			}, syncer)

			response, err := service.GetInfo(context.Background(), &cloudsecretspb.GetInfoRequest{})
			require.NoError(t, err)
			assert.Equal(t, "v1.2.3", response.GetVersion())
			assert.Equal(t, "HashiCorp Vault", response.GetProvider().GetName())
			assert.Equal(t, "https://vault.example.com", response.GetProvider().GetLink())

			if tt.hasSync {
				require.NotNil(t, response.GetLastSyncAt())
				assert.Equal(t, lastSyncAt, response.GetLastSyncAt().AsTime())
			} else {
				assert.Nil(t, response.GetLastSyncAt())
			}

			if tt.hasNext {
				require.NotNil(t, response.GetNextSyncAt())
				assert.Equal(t, nextSyncAt, response.GetNextSyncAt().AsTime())
			} else {
				assert.Nil(t, response.GetNextSyncAt())
			}
		})
	}
}

func TestService_Sync(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	syncer := NewMockSyncer(ctrl)
	syncer.EXPECT().Sync(gomock.Any(), grpcSyncTrigger).Return(cloudsecretssync.Result{
		Created:               2,
		RemovedSecrets:        3,
		RemovedSecretVersions: 4,
		Updated:               5,
		Skipped:               7,
	}, nil)

	service := NewService("v1.2.3", contracts.ProviderDefinition{}, syncer)
	response, err := service.Sync(context.Background(), &cloudsecretspb.SyncRequest{})

	require.NoError(t, err)
	assert.Equal(t, uint32(2), response.GetCreated())
	assert.Equal(t, uint32(5), response.GetUpdated())
	assert.Equal(t, uint32(3), response.GetRemoved())
	assert.Equal(t, uint32(7), response.GetUnchanged())
}

func TestService_Sync_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "sync already running", err: cs.ErrSyncInProgress, code: codes.Aborted},
		{name: "request canceled", err: context.Canceled, code: codes.Canceled},
		{name: "deadline exceeded", err: context.DeadlineExceeded, code: codes.DeadlineExceeded},
		{name: "sync failed", err: errors.New("provider unavailable"), code: codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			syncer := NewMockSyncer(ctrl)
			syncer.EXPECT().Sync(gomock.Any(), grpcSyncTrigger).Return(cloudsecretssync.Result{}, tt.err)
			service := NewService("v1.2.3", contracts.ProviderDefinition{}, syncer)

			response, err := service.Sync(context.Background(), &cloudsecretspb.SyncRequest{})

			assert.Nil(t, response)
			require.Error(t, err)
			assert.Equal(t, tt.code, status.Code(err))
		})
	}
}
