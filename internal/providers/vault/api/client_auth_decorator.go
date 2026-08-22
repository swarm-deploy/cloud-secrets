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
