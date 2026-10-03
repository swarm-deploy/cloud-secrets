package sync

import (
	"context"
	"fmt"
	"log/slog"
)

const stepLoadExternalState = "load_external_state"

func (s *Synchronizer) loadExternalState(ctx context.Context, payload *syncPayload) error {
	externalSecrets, err := s.secretProvider.ListSecrets(ctx)
	if err != nil {
		return fmt.Errorf("list secrets in external storage: %w", err)
	}

	// Distinct provider paths can map to the same Docker name after folder normalization.
	paths := make(map[string]string, len(externalSecrets))
	for _, secret := range externalSecrets {
		path := s.prepareSecretPath(secret.Path)
		if previous, exists := paths[path]; exists {
			return fmt.Errorf("external secrets %q and %q map to the same Docker secret %q", previous, secret.FullPath, path)
		}
		paths[path] = secret.FullPath
	}
	payload.externalSecrets = externalSecrets

	slog.DebugContext(ctx, "[synchronizer] secrets loaded from external storage",
		slog.Any("secrets", externalSecrets),
	)

	return nil
}
