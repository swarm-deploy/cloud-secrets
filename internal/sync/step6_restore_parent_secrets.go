package sync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

const stepRestoreSecrets = "restore_parent_secrets"

func (s *Synchronizer) restorePendingSecrets(ctx context.Context, payload *syncPayload) error {
	return s.restoreSecrets(ctx, payload.pendingSecretRestores)
}

func (s *Synchronizer) restoreSecrets(
	ctx context.Context,
	pendingSecretRestores []UpdatedSecret,
) error {
	for _, secret := range pendingSecretRestores {
		slog.DebugContext(ctx, "[synchronizer] restore parent secret", slog.String("secret.path", secret.Path))

		err := s.engine.CreateSecret(ctx, engine.CreatingSecret{
			Path:              secret.Path,
			Value:             secret.Value,
			ExternalPath:      secret.ExternalPath,
			ExternalVersionID: secret.ExternalID,
		})
		if err != nil {
			return fmt.Errorf("create new parent secret: %w", err)
		}
	}

	return nil
}
