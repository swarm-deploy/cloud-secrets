package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/moby/moby/api/types/swarm"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
)

type Synchronizer struct {
	engine         *engine.Client
	secretProvider contracts.Provider
}

func NewSynchronizer(engine *engine.Client, secretProvider contracts.Provider) *Synchronizer {
	return &Synchronizer{
		engine:         engine,
		secretProvider: secretProvider,
	}
}

type Result struct {
	Created int
	Updated int
	Skipped int
}

func (s *Synchronizer) Sync(ctx context.Context) (Result, error) { //nolint:gocognit,funlen,lll // optimal solution not found
	servicesMap, err := s.engine.MapServicesBySecrets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("map services by secrets: %w", err)
	}

	swarmSecretsMap, err := s.engine.MapSecrets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list engine secrets: %w", err)
	}

	slog.DebugContext(ctx, "[synchronizer] fetched secrets from engine", slog.Any("secrets", swarmSecretsMap))

	result := Result{}

	externalSecrets, err := s.secretProvider.ListSecrets(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list secrets in external storage: %w", err)
	}

	pendingServices := newServiceQueue()

	for _, externalSecret := range externalSecrets {
		fixedSecretPath := prepareSecretPath(externalSecret.Path)

		swarmSecret, alreadyExists := swarmSecretsMap[fixedSecretPath]
		if !alreadyExists {
			payload, perr := s.secretProvider.GetSecretPayload(ctx, externalSecret.Path)
			if perr != nil {
				return result, fmt.Errorf("get payload of secret %q: %w", externalSecret.Path, perr)
			}

			err = s.engine.CreateSecret(ctx, engine.CreatingSecret{
				Path:              fixedSecretPath,
				Value:             payload,
				ExternalPath:      externalSecret.Path,
				ExternalVersionID: externalSecret.VersionID,
			})
			if err != nil {
				return result, fmt.Errorf("create secret %q: %w", fixedSecretPath, err)
			}

			result.Created++

			continue
		}

		if swarmSecret.LatestVersion().ExternalID == externalSecret.VersionID {
			services, ok := servicesMap[swarmSecret.Path]
			if ok {
				for _, service := range services {
					for _, ref := range service.Spec.TaskTemplate.ContainerSpec.Secrets {
						if ref.File.Name == externalSecret.Path && ref.SecretID != swarmSecret.ID {
							pendingServices.PushService(service, UpdatingServiceSecret{
								Name: externalSecret.Path,
								ID:   swarmSecret.LatestVersion().ExternalID,
								Path: swarmSecret.Path,
							})
						}
					}
				}
			}

			result.Skipped++
			continue
		}

		payload, perr := s.secretProvider.GetSecretPayload(ctx, externalSecret.Path)
		if perr != nil {
			return result, fmt.Errorf("get payload of secret %q: %w", externalSecret.Path, perr)
		}

		secretPayload := engine.CreatingSecretVersion{
			ExternalID: externalSecret.VersionID,
			Value:      payload,
		}

		vers, verr := s.engine.CreateSecretVersion(ctx, *swarmSecret, secretPayload)
		if verr != nil {
			return result, fmt.Errorf("create secret version: %w", verr)
		}

		services, ok := servicesMap[swarmSecret.Path]
		if ok {
			for _, service := range services {
				pendingServices.PushService(service, UpdatingServiceSecret{
					Name: vers.Name,
					ID:   vers.ID,
					Path: swarmSecret.Path,
				})
			}
		}

		pendingServices.PushSecret(UpdatedSecret{
			Name:       vers.Name,
			ID:         vers.ID,
			Path:       swarmSecret.Path,
			Value:      secretPayload.Value,
			ExternalID: externalSecret.VersionID,
		})

		result.Updated++
	}

	if len(pendingServices.services) == 0 && len(pendingServices.secrets) == 0 {
		slog.DebugContext(ctx, "[synchronizer] no updated secrets")
		return result, nil
	}

	// removing old secret in services.
	for _, service := range pendingServices.services {
		secrets := []*swarm.SecretReference{}
		for _, secRef := range service.Service.Spec.TaskTemplate.ContainerSpec.Secrets {
			if _, ok := service.Secrets[secRef.File.Name]; !ok {
				secrets = append(secrets, secRef)
			}
		}

		for _, secret := range service.Secrets {
			secrets = append(secrets, NewSecretRef(secret.Path, secret.Name, secret.ID))
		}

		service.Service.Spec.TaskTemplate.ContainerSpec.Secrets = secrets

		slog.DebugContext(ctx, "[synchronizer] swap secrets in service",
			slog.String("service", service.Service.Spec.Name),
			slog.Any("new_secrets", secrets),
		)

		err = s.engine.UpdateService(ctx, service.Service)
		if err != nil {
			return result, fmt.Errorf("update service %q: %w", service.Service.Spec.Name, err)
		}
	}

	err = s.restoreSecrets(ctx, pendingServices, swarmSecretsMap)
	if err != nil {
		return Result{}, fmt.Errorf("restore secrets: %w", err)
	}

	return result, nil
}

func (s *Synchronizer) restoreSecrets(
	ctx context.Context,
	pendingServices *TaskQueue,
	swarmSecretsMap map[string]*engine.ExistingSecret,
) error {
	for _, secret := range pendingServices.secrets {
		slog.DebugContext(ctx, "[synchronizer] removing previous secret versions in engine",
			slog.String("secret.path", secret.Path),
			slog.Int("secret.previous_versions.count", len(swarmSecretsMap[secret.Path].Versions)),
		)

		parent := swarmSecretsMap[secret.Path]

		for _, prevVersion := range parent.Versions {
			slog.DebugContext(ctx, "[synchronizer] removing previous secret version in engine",
				slog.String("secret.path", secret.Path),
				slog.String("secret.prev_id", prevVersion.ID),
			)

			err := s.engine.RemoveSecret(ctx, prevVersion.ID)
			if err != nil {
				return fmt.Errorf("remove previous secret %q version %q: %w", secret.Path, prevVersion.ID, err)
			}
		}

		slog.DebugContext(ctx, "[synchronizer] restore parent secret", slog.String("secret.path", secret.Path))

		err := s.engine.CreateSecret(ctx, engine.CreatingSecret{
			Path:              parent.Path,
			Value:             secret.Value,
			ExternalPath:      parent.ExternalPath,
			ExternalVersionID: secret.ExternalID,
		})
		if err != nil {
			return fmt.Errorf("create new parent secret: %w", err)
		}
	}

	return nil
}

func prepareSecretPath(path string) string {
	return strings.ReplaceAll(path, "/", "-")
}
