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

func (d *AuthenticatedClient) CreateACLPolicy(ctx context.Context, name string) error {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}

	return d.client.CreateACLPolicy(ctx, name)
}

func (d *AuthenticatedClient) CreateToken(ctx context.Context, policies []string) (string, error) {
	if err := d.authenticator.Authenticate(ctx); err != nil {
		return "", fmt.Errorf("authenticate: %w", err)
	}

	return d.client.CreateToken(ctx, policies)
}
