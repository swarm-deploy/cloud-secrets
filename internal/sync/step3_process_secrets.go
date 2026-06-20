package sync

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/moby/moby/api/types/swarm"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
)

const stepProcessSecrets = "process_secrets"

func (s *Synchronizer) processExternalSecrets(ctx context.Context, payload *syncPayload) error {
	for _, externalSecret := range payload.externalSecrets {
		err := s.processExternalSecret(ctx, payload, externalSecret)
		if err != nil {
			return err
		}
	}

	if !payload.hasPendingChanges() {
		slog.DebugContext(ctx, "[synchronizer] no updated secrets")
	}

	return nil
}

func (s *Synchronizer) processExternalSecret(
	ctx context.Context,
	payload *syncPayload,
	externalSecret contracts.Secret,
) error {
	fixedSecretPath := s.prepareSecretPath(externalSecret.Path)

	swarmSecret, alreadyExists := payload.swarmSecretsMap[fixedSecretPath]
	if !alreadyExists {
		return s.createMissingSecret(ctx, &payload.result, externalSecret, fixedSecretPath)
	}

	if swarmSecret.LatestVersion().ExternalID == externalSecret.VersionID {
		s.enqueueSameVersionServices(
			payload.pendingServiceUpdates,
			&payload.pendingServiceUpdateOrder,
			payload.services,
			swarmSecret,
		)

		if swarmSecret.Managed && len(swarmSecret.Versions) > 1 {
			payload.pendingVersionRemovals = append(payload.pendingVersionRemovals, swarmSecret)
		}

		payload.result.Skipped++

		return nil
	}

	return s.createUpdatedSecretVersion(ctx, payload, externalSecret, swarmSecret)
}

func (s *Synchronizer) createMissingSecret(
	ctx context.Context,
	result *Result,
	externalSecret contracts.Secret,
	fixedSecretPath string,
) error {
	payload, err := s.secretProvider.GetSecretPayload(ctx, externalSecret.Path)
	if err != nil {
		return fmt.Errorf("get payload of secret %q: %w", externalSecret.Path, err)
	}

	err = s.engine.CreateSecret(ctx, engine.CreatingSecret{
		Path:              fixedSecretPath,
		Value:             payload,
		ExternalPath:      externalSecret.Path,
		ExternalVersionID: externalSecret.VersionID,
	})
	if err != nil {
		return fmt.Errorf("create secret %q: %w", fixedSecretPath, err)
	}

	result.Created++
	s.metrics.IncCreated()

	return nil
}

func (s *Synchronizer) createUpdatedSecretVersion(
	ctx context.Context,
	payload *syncPayload,
	externalSecret contracts.Secret,
	swarmSecret *engine.ExistingSecret,
) error {
	secretPayload, err := s.getUpdatedSecretPayload(ctx, externalSecret)
	if err != nil {
		return err
	}

	createdVersion, err := s.engine.CreateSecretVersion(ctx, *swarmSecret, secretPayload)
	if err != nil {
		return fmt.Errorf("create secret version: %w", err)
	}

	swarmSecret.Managed = true

	s.enqueueUpdatedServices(
		payload.pendingServiceUpdates,
		&payload.pendingServiceUpdateOrder,
		payload.services,
		swarmSecret.Path,
		createdVersion,
	)
	payload.pendingSecretRestores = append(payload.pendingSecretRestores, UpdatedSecret{
		Name:       createdVersion.Name,
		ID:         createdVersion.ID,
		Path:       swarmSecret.Path,
		Value:      secretPayload.Value,
		ExternalID: externalSecret.VersionID,
	})

	payload.result.Updated++
	s.metrics.IncUpdated()

	return nil
}

func (s *Synchronizer) getUpdatedSecretPayload(
	ctx context.Context,
	externalSecret contracts.Secret,
) (engine.CreatingSecretVersion, error) {
	payload, err := s.secretProvider.GetSecretPayload(ctx, externalSecret.Path)
	if err != nil {
		return engine.CreatingSecretVersion{}, fmt.Errorf("get payload of secret %q: %w", externalSecret.Path, err)
	}

	return engine.CreatingSecretVersion{
		Path:       secretname.Generate(externalSecret.Path, s.folderDelimiter, externalSecret.VersionID),
		ExternalID: externalSecret.VersionID,
		Value:      payload,
	}, nil
}

func (s *Synchronizer) enqueueSameVersionServices(
	pendingServiceUpdates map[string]*ServiceTask,
	pendingServiceUpdateOrder *[]*ServiceTask,
	services []swarm.Service,
	swarmSecret *engine.ExistingSecret,
) {
	for _, service := range services {
		for _, ref := range service.Spec.TaskTemplate.ContainerSpec.Secrets {
			if ref.File.Name != swarmSecret.Path || ref.SecretID == swarmSecret.ID {
				continue
			}

			task, ok := pendingServiceUpdates[service.ID]
			if !ok {
				task = &ServiceTask{
					Service: service,
					Secrets: make(map[string]updatingServiceSecret),
				}

				pendingServiceUpdates[service.ID] = task
				*pendingServiceUpdateOrder = append(*pendingServiceUpdateOrder, task)
			}

			task.Secrets[swarmSecret.Path] = updatingServiceSecret{
				Name: swarmSecret.Path,
				ID:   swarmSecret.ID,
				Path: swarmSecret.Path,
			}
		}
	}
}

func (s *Synchronizer) enqueueUpdatedServices(
	pendingServiceUpdates map[string]*ServiceTask,
	pendingServiceUpdateOrder *[]*ServiceTask,
	services []swarm.Service,
	path string,
	secret engine.CreatedSecretVersion,
) {
	for _, service := range services {
		if !serviceUsesPath(service, path) {
			continue
		}

		task, ok := pendingServiceUpdates[service.ID]
		if !ok {
			task = &ServiceTask{
				Service: service,
				Secrets: make(map[string]updatingServiceSecret),
			}

			pendingServiceUpdates[service.ID] = task
			*pendingServiceUpdateOrder = append(*pendingServiceUpdateOrder, task)
		}

		task.Secrets[path] = updatingServiceSecret{
			Name: secret.Name,
			ID:   secret.ID,
			Path: path,
		}
	}
}

func serviceUsesPath(service swarm.Service, path string) bool {
	for _, ref := range service.Spec.TaskTemplate.ContainerSpec.Secrets {
		if ref.File.Name == path {
			return true
		}
	}

	return false
}

func (s *Synchronizer) prepareSecretPath(path string) string {
	return strings.ReplaceAll(path, "/", string(s.folderDelimiter))
}
