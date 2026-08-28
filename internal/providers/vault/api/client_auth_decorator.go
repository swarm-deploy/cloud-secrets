package api

import (
	"context"
	"fmt"
)

type AuthenticatedClient struct {
	client        Client
	authenticator Authenticator
}

func (d *AuthenticatedClient) ListKeys(ctx context.Context, path string) ([]string, error) {
	err := d.authenticator.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	return d.client.ListKeys(ctx, path)
}

func (d *AuthenticatedClient) Get(ctx context.Context, path, key string) (*Secret, error) {
	err := d.authenticator.Authenticate(ctx)
	if err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	return d.client.Get(ctx, path, key)
}

func (d *AuthenticatedClient) CreateSecret(
	ctx context.Context,
	path string,
	data map[string]interface{},
) (*Secret, error) {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return nil, fmt.Errorf("authenticate: %w", err)
	}

	return d.client.CreateSecret(ctx, path, data)
}

func (d *AuthenticatedClient) DeleteSecret(ctx context.Context, path string) error {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	return d.client.DeleteSecret(ctx, path)
}

func (d *AuthenticatedClient) CreateACLPolicy(ctx context.Context, req CreateACLPolicyRequest) error {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	return d.client.CreateACLPolicy(ctx, req)
}

func (d *AuthenticatedClient) CreateToken(ctx context.Context, policies []string) (string, error) {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return "", fmt.Errorf("authenticate: %w", err)
	}

	return d.client.CreateToken(ctx, policies)
}

func (d *AuthenticatedClient) AppRoleAuthEnabled(ctx context.Context) (bool, error) {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return false, fmt.Errorf("authenticate: %w", err)
	}

	return d.client.AppRoleAuthEnabled(ctx)
}

func (d *AuthenticatedClient) EnableAppRoleAuth(ctx context.Context) error {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	return d.client.EnableAppRoleAuth(ctx)
}

func (d *AuthenticatedClient) CreateAppRole(ctx context.Context, name string, policyName string) error {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	return d.client.CreateAppRole(ctx, name, policyName)
}

func (d *AuthenticatedClient) ReadAppRoleRoleID(ctx context.Context, name string) (string, error) {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return "", fmt.Errorf("authenticate: %w", err)
	}

	return d.client.ReadAppRoleRoleID(ctx, name)
}

func (d *AuthenticatedClient) CreateAppRoleSecretID(ctx context.Context, name string) (string, error) {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return "", fmt.Errorf("authenticate: %w", err)
	}

	return d.client.CreateAppRoleSecretID(ctx, name)
}
