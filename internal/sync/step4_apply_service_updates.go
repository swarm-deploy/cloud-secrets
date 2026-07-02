package sync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/moby/moby/api/types/swarm"
)

const stepApplyServices = "apply_service_updates"

func (s *Synchronizer) applyServiceUpdates(ctx context.Context, payload *syncPayload) error {
	if len(payload.pendingServiceUpdateOrder) == 0 {
		return nil
	}

	if len(payload.pendingServiceUpdateOrder) <= payload.pendingServiceOffset {
		return nil
	}

	for _, service := range payload.pendingServiceUpdateOrder[payload.pendingServiceOffset:] {
		secrets := make([]*swarm.SecretReference, 0, len(service.Service.Spec.TaskTemplate.ContainerSpec.Secrets))
		for _, secRef := range service.Service.Spec.TaskTemplate.ContainerSpec.Secrets {
			secret, ok := service.Secrets[secRef.SecretID]
			if !ok {
				secrets = append(secrets, secRef)
				continue
			}

			updatedRef := *secRef
			updatedRef.SecretID = secret.ID
			updatedRef.SecretName = secret.Name
			secrets = append(secrets, &updatedRef)
		}

		service.Service.Spec.TaskTemplate.ContainerSpec.Secrets = secrets

		slog.DebugContext(ctx, "[synchronizer] swap secrets in service",
			slog.String("service", service.Service.Spec.Name),
			slog.Any("new_secrets", secrets),
		)

		err := s.engine.UpdateService(ctx, service.Service)
		if err != nil {
			return fmt.Errorf("update service %q: %w", service.Service.Spec.Name, err)
		}

		payload.pendingServiceOffset++
	}

	return nil
}
