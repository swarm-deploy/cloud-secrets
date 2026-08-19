package vault

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	vaultclient "github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
)

func TestProviderListSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := vaultclient.NewMockClient(ctrl)
	provider := newTestProvider(client)

	gomock.InOrder(
		client.EXPECT().ListKeys(gomock.Any(), "cloud-secrets").Return([]string{"db", "json", "mixed", "nested/"}, nil),
		client.EXPECT().Get(gomock.Any(), "cloud-secrets/db", "").Return(&vaultclient.Secret{
			VersionID: "2",
			Keys:      []string{"value"},
		}, nil),
		client.EXPECT().Get(gomock.Any(), "cloud-secrets/json", "").Return(&vaultclient.Secret{
			VersionID: "3",
			Keys:      []string{"password", "username"},
		}, nil),
		client.EXPECT().Get(gomock.Any(), "cloud-secrets/mixed", "").Return(&vaultclient.Secret{
			VersionID: "5",
			Keys:      []string{"password", "value"},
		}, nil),
		client.EXPECT().ListKeys(gomock.Any(), "cloud-secrets/nested").Return([]string{"api"}, nil),
		client.EXPECT().Get(gomock.Any(), "cloud-secrets/nested/api", "").Return(&vaultclient.Secret{
			VersionID: "7",
			Keys:      []string{"value"},
		}, nil),
	)

	secrets, err := provider.ListSecrets(context.Background())
	require.NoError(t, err)

	require.Len(t, secrets, 6)
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/db/value",
		VersionID: "2",
	}, secrets["cloud-secrets/db/value"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/json/password",
		VersionID: "3",
	}, secrets["cloud-secrets/json/password"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/json/username",
		VersionID: "3",
	}, secrets["cloud-secrets/json/username"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/nested/api/value",
		VersionID: "7",
	}, secrets["cloud-secrets/nested/api/value"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/mixed/password",
		VersionID: "5",
	}, secrets["cloud-secrets/mixed/password"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/mixed/value",
		VersionID: "5",
	}, secrets["cloud-secrets/mixed/value"])
	assert.NotContains(t, secrets, "cloud-secrets/db")
	assert.NotContains(t, secrets, "cloud-secrets/nested/api")
}

func TestProviderGetSecretPayload(t *testing.T) {
	t.Run("value key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := vaultclient.NewMockClient(ctrl)
		provider := newTestProvider(client)

		client.EXPECT().Get(gomock.Any(), "cloud-secrets/db", "value").Return(&vaultclient.Secret{
			Payload: []byte("postgres://dsn"),
		}, nil)

		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/db/value")
		require.NoError(t, err)
		assert.Equal(t, []byte("postgres://dsn"), payload)
	})

	t.Run("json key payload", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := vaultclient.NewMockClient(ctrl)
		provider := newTestProvider(client)

		client.EXPECT().Get(gomock.Any(), "cloud-secrets/json", "password").Return(&vaultclient.Secret{
			Payload: []byte("secret"),
		}, nil)

		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/json/password")
		require.NoError(t, err)
		assert.Equal(t, []byte("secret"), payload)
	})

	t.Run("multi field payload with value key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := vaultclient.NewMockClient(ctrl)
		provider := newTestProvider(client)

		client.EXPECT().Get(gomock.Any(), "cloud-secrets/mixed", "password").Return(&vaultclient.Secret{
			Payload: []byte("secret"),
		}, nil)

		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/mixed/password")
		require.NoError(t, err)
		assert.Equal(t, []byte("secret"), payload)
	})

	t.Run("base path is invalid in always multi-key mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		client := vaultclient.NewMockClient(ctrl)
		provider := newTestProvider(client)

		client.EXPECT().Get(gomock.Any(), "cloud-secrets", "db").Return(nil, assert.AnError)

		_, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/db")
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func newTestProvider(client vaultclient.Client) *Provider {
	return &Provider{
		cfg: Config{
			MountPath: "secret",
			Prefix:    "cloud-secrets",
		},
		client: client,
	}
}
