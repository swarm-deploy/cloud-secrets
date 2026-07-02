package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

const stepRemoveOldVersions = "remove_old_secret_versions"

func (s *Synchronizer) removePendingOldVersions(ctx context.Context, payload *syncPayload) error {
	return s.removeOldVersions(ctx, &payload.result, payload.pendingVersionRemovals)
}

func (s *Synchronizer) removeOldVersions(
	ctx context.Context,
	result *Result,
	removals []SecretVersionRemoval,
) error {
	for _, removal := range removals {
		secret := removal.Secret
		slog.DebugContext(ctx, "[synchronizer] removing old secret versions in engine",
			slog.String("secret.path", secret.Path),
			slog.Int("secret.old_versions.count", len(secret.Versions)-1),
			slog.Bool("secret.remove_parent", removal.RemoveParent),
		)

		for _, version := range secret.Versions {
			if version.ID == secret.ID && !removal.RemoveParent {
				continue
			}

			slog.DebugContext(ctx, "[synchronizer] removing secret version in engine",
				slog.String("secret.path", secret.Path),
				slog.String("secret.version_id", version.ID),
			)

			err := s.engine.RemoveSecret(ctx, version.ID)
			if err != nil {
				if _, ok := errors.AsType[*engine.ErrSecretNotFound](err); ok {
					slog.DebugContext(ctx, "[synchronizer] secret version already removed in engine",
						slog.String("secret.path", secret.Path),
						slog.String("secret.version_id", version.ID),
					)

					continue
				}

				return fmt.Errorf("remove secret %q version %q: %w", secret.Path, version.ID, err)
			}

			if version.ID == secret.ID {
				result.RemovedSecrets++
				s.metrics.IncRemovedSecrets()
				continue
			}

			result.RemovedSecretVersions++
			s.metrics.IncRemovedVersions()
		}
	}

	return nil
}
