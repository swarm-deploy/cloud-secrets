package vault

import (
	"fmt"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

// Provider reads secrets from HashiCorp Vault KV v2.
type Provider struct {
	cfg Config

	client *vaultapi.Client
}

func NewProvider(cfg Config) (*Provider, error) {
	client, err := vaultapi.NewClient(&vaultapi.Config{
		Address: cfg.Address.String(),
	})
	if err != nil {
		return nil, fmt.Errorf("create vault client: %w", err)
	}

	client.SetToken(cfg.Token.Value)

	cfg.MountPath = strings.Trim(cfg.MountPath, "/")
	cfg.Prefix = strings.Trim(cfg.Prefix, "/")

	return &Provider{
		cfg:    cfg,
		client: client,
	}, nil
}
