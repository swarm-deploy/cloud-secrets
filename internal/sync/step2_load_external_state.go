package sync

import (
	"context"
	"fmt"
)

const stepLoadExternalState = "load_external_state"

func (s *Synchronizer) loadExternalState(ctx context.Context, payload *syncPayload) error {
	externalSecrets, err := s.secretProvider.ListSecrets(ctx)
	if err != nil {
		return fmt.Errorf("list secrets in external storage: %w", err)
	}

	payload.externalSecrets = externalSecrets

	return nil
}
