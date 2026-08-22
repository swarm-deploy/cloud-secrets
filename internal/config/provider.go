package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/vault"
)

// ProviderName defines available external secrets backends.
type ProviderName string

const (
	// ProviderNameCloudRU selects Cloud.ru Secret Manager backend.
	ProviderNameCloudRU ProviderName = "cloudru"
	// ProviderNameVault selects HashiCorp Vault backend.
	ProviderNameVault ProviderName = "vault"
)

var ProviderNames = []string{
	string(ProviderNameCloudRU),
	string(ProviderNameVault),
}

func (t *ProviderName) UnmarshalText(text []byte) error {
	typ := ProviderName(text)
	if err := typ.Validate(); err != nil {
		return err
	}

	*t = typ

	return nil
}

func (t ProviderName) Validate() error {
	switch t {
	case ProviderNameCloudRU:
		return nil
	case ProviderNameVault:
		return nil
	default:
		return fmt.Errorf(
			"unsupported CS_PROVIDER=%q, supported values: %q, %q",
			t,
			ProviderNameCloudRU,
			ProviderNameVault,
		)
	}
}

func loadProviderConfig(cfg *Config) error {
	switch cfg.CloudSecrets.Provider {
	case ProviderNameCloudRU:
		return loadProviderCloudruConfig(cfg)
	case ProviderNameVault:
		return loadProviderVaultConfig(cfg)
	default:
		return fmt.Errorf(
			"unsupported CS_PROVIDER=%q, supported values: %q, %q",
			cfg.CloudSecrets.Provider,
			ProviderNameCloudRU,
			ProviderNameVault,
		)
	}
}

func loadProviderCloudruConfig(cfg *Config) error {
	providerConfig := cloudru.Config{}

	if err := env.ParseWithOptions(&providerConfig, env.Options{
		Prefix: "CLOUDRU_",
	}); err != nil {
		return fmt.Errorf("parse cloudru config: %w", err)
	}
	cfg.CloudRu = &providerConfig

	if err := cfg.CloudRu.Validate(); err != nil {
		return fmt.Errorf("validate cloudru config: %w", err)
	}

	return nil
}

func loadProviderVaultConfig(cfg *Config) error {
	providerConfig := vault.Config{}

	if err := env.ParseWithOptions(&providerConfig, env.Options{
		Prefix: "VAULT_",
	}); err != nil {
		return fmt.Errorf("parse vault config: %w", err)
	}
	cfg.Vault = &providerConfig

	if err := cfg.Vault.Validate(); err != nil {
		return fmt.Errorf("validate vault config: %w", err)
	}

	return nil
}
