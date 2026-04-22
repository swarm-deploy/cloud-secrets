package application //nolint:testpackage // Tests verify internal provider factory selection.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/config"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/vault"
)

type stubProvider struct {
	payload []byte
	secrets map[string]contracts.Secret
}

func (p stubProvider) GetSecretPayload(context.Context, string) ([]byte, error) {
	return p.payload, nil
}

func (p stubProvider) ListSecrets(context.Context) (map[string]contracts.Secret, error) {
	return p.secrets, nil
}

func TestCreateSecretProvider(t *testing.T) {
	cloudProvider := stubProvider{
		payload: []byte("cloud"),
		secrets: map[string]contracts.Secret{
			"cloud/path": {
				Path:      "cloud/path",
				VersionID: "1",
			},
		},
	}
	vaultProvider := stubProvider{
		payload: []byte("vault"),
		secrets: map[string]contracts.Secret{
			"vault/path": {
				Path:      "vault/path",
				VersionID: "2",
			},
		},
	}

	oldCloudFactory := cloudRUProviderFactory
	oldVaultFactory := vaultProviderFactory
	t.Cleanup(func() {
		cloudRUProviderFactory = oldCloudFactory
		vaultProviderFactory = oldVaultFactory
	})

	cloudRUProviderFactory = func(context.Context, cloudru.Config) (contracts.Provider, error) {
		return cloudProvider, nil
	}
	vaultProviderFactory = func(context.Context, vault.Config) (contracts.Provider, error) {
		return vaultProvider, nil
	}

	metricsGroup := metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test_create_secret_provider"})
	baseCfg := config.Config{}

	t.Run("cloudru", func(t *testing.T) {
		cfg := baseCfg
		cfg.CloudSecrets.Provider = config.ProviderTypeCloudRU

		provider, err := createSecretProvider(context.Background(), cfg, metricsGroup.Provider)
		require.NoError(t, err)

		secrets, err := provider.ListSecrets(context.Background())
		require.NoError(t, err)
		assert.Equal(t, cloudProvider.secrets, secrets)

		payload, err := provider.GetSecretPayload(context.Background(), "cloud/path")
		require.NoError(t, err)
		assert.Equal(t, cloudProvider.payload, payload)
	})

	t.Run("vault", func(t *testing.T) {
		cfg := baseCfg
		cfg.CloudSecrets.Provider = config.ProviderType("VaUlT")

		provider, err := createSecretProvider(context.Background(), cfg, metricsGroup.Provider)
		require.NoError(t, err)

		secrets, err := provider.ListSecrets(context.Background())
		require.NoError(t, err)
		assert.Equal(t, vaultProvider.secrets, secrets)

		payload, err := provider.GetSecretPayload(context.Background(), "vault/path")
		require.NoError(t, err)
		assert.Equal(t, vaultProvider.payload, payload)
	})

	t.Run("invalid", func(t *testing.T) {
		cfg := baseCfg
		cfg.CloudSecrets.Provider = config.ProviderType("unknown")

		_, err := createSecretProvider(context.Background(), cfg, metricsGroup.Provider)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported secrets provider")
	})
}
