package vault

import (
	"fmt"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	vaultclient "github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
)

// Provider reads secrets from HashiCorp Vault KV v2.
type Provider struct {
	cfg Config

	client vaultclient.Client
}

func NewProvider(cfg Config) (*Provider, error) {
	vaultCfg := vaultapi.DefaultConfig()
	vaultCfg.Address = cfg.Addr.String()

	client, err := vaultapi.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}

	client.SetToken(cfg.Token.Value)

	cfg.MountPath = strings.Trim(cfg.MountPath, "/")
	cfg.Prefix = strings.Trim(cfg.Prefix, "/")

	return &Provider{
		cfg:    cfg,
		client: vaultclient.NewHttpClient(client, cfg.MountPath),
	}, nil
}

func (p *Provider) Definition() contracts.ProviderDefinition {
	return contracts.ProviderDefinition{
		Name: "HashiCorp Vault",
	}
}
