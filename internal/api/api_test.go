package api

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/application/cs"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	cloudsecretssync "github.com/swarm-deploy/cloud-secrets/internal/sync"
	grpcapi "github.com/swarm-deploy/cloud-secrets/pkg/grpc-api"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestService_GetInfo(t *testing.T) {
	t.Parallel()

	lastSyncAt := time.Date(2026, time.September, 29, 21, 0, 0, 0, time.UTC)
	nextSyncAt := lastSyncAt.Add(5 * time.Minute)
	tests := []struct {
		name          string
		lastSyncAt    time.Time
		hasSync       bool
		nextSyncAt    time.Time
		hasNext       bool
		providerLinks contracts.Links
	}{
		{name: "without successful sync"},
		{
			name:       "with successful sync and schedule",
			lastSyncAt: lastSyncAt,
			hasSync:    true,
			nextSyncAt: nextSyncAt,
			hasNext:    true,
			providerLinks: contracts.Links{
				Doc:     "https://developer.hashicorp.com/vault/docs",
				Manager: "https://vault.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			provider := contracts.NewMockProvider(ctrl)
			provider.EXPECT().Definition().Return(contracts.ProviderDefinition{
				Name:  "HashiCorp Vault",
				Links: tt.providerLinks,
			})
			syncer := NewMockSyncer(ctrl)
			syncer.EXPECT().LastSyncAt().Return(tt.lastSyncAt, tt.hasSync)
			syncer.EXPECT().NextSyncAt().Return(tt.nextSyncAt, tt.hasNext)

			service := NewService("v1.2.3", provider, syncer)

			response, err := service.GetInfo(context.Background(), &grpcapi.GetInfoRequest{})
			require.NoError(t, err)
			assert.Equal(t, "v1.2.3", response.GetVersion())
			assert.Equal(t, "HashiCorp Vault", response.GetProvider().GetName())
			if tt.providerLinks != (contracts.Links{}) {
				require.NotNil(t, response.GetProvider().GetLinks())
				assert.Equal(t, tt.providerLinks.Doc, response.GetProvider().GetLinks().GetDoc())
				assert.Equal(t, tt.providerLinks.Manager, response.GetProvider().GetLinks().GetManager())
			} else {
				assert.Nil(t, response.GetProvider().GetLinks())
			}

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
	provider := contracts.NewMockProvider(ctrl)
	syncer := NewMockSyncer(ctrl)
	syncer.EXPECT().Sync(gomock.Any(), grpcSyncTrigger).Return(cloudsecretssync.Result{
		Created:               2,
		RemovedSecrets:        3,
		RemovedSecretVersions: 4,
		Updated:               5,
		Skipped:               7,
	}, nil)

	service := NewService("v1.2.3", provider, syncer)
	response, err := service.Sync(context.Background(), &grpcapi.SyncRequest{})

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
			provider := contracts.NewMockProvider(ctrl)
			syncer := NewMockSyncer(ctrl)
			syncer.EXPECT().Sync(gomock.Any(), grpcSyncTrigger).Return(cloudsecretssync.Result{}, tt.err)
			service := NewService("v1.2.3", provider, syncer)

			response, err := service.Sync(context.Background(), &grpcapi.SyncRequest{})

			assert.Nil(t, response)
			require.Error(t, err)
			assert.Equal(t, tt.code, status.Code(err))
		})
	}
}

func TestService_Create(t *testing.T) {
	t.Parallel()

	providerFailure := errors.New("provider unavailable")
	tests := []struct {
		name     string
		request  *grpcapi.CreateSecretRequest
		setup    func(*contracts.MockProvider)
		wantPath string
		wantURL  string
		wantCode codes.Code
	}{
		{
			name: "creates secret",
			request: &grpcapi.CreateSecretRequest{
				Path:        "/prod/db/",
				Value:       []byte("sensitive"),
				Key:         "password",
				Description: "database password",
			},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), contracts.CreateSecretRequest{
					Path:        "/prod/db/",
					Value:       []byte("sensitive"),
					Key:         "password",
					Description: "database password",
				}).Return(contracts.Secret{
					Path:   "prod/db",
					WebURL: "https://manager.example.com/prod/db",
				}, nil)
			},
			wantPath: "prod/db",
			wantURL:  "https://manager.example.com/prod/db",
			wantCode: codes.OK,
		},
		{
			name:    "omits empty manager URL",
			request: &grpcapi.CreateSecretRequest{Path: "prod/db"},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).Return(
					contracts.Secret{Path: "prod/db"},
					nil,
				)
			},
			wantPath: "prod/db",
			wantCode: codes.OK,
		},
		{
			name:     "rejects missing path",
			request:  &grpcapi.CreateSecretRequest{Value: []byte("sensitive")},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "rejects nil request",
			request:  nil,
			wantCode: codes.InvalidArgument,
		},
		{
			name:    "maps provider validation",
			request: &grpcapi.CreateSecretRequest{Path: "prod/db"},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).Return(
					contracts.Secret{},
					fmt.Errorf("%w: key is required", contracts.ErrInvalidCreateRequest),
				)
			},
			wantCode: codes.InvalidArgument,
		},
		{
			name:    "maps duplicate",
			request: &grpcapi.CreateSecretRequest{Path: "prod/db"},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).Return(
					contracts.Secret{},
					fmt.Errorf("%w: prod/db", contracts.ErrSecretAlreadyExists),
				)
			},
			wantCode: codes.AlreadyExists,
		},
		{
			name:    "maps concurrent conflict",
			request: &grpcapi.CreateSecretRequest{Path: "prod/db"},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).Return(contracts.Secret{}, contracts.ErrCreateConflict)
			},
			wantCode: codes.Aborted,
		},
		{
			name:    "hides provider failure",
			request: &grpcapi.CreateSecretRequest{Path: "prod/db"},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).Return(contracts.Secret{}, providerFailure)
			},
			wantCode: codes.Internal,
		},
		{
			name:    "preserves cancellation",
			request: &grpcapi.CreateSecretRequest{Path: "prod/db"},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).Return(contracts.Secret{}, context.Canceled)
			},
			wantCode: codes.Canceled,
		},
		{
			name:    "preserves deadline",
			request: &grpcapi.CreateSecretRequest{Path: "prod/db"},
			setup: func(provider *contracts.MockProvider) {
				provider.EXPECT().CreateSecret(gomock.Any(), gomock.Any()).Return(contracts.Secret{}, context.DeadlineExceeded)
			},
			wantCode: codes.DeadlineExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			provider := contracts.NewMockProvider(ctrl)
			if tt.setup != nil {
				tt.setup(provider)
			}

			service := NewService("v1.2.3", provider, nil)
			response, err := service.Create(context.Background(), tt.request)

			if tt.wantCode == codes.OK {
				require.NoError(t, err)
				require.NotNil(t, response)
				assert.Equal(t, tt.wantPath, response.GetPath())
				assert.Equal(t, tt.wantURL, response.GetManagerUrl())
				assert.Equal(t, tt.wantURL != "", response.ManagerUrl != nil)
				return
			}

			assert.Nil(t, response)
			require.Error(t, err)
			assert.Equal(t, tt.wantCode, status.Code(err))
			assert.NotContains(t, err.Error(), "sensitive")
			assert.NotContains(t, err.Error(), providerFailure.Error())
		})
	}
}
