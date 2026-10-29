package vault

import (
	"context"
	"crypto/sha256"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	vaultclient "github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
	"go.uber.org/mock/gomock"
)

func TestProvider_Definition(t *testing.T) {
	t.Parallel()

	provider := &Provider{cfg: Config{Addr: url.URL{
		Scheme: "https",
		Host:   "vault.example.com",
	}}}

	definition := provider.Definition()

	assert.Equal(t, "HashiCorp Vault", definition.Name)
	assert.Equal(t, "https://developer.hashicorp.com/vault/docs", definition.Links.Doc)
	assert.Empty(t, definition.Links.Manager)
}

func TestProvider_CreateSecret(t *testing.T) {
	t.Parallel()

	providerFailure := errors.New("provider unavailable")
	tests := []struct {
		name        string
		request     contracts.CreateSecretRequest
		setup       func(*vaultclient.MockClient)
		wantPath    string
		wantErr     error
		wantMessage string
	}{
		{
			name: "creates normalized document key",
			request: contracts.CreateSecretRequest{
				Path:  "/prod/db/",
				Key:   "password",
				Value: []byte("sensitive"),
			},
			setup: func(client *vaultclient.MockClient) {
				client.EXPECT().CreateSecretKey(gomock.Any(), "prod/db", "password", []byte("sensitive")).Return(nil)
			},
			wantPath: "prod/db",
		},
		{
			name:    "rejects empty path",
			request: contracts.CreateSecretRequest{Key: "password"},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name:    "rejects traversal",
			request: contracts.CreateSecretRequest{Path: "prod/../db", Key: "password"},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name:    "requires key",
			request: contracts.CreateSecretRequest{Path: "prod/db"},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name:    "rejects slash in key",
			request: contracts.CreateSecretRequest{Path: "prod/db", Key: "db/password"},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name: "rejects description",
			request: contracts.CreateSecretRequest{
				Path:        "prod/db",
				Key:         "password",
				Description: "database password",
			},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name: "rejects non UTF-8 value",
			request: contracts.CreateSecretRequest{
				Path:  "prod/db",
				Key:   "password",
				Value: []byte{0xff},
			},
			wantErr: contracts.ErrInvalidCreateRequest,
		},
		{
			name:    "maps duplicate key",
			request: contracts.CreateSecretRequest{Path: "prod/db", Key: "password"},
			setup: func(client *vaultclient.MockClient) {
				client.EXPECT().CreateSecretKey(gomock.Any(), "prod/db", "password", gomock.Any()).Return(
					vaultclient.ErrSecretKeyExists,
				)
			},
			wantErr: contracts.ErrSecretAlreadyExists,
		},
		{
			name:    "maps exhausted CAS conflict",
			request: contracts.CreateSecretRequest{Path: "prod/db", Key: "password"},
			setup: func(client *vaultclient.MockClient) {
				client.EXPECT().CreateSecretKey(gomock.Any(), "prod/db", "password", gomock.Any()).Return(
					vaultclient.ErrCASConflict,
				)
			},
			wantErr: contracts.ErrCreateConflict,
		},
		{
			name:    "wraps provider failure",
			request: contracts.CreateSecretRequest{Path: "prod/db", Key: "password"},
			setup: func(client *vaultclient.MockClient) {
				client.EXPECT().CreateSecretKey(gomock.Any(), "prod/db", "password", gomock.Any()).Return(providerFailure)
			},
			wantErr:     providerFailure,
			wantMessage: "create Vault secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			client := vaultclient.NewMockClient(ctrl)
			if tt.setup != nil {
				tt.setup(client)
			}

			provider := &Provider{client: client}
			path, err := provider.CreateSecret(context.Background(), tt.request)

			if tt.wantErr == nil {
				require.NoError(t, err)
				assert.Equal(t, tt.wantPath, path)
				return
			}

			assert.Empty(t, path)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			if tt.wantMessage != "" {
				assert.ErrorContains(t, err, tt.wantMessage)
			}
			assert.NotContains(t, err.Error(), "sensitive")
		})
	}
}

func TestProvider_CreatedSecretUsesExistingReadAndListPaths(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := vaultclient.NewMockClient(ctrl)
	client.EXPECT().CreateSecretKey(gomock.Any(), "prod/db", "password", []byte("sensitive")).Return(nil)
	client.EXPECT().ListKeys(gomock.Any(), "").Return([]string{"prod/"}, nil)
	client.EXPECT().ListKeys(gomock.Any(), "prod").Return([]string{"db"}, nil)
	client.EXPECT().GetSecret(gomock.Any(), "prod/db", "").Return(&vaultclient.Secret{
		VersionID: "1",
		Keys:      []string{"password"},
	}, nil)
	client.EXPECT().GetSecret(gomock.Any(), "prod/db", "password").Return(&vaultclient.Secret{
		VersionID: "1",
		Payload:   []byte("sensitive"),
	}, nil)

	provider := &Provider{client: client}
	createdPath, err := provider.CreateSecret(context.Background(), contracts.CreateSecretRequest{
		Path:  "prod/db",
		Key:   "password",
		Value: []byte("sensitive"),
	})
	require.NoError(t, err)
	assert.Equal(t, "prod/db", createdPath)

	secrets, err := provider.ListSecrets(context.Background())
	require.NoError(t, err)
	assert.Equal(t, contracts.Secret{Path: "prod/db/password", VersionID: "1"}, secrets["prod/db/password"])

	payload, err := provider.GetSecretPayload(context.Background(), "prod/db/password")
	require.NoError(t, err)
	assert.Equal(t, sha256.Sum256([]byte("sensitive")), sha256.Sum256(payload))
}
