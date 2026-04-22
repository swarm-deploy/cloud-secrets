package providers

import (
	"context"
	"fmt"

	"github.com/swarm-deploy/cloud-secrets/internal/config"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/vault"
)

func Create(
	ctx context.Context,
	cfg config.Config,
	providerMetrics metrics.Provider,
) (contracts.Provider, error) {
	var provider contracts.Provider

	switch cfg.CloudSecrets.Provider {
	case config.ProviderTypeCloudRU:
		p, err := cloudru.NewProvider(ctx, cfg.CloudRu)
		if err != nil {
			return nil, err
		}

		provider = p
	case config.ProviderTypeVault:
		p, err := vault.NewProvider(cfg.Vault)
		if err != nil {
			return nil, err
		}

		provider = p
	default:
		return nil, fmt.Errorf(
			"unsupported secrets provider %q, supported values: %q, %q",
			cfg.CloudSecrets.Provider,
			config.ProviderTypeCloudRU,
			config.ProviderTypeVault,
		)
	}

	return contracts.WithMetrics(provider, providerMetrics), nil
}
