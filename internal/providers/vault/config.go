package vault

import (
	"errors"
	"net/url"
)

// Config defines Vault provider settings.
type Config struct {
	// Address is Vault HTTP API endpoint.
	Address url.URL `env:"ADDRESS"`
	// Token is Vault token used for API authentication.
	Token string `env:"TOKEN,file" json:"-"`
	// MountPath is KV v2 mount path.
	MountPath string `env:"MOUNT_PATH" envDefault:"secret"`
	// Prefix limits synchronization to a sub-tree under mount path.
	Prefix string `env:"PREFIX"`
}

func (c *Config) Validate() error {
	if c.Address.Host == "" {
		return errors.New("VAULT_ADDRESS is required when CS_PROVIDER=vault")
	}

	if c.Token == "" {
		return errors.New("VAULT_TOKEN is required when CS_PROVIDER=vault")
	}

	if c.MountPath == "" {
		return errors.New("VAULT_MOUNT_PATH is required when CS_PROVIDER=vault")
	}

	return nil
}
