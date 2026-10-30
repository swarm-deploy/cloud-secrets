package cloudru

import (
	"context"
	"crypto/sha256"
	"testing"

	v2 "github.com/cloudru-tech/secret-manager-sdk/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestProvider_resolveSecretPath(t *testing.T) {
	t.Parallel()

	provider := Provider{
		cfg: Config{
			RootFolder:           RootFolder("prod/apps"),
			RootFolderOmitPrefix: true,
		},
	}

	assert.Equal(t, "prod/apps/api/db/password", provider.resolveSecretPath("api/db/password"))
	assert.Equal(t, "prod/apps/api/db/password", provider.resolveSecretPath("prod/apps/api/db/password"))
}

func TestProvider_CreateSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		config       Config
		request      contracts.CreateSecretRequest
		providerErr  error
		wantPath     string
		wantFullPath string
		wantErr      error
	}{
		{
			name: "creates full path when prefix is retained",
			config: Config{
				ProjectID:  "project-id",
				RootFolder: RootFolder("prod/apps"),
			},
			request: contracts.CreateSecretRequest{
				Path:        "/prod/apps/api/db/password/",
				Value:       []byte("sensitive"),
				Description: "database password",
			},
			wantPath:     "prod/apps/api/db/password",
			wantFullPath: "prod/apps/api/db/password",
		},
		{
			name: "adds omitted root prefix",
			config: Config{
				ProjectID:            "project-id",
				RootFolder:           RootFolder("prod/apps"),
				RootFolderOmitPrefix: true,
			},
			request: contracts.CreateSecretRequest{
				Path:        "api/db/password",
				Value:       []byte("sensitive"),
				Description: "database password",
			},
			wantPath:     "api/db/password",
			wantFullPath: "prod/apps/api/db/password",
		},
		{
			name: "normalizes an included root prefix when omit is enabled",
			config: Config{
				ProjectID:            "project-id",
				RootFolder:           RootFolder("prod/apps"),
				RootFolderOmitPrefix: true,
			},
			request: contracts.CreateSecretRequest{
				Path:  "prod/apps/api/db/password",
				Value: []byte("sensitive"),
			},
			wantPath:     "api/db/password",
			wantFullPath: "prod/apps/api/db/password",
		},
		{
			name: "maps duplicate",
			config: Config{
				ProjectID: "project-id",
			},
			request:      contracts.CreateSecretRequest{Path: "api/db/password"},
			providerErr:  status.Error(codes.AlreadyExists, "duplicate"),
			wantFullPath: "api/db/password",
			wantErr:      contracts.ErrSecretAlreadyExists,
		},
		{
			name:    "rejects key",
			request: contracts.CreateSecretRequest{Path: "api/db/password", Key: "password"},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name: "rejects path outside retained root",
			config: Config{
				RootFolder: RootFolder("prod/apps"),
			},
			request: contracts.CreateSecretRequest{Path: "other/api/db/password"},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name:    "rejects traversal",
			request: contracts.CreateSecretRequest{Path: "api/../password"},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			client := NewMocksecretServiceClient(ctrl)
			if tt.wantFullPath != "" {
				client.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, request *v2.CreateSecretRequest, _ ...grpc.CallOption) (*v2.CreateSecretResponse, error) {
						assert.Equal(t, tt.config.ProjectID, request.GetProjectId())
						assert.Equal(t, tt.wantFullPath, request.GetPath())
						assert.Equal(t, tt.request.Description, request.GetDescription())
						assert.Equal(
							t,
							sha256.Sum256(tt.request.Value),
							sha256.Sum256(request.GetPayload().GetValue()),
						)

						return &v2.CreateSecretResponse{}, tt.providerErr
					},
				)
			}

			provider := Provider{cfg: tt.config, secretService: client}
			secret, err := provider.CreateSecret(context.Background(), tt.request)

			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.Equal(t, contracts.Secret{
					Path:        tt.wantPath,
					FullPath:    tt.wantFullPath,
					Description: tt.request.Description,
					WebURL: "https://console.cloud.ru/spa/secret-manager/show/" + tt.wantFullPath +
						"?projectId=" + tt.config.ProjectID,
				}, secret)
				return
			}

			assert.Empty(t, secret)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), "sensitive")
		})
	}
}

func TestProvider_CreatedSecretUsesExistingReadAndListPaths(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := NewMocksecretServiceClient(ctrl)
	client.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request *v2.CreateSecretRequest, _ ...grpc.CallOption) (*v2.CreateSecretResponse, error) {
			assert.Equal(t, "prod/apps/api/db/password", request.GetPath())
			assert.Equal(
				t,
				sha256.Sum256([]byte("sensitive")),
				sha256.Sum256(request.GetPayload().GetValue()),
			)

			return &v2.CreateSecretResponse{}, nil
		},
	)
	client.EXPECT().Search(gomock.Any(), &v2.SearchSecretRequest{
		ProjectId: "project-id",
		Path:      "prod/apps",
		Depth:     -1,
	}).Return(&v2.SearchSecretResponse{Secrets: []*v2.Secret{
		{
			Path: "prod/apps/api/db/password",
			Versions: []*v2.SecretVersion{
				{Id: 1, State: v2.VersionState_ENABLED},
			},
		},
	}}, nil)
	client.EXPECT().Access(gomock.Any(), &v2.AccessSecretRequest{
		ProjectId: "project-id",
		Path:      "prod/apps/api/db/password",
	}).Return(&v2.AccessSecretResponse{Payload: wrapperspb.Bytes([]byte("sensitive"))}, nil)

	provider := Provider{
		cfg: Config{
			ProjectID:            "project-id",
			RootFolder:           RootFolder("prod/apps"),
			RootFolderOmitPrefix: true,
		},
		secretService: client,
	}

	createdSecret, err := provider.CreateSecret(context.Background(), contracts.CreateSecretRequest{
		Path:  "api/db/password",
		Value: []byte("sensitive"),
	})
	require.NoError(t, err)
	assert.Equal(t, contracts.Secret{
		Path:     "api/db/password",
		FullPath: "prod/apps/api/db/password",
		WebURL:   "https://console.cloud.ru/spa/secret-manager/show/prod/apps/api/db/password?projectId=project-id",
	}, createdSecret)

	secrets, err := provider.ListSecrets(context.Background())
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	assert.Equal(t, "api/db/password", secrets["prod/apps/api/db/password"].Path)
	assert.Empty(t, secrets["prod/apps/api/db/password"].WebURL)

	payload, err := provider.GetSecretPayload(context.Background(), createdSecret.Path)
	require.NoError(t, err)
	assert.Equal(t, sha256.Sum256([]byte("sensitive")), sha256.Sum256(payload))
}

func TestProvider_scopeSecretPath(t *testing.T) {
	t.Parallel()

	t.Run("keeps full path when omit prefix disabled", func(t *testing.T) {
		t.Parallel()

		provider := Provider{
			cfg: Config{
				RootFolder: RootFolder("prod/apps"),
			},
		}

		path, err := provider.scopeSecretPath("prod/apps/api/db/password")
		assert.NoError(t, err)
		assert.Equal(t, "prod/apps/api/db/password", path)
	})

	t.Run("omits configured root prefix", func(t *testing.T) {
		t.Parallel()

		provider := Provider{
			cfg: Config{
				RootFolder:           RootFolder("prod/apps"),
				RootFolderOmitPrefix: true,
			},
		}

		path, err := provider.scopeSecretPath("prod/apps/api/db/password")
		assert.NoError(t, err)
		assert.Equal(t, "api/db/password", path)
	})

	t.Run("fails for path outside root folder", func(t *testing.T) {
		t.Parallel()

		provider := Provider{
			cfg: Config{
				RootFolder:           RootFolder("prod/apps"),
				RootFolderOmitPrefix: true,
			},
		}

		_, err := provider.scopeSecretPath("prod/other/api/db/password")
		assert.EqualError(t, err, "path is outside root folder \"prod/apps\"")
	})
}
