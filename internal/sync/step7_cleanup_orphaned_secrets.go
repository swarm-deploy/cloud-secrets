package sync

import (
	"context"
	"fmt"
	"log/slog"
)

const stepCleanupOrphanedSecrets = "cleanup_orphaned_secrets"

func (s *Synchronizer) cleanupOrphanedSecrets(ctx context.Context, payload *syncPayload) error {
	removals := payload.orphanedManagedSecretRemovals(s.folderDelimiter)
	if len(removals) == 0 {
		return nil
	}

	for _, removal := range removals {
		slog.DebugContext(ctx, "[synchronizer] cleanup orphaned secret",
			slog.String("secret.path", removal.Secret.Path),
			slog.Int("secret.versions.count", len(removal.Secret.Versions)),
		)
	}

	err := s.removeOldVersions(ctx, &payload.result, removals)
	if err != nil {
		return fmt.Errorf("cleanup orphaned secrets: %w", err)
	}

	return nil
}
