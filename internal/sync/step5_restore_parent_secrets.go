package sync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

const stepRestoreSecrets = "restore_parent_secrets"

func (s *Synchronizer) restorePendingSecrets(ctx context.Context, payload *syncPayload) error {
	return s.restoreSecrets(ctx, payload.pendingSecretRestores, payload.swarmSecretsMap)
}

func (s *Synchronizer) restoreSecrets(
	ctx context.Context,
	pendingSecretRestores []UpdatedSecret,
	swarmSecretsMap map[string]*engine.ExistingSecret,
) error {
	for _, secret := range pendingSecretRestores {
		slog.DebugContext(ctx, "[synchronizer] removing previous secret versions in engine",
			slog.String("secret.path", secret.Path),
			slog.Int("secret.previous_versions.count", len(swarmSecretsMap[secret.Path].Versions)),
		)

		parent := swarmSecretsMap[secret.Path]

		for _, prevVersion := range parent.Versions {
			slog.DebugContext(ctx, "[synchronizer] removing previous secret version in engine",
				slog.String("secret.path", secret.Path),
				slog.String("secret.prev_id", prevVersion.ID),
			)

			err := s.engine.RemoveSecret(ctx, prevVersion.ID)
			if err != nil {
				return fmt.Errorf("remove previous secret %q version %q: %w", secret.Path, prevVersion.ID, err)
			}
		}

		slog.DebugContext(ctx, "[synchronizer] restore parent secret", slog.String("secret.path", secret.Path))

		err := s.engine.CreateSecret(ctx, engine.CreatingSecret{
			Path:              parent.Path,
			Value:             secret.Value,
			ExternalPath:      parent.ExternalPath,
			ExternalVersionID: secret.ExternalID,
		})
		if err != nil {
			return fmt.Errorf("create new parent secret: %w", err)
		}
	}

	return nil
}
