package vault

import (
	"errors"
	"net/url"
	"strings"
)

// Config defines Vault provider settings.
type Config struct {
	// Address is Vault HTTP API endpoint.
	Address url.URL `env:"ADDRESS"`
	// Token is Vault token used for API authentication.
	Token Token `env:"TOKEN,file" json:"-"`
	// MountPath is KV v2 mount path.
	MountPath string `env:"MOUNT_PATH" envDefault:"secret"`
	// Prefix limits synchronization to a subtree under mount path.
	Prefix string `env:"PREFIX"`
}

type Token struct {
	Value string
}

func (c *Config) Validate() error {
	c.MountPath = strings.Trim(c.MountPath, "/")
	c.Prefix = strings.Trim(c.Prefix, "/")

	if c.Address.Host == "" {
		return errors.New("VAULT_ADDRESS is required when CS_PROVIDER=vault")
	}

	if c.Token.Value == "" {
		return errors.New("VAULT_TOKEN is required when CS_PROVIDER=vault")
	}

	if c.MountPath == "" {
		return errors.New("VAULT_MOUNT_PATH is required when CS_PROVIDER=vault")
	}

	return nil
}

func (t *Token) UnmarshalText(text []byte) error {
	val := strings.TrimPrefix(string(text), "\uFEFF")
	val = strings.TrimSpace(val)

	t.Value = val

	return nil
}
