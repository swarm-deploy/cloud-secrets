package vault

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
)

const (
	EnvAuthTokenPath       = "VAULT_AUTH_TOKEN" //nolint:gosec // false-positive
	EnvAuthAppRoleRoleID   = "VAULT_AUTH_APPROLE_ROLE_ID"
	EnvAuthAppRoleSecretID = "VAULT_AUTH_APPROLE_SECRET_ID" //nolint:gosec // false-positive
)

// Config defines Vault provider settings.
type Config struct {
	// Addr is Vault HTTP API endpoint.
	Addr url.URL `env:"ADDR,required"`

	Auth api.AuthConfig `envPrefix:"AUTH_"`

	// MountPath is KV v2 mount path.
	MountPath string `env:"MOUNT_PATH" envDefault:"secret"`
	// Prefix limits synchronization to a subtree under mount path.
	Prefix string `env:"PREFIX"`
}

func (c *Config) Validate() error {
	c.Auth.Token = normalizeSecretValue(c.Auth.Token)
	c.Auth.AppRole.RoleID = normalizeSecretValue(c.Auth.AppRole.RoleID)
	c.Auth.AppRole.SecretID = normalizeSecretValue(c.Auth.AppRole.SecretID)

	tokenConfigured := c.Auth.Token != ""
	roleIDConfigured := c.Auth.AppRole.RoleID != ""
	secretIDConfigured := c.Auth.AppRole.SecretID != ""

	switch {
	case tokenConfigured && (roleIDConfigured || secretIDConfigured):
		return fmt.Errorf(
			"%s cannot be combined with %s or %s",
			EnvAuthTokenPath,
			EnvAuthAppRoleRoleID,
			EnvAuthAppRoleSecretID,
		)
	case roleIDConfigured && !secretIDConfigured:
		return fmt.Errorf("%s must be set when %s is set", EnvAuthAppRoleSecretID, EnvAuthAppRoleRoleID)
	case secretIDConfigured && !roleIDConfigured:
		return fmt.Errorf("%s must be set when %s is set", EnvAuthAppRoleRoleID, EnvAuthAppRoleSecretID)
	case !tokenConfigured && !roleIDConfigured:
		return fmt.Errorf(
			"vault authentication is not configured: set %s or both %s and %s",
			EnvAuthTokenPath,
			EnvAuthAppRoleRoleID,
			EnvAuthAppRoleSecretID,
		)
	}

	return nil
}

func normalizeSecretValue(text string) string {
	val := strings.TrimPrefix(text, "\uFEFF")
	return strings.TrimSpace(val)
}
