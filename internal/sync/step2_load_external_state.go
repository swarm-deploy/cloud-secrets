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

	payload.externalSecrets = externalSecrets

	slog.DebugContext(ctx, "[synchronizer] secrets loaded from external storage",
		slog.Any("secrets", externalSecrets),
	)

	return nil
}
