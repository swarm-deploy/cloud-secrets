package sync

import (
	"context"
	"fmt"
	"log/slog"
)

const stepLoadSwarmState = "load_swarm_state"

func (s *Synchronizer) loadSwarmState(ctx context.Context, payload *syncPayload) error {
	services, err := s.engine.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}

	swarmSecretsMap, err := s.engine.MapSecrets(ctx)
	if err != nil {
		return fmt.Errorf("list engine secrets: %w", err)
	}

	payload.services = services
	payload.swarmSecretsMap = swarmSecretsMap

	slog.DebugContext(ctx, "[synchronizer] fetched secrets from engine", slog.Any("secrets", payload.swarmSecretsMap))

	return nil
}
