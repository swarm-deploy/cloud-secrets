package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/api/auth/approle"
)

type AppRoleAuthenticator struct {
	appRole        *approle.AppRoleAuth
	client         *vaultapi.Client
	tokenExpiresAt time.Time
	mu             sync.Mutex
	now            func() time.Time
}

func NewAppRoleAuthenticator(
	client *vaultapi.Client,
	cfg AppRoleConfig,
) (*AppRoleAuthenticator, error) {
	appRole, err := approle.NewAppRoleAuth(cfg.RoleID, &approle.SecretID{
		FromString: cfg.SecretID,
	})
	if err != nil {
		return nil, fmt.Errorf("create auth: %w", err)
	}

	return &AppRoleAuthenticator{
		appRole: appRole,
		client:  client,
		now:     time.Now,
	}, nil
}

func (a *AppRoleAuthenticator) Authenticate(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.tokenExpiresAt.IsZero() && a.now().Before(a.tokenExpiresAt) {
		return nil
	}

	return a.authenticateLocked(ctx)
}

func (a *AppRoleAuthenticator) authenticateLocked(ctx context.Context) error {
	secret, err := a.appRole.Login(ctx, a.client)
	if err != nil {
		return fmt.Errorf("login: %w", err)
	}
	if secret.Auth == nil {
		return errors.New("given secret not contains auth data")
	}

	a.client.SetToken(secret.Auth.ClientToken)
	a.tokenExpiresAt = a.now().Add(time.Duration(secret.Auth.LeaseDuration) * time.Second)

	slog.InfoContext(ctx, "[vault/auth] given new token", slog.Any("expired_at", a.tokenExpiresAt))

	return nil
}
