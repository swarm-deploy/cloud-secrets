package sync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

const stepRemoveOldVersions = "remove_old_secret_versions"

func (s *Synchronizer) removePendingOldVersions(ctx context.Context, payload *syncPayload) error {
	return s.removeOldVersions(ctx, payload.pendingVersionRemovals)
}

func (s *Synchronizer) removeOldVersions(ctx context.Context, secrets []*engine.ExistingSecret) error {
	for _, secret := range secrets {
		slog.DebugContext(ctx, "[synchronizer] removing old secret versions in engine",
			slog.String("secret.path", secret.Path),
			slog.Int("secret.old_versions.count", len(secret.Versions)-1),
		)

		for _, version := range secret.Versions {
			if version.ID == secret.ID {
				continue
			}

			slog.DebugContext(ctx, "[synchronizer] removing old secret version in engine",
				slog.String("secret.path", secret.Path),
				slog.String("secret.version_id", version.ID),
			)

			err := s.engine.RemoveSecret(ctx, version.ID)
			if err != nil {
				return fmt.Errorf("remove old secret %q version %q: %w", secret.Path, version.ID, err)
			}
		}
	}

	return nil
}
