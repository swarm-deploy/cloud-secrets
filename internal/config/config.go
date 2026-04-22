package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/vault"
)

// ProviderType defines available external secrets backends.
type ProviderType string

const (
	// ProviderTypeCloudRU selects Cloud.ru Secret Manager backend.
	ProviderTypeCloudRU ProviderType = "cloudru"
	// ProviderTypeVault selects HashiCorp Vault backend.
	ProviderTypeVault ProviderType = "vault"
)

type Config struct {
	CloudRu cloudru.Config `envPrefix:"CLOUDRU_"`
	Vault   vault.Config   `envPrefix:"VAULT_"`

	CloudSecrets struct {
		Provider ProviderType `env:"PROVIDER" envDefault:"cloudru"`

		RefreshInterval time.Duration `env:"REFRESH_INTERVAL" envDefault:"5m"`

		Log struct {
			Level slog.Level `env:"LEVEL"`
		} `envPrefix:"LOG_"`
	} `envPrefix:"CS_"`
}

// Validate checks that the selected provider has all required settings.
func (c *Config) Validate() error {
	switch c.CloudSecrets.Provider {
	case ProviderTypeCloudRU:
		return c.CloudRu.Validate()
	case ProviderTypeVault:
		return c.Vault.Validate()
	default:
		return fmt.Errorf(
			"unsupported CS_PROVIDER=%q, supported values: %q, %q",
			c.CloudSecrets.Provider,
			ProviderTypeCloudRU,
			ProviderTypeVault,
		)
	}
}

func (t *ProviderType) UnmarshalText(text []byte) error {
	typ := ProviderType(text)
	if err := typ.Validate(); err != nil {
		return err
	}

	*t = typ

	return nil
}

func (t ProviderType) Validate() error {
	switch t {
	case ProviderTypeCloudRU:
		return nil
	case ProviderTypeVault:
		return nil
	default:
		return fmt.Errorf(
			"unsupported CS_PROVIDER=%q, supported values: %q, %q",
			t,
			ProviderTypeCloudRU,
			ProviderTypeVault,
		)
	}
}
