package sync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/moby/moby/api/types/swarm"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
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
		secrets := []*swarm.SecretReference{}
		for _, secRef := range service.Service.Spec.TaskTemplate.ContainerSpec.Secrets {
			if _, ok := service.Secrets[secRef.File.Name]; !ok {
				secrets = append(secrets, secRef)
			}
		}

		for _, secret := range service.Secrets {
			secrets = append(secrets, engine.NewSecretRef(secret.Path, secret.Name, secret.ID))
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
