package api

import (
	"context"
	"fmt"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

const appRoleAuthPath = "approle"

func (c *HttpClient) AppRoleAuthEnabled(ctx context.Context) (bool, error) {
	mounts, err := c.client.Sys().ListAuthWithContext(ctx)
	if err != nil {
		return false, fmt.Errorf("list Vault auth methods: %w", err)
	}

	mount, ok := mounts[appRoleAuthPath+"/"]
	if !ok {
		return false, nil
	}
	if mount.Type != appRoleAuthPath {
		return false, fmt.Errorf("vault auth mount %q already exists with type %q", appRoleAuthPath, mount.Type)
	}

	return true, nil
}

func (c *HttpClient) EnableAppRoleAuth(ctx context.Context) error {
	err := c.client.Sys().EnableAuthWithOptionsWithContext(ctx, appRoleAuthPath, &vaultapi.EnableAuthOptions{
		Type: appRoleAuthPath,
	})
	if err != nil {
		return fmt.Errorf("enable Vault AppRole auth: %w", err)
	}

	return nil
}

func (c *HttpClient) CreateAppRole(ctx context.Context, req CreateAppRoleRequest) error {
	_, err := c.client.Logical().WriteWithContext(ctx, appRolePath(req.Name), map[string]interface{}{
		"token_policies":     req.TokenPolicies,
		"token_ttl":          req.TokenTTL,
		"token_max_ttl":      req.TokenMaxTTL,
		"secret_id_ttl":      req.SecretIDTTL,
		"secret_id_num_uses": req.SecretIDNumUses,
	})
	if err != nil {
		return fmt.Errorf("configure Vault AppRole %q: %w", req.Name, err)
	}

	return nil
}

func (c *HttpClient) ReadAppRoleRoleID(ctx context.Context, name string) (string, error) {
	secret, err := c.client.Logical().ReadWithContext(ctx, appRolePath(name)+"/role-id")
	if err != nil {
		return "", fmt.Errorf("read Vault AppRole %q role_id: %w", name, err)
	}

	roleID, err := readVaultString(secret, "role_id")
	if err != nil {
		return "", fmt.Errorf("read Vault AppRole %q role_id: %w", name, err)
	}

	return roleID, nil
}

func (c *HttpClient) CreateAppRoleSecretID(ctx context.Context, name string) (string, error) {
	secret, err := c.client.Logical().WriteWithContext(ctx, appRolePath(name)+"/secret-id", map[string]interface{}{})
	if err != nil {
		return "", fmt.Errorf("generate Vault AppRole %q secret_id: %w", name, err)
	}

	secretID, err := readVaultString(secret, "secret_id")
	if err != nil {
		return "", fmt.Errorf("generate Vault AppRole %q secret_id: %w", name, err)
	}

	return secretID, nil
}

func appRolePath(name string) string {
	return fmt.Sprintf("auth/%s/role/%s", appRoleAuthPath, strings.TrimSpace(name))
}

func readVaultString(secret *vaultapi.Secret, key string) (string, error) {
	if secret == nil || secret.Data == nil {
		return "", fmt.Errorf("vault returned empty response")
	}

	raw, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("vault response missing %q", key)
	}

	value, ok := raw.(string)
	if !ok || value == "" {
		return "", fmt.Errorf("vault response has invalid %q", key)
	}

	return value, nil
}
