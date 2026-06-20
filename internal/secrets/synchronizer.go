package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/artarts36/gopipe"
	"github.com/moby/moby/api/types/swarm"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
)

type Synchronizer struct {
	engine          engine.Client
	secretProvider  contracts.Provider
	metrics         metrics.Secrets
	folderDelimiter secretname.FolderDelimiter

	pipeline *gopipe.Pipeline[*syncPayload]
}

func NewSynchronizer(
	engine engine.Client,
	secretProvider contracts.Provider,
	secretsMetrics metrics.Secrets,
	folderDelimiter secretname.FolderDelimiter,
) *Synchronizer {
	s := &Synchronizer{
		engine:          engine,
		secretProvider:  secretProvider,
		metrics:         secretsMetrics,
		folderDelimiter: folderDelimiter,
	}

	s.attachPipeline()

	return s
}

type Result struct {
	Created int
	Updated int
	Skipped int
}

const (
	stepLoadSwarmState    = "load_swarm_state"
	stepLoadExternalState = "load_external_state"
	stepProcessSecrets    = "process_secrets"
	stepApplyServices     = "apply_service_updates"
	stepRestoreSecrets    = "restore_parent_secrets"
)

type syncPayload struct {
	result Result

	servicesMap     map[string][]swarm.Service
	swarmSecretsMap map[string]*engine.ExistingSecret
	externalSecrets map[string]contracts.Secret

	pendingServiceUpdates     map[string]*ServiceTask
	pendingServiceUpdateOrder []*ServiceTask
	pendingSecretRestores     []UpdatedSecret
	pendingServiceOffset      int
}

type ServiceTask struct {
	Service swarm.Service
	Secrets map[string]UpdatingServiceSecret
}

type UpdatedSecret struct {
	Name  string
	ID    string
	Path  string
	Value []byte

	ExternalID string
}

type UpdatingServiceSecret struct {
	Name string
	ID   string
	Path string
}

func (s *Synchronizer) Sync(ctx context.Context) (Result, error) {
	payload := &syncPayload{
		pendingServiceUpdates: make(map[string]*ServiceTask),
		pendingSecretRestores: make([]UpdatedSecret, 0),
	}

	err := s.pipeline.Run(ctx, payload)
	if err == nil {
		return payload.result, nil
	}

	if stepErr, ok := err.(*gopipe.StepError); ok && stepErr.StepName == stepRestoreSecrets {
		return Result{}, err
	}

	return payload.result, err
}

func (s *Synchronizer) attachPipeline() {
	s.pipeline = gopipe.NewPipelineWithConfig[*syncPayload](gopipe.Config{
		PipelineName: "sync_secrets",
		Logger:       slog.Default(),
	})

	s.pipeline.Add(gopipe.Step[*syncPayload]{
		Name: stepLoadSwarmState,
		Run:  s.loadSwarmState,
	})

	s.pipeline.Add(gopipe.Step[*syncPayload]{
		Name:       stepLoadExternalState,
		Retries:    3,                      //nolint:mnd // it's look as const
		RetryDelay: 150 * time.Millisecond, //nolint:mnd // it's look as const
		Run:        s.loadExternalState,
	})

	s.pipeline.Add(gopipe.Step[*syncPayload]{
		Name: stepProcessSecrets,
		Run:  s.processExternalSecrets,
	})

	s.pipeline.Add(gopipe.Step[*syncPayload]{
		Name:       stepApplyServices,
		Retries:    3,                     //nolint:mnd // it's look as const
		RetryDelay: 50 * time.Millisecond, //nolint:mnd // it's look as const
		When: gopipe.When(func(payload *syncPayload) bool {
			return payload.hasPendingServiceUpdates()
		}),
		Run: s.applyServiceUpdates,
	})

	s.pipeline.Add(gopipe.Step[*syncPayload]{
		Name: stepRestoreSecrets,
		When: gopipe.When(func(payload *syncPayload) bool {
			return payload.hasPendingSecretRestores()
		}),
		Run: s.restorePendingSecrets,
	})
}

func (s *Synchronizer) loadSwarmState(ctx context.Context, payload *syncPayload) error {
	servicesMap, err := s.engine.MapServicesBySecrets(ctx)
	if err != nil {
		return fmt.Errorf("map services by secrets: %w", err)
	}

	swarmSecretsMap, err := s.engine.MapSecrets(ctx)
	if err != nil {
		return fmt.Errorf("list engine secrets: %w", err)
	}

	payload.servicesMap = servicesMap
	payload.swarmSecretsMap = swarmSecretsMap

	slog.DebugContext(ctx, "[synchronizer] fetched secrets from engine", slog.Any("secrets", payload.swarmSecretsMap))

	return nil
}

func (s *Synchronizer) loadExternalState(ctx context.Context, payload *syncPayload) error {
	externalSecrets, err := s.secretProvider.ListSecrets(ctx)
	if err != nil {
		return fmt.Errorf("list secrets in external storage: %w", err)
	}

	payload.externalSecrets = externalSecrets

	return nil
}

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
			payload.servicesMap[swarmSecret.Path],
			externalSecret,
			swarmSecret,
		)
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

	s.enqueueUpdatedServices(
		payload.pendingServiceUpdates,
		&payload.pendingServiceUpdateOrder,
		payload.servicesMap[swarmSecret.Path],
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
	externalSecret contracts.Secret,
	swarmSecret *engine.ExistingSecret,
) {
	for _, service := range services {
		for _, ref := range service.Spec.TaskTemplate.ContainerSpec.Secrets {
			if ref.File.Name == externalSecret.Path && ref.SecretID != swarmSecret.ID {
				task, ok := pendingServiceUpdates[service.ID]
				if !ok {
					task = &ServiceTask{
						Service: service,
						Secrets: make(map[string]UpdatingServiceSecret),
					}

					pendingServiceUpdates[service.ID] = task
					*pendingServiceUpdateOrder = append(*pendingServiceUpdateOrder, task)
				}

				task.Secrets[swarmSecret.Path] = UpdatingServiceSecret{
					Name: externalSecret.Path,
					ID:   swarmSecret.LatestVersion().ExternalID,
					Path: swarmSecret.Path,
				}
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
		task, ok := pendingServiceUpdates[service.ID]
		if !ok {
			task = &ServiceTask{
				Service: service,
				Secrets: make(map[string]UpdatingServiceSecret),
			}

			pendingServiceUpdates[service.ID] = task
			*pendingServiceUpdateOrder = append(*pendingServiceUpdateOrder, task)
		}

		task.Secrets[path] = UpdatingServiceSecret{
			Name: secret.Name,
			ID:   secret.ID,
			Path: path,
		}
	}
}

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

func (s *Synchronizer) restorePendingSecrets(ctx context.Context, payload *syncPayload) error {
	return s.restoreSecrets(ctx, payload.pendingSecretRestores, payload.swarmSecretsMap)
}

func (s *Synchronizer) restoreSecrets(
	ctx context.Context,
	pendingSecretRestores []UpdatedSecret,
	swarmSecretsMap map[string]*engine.ExistingSecret,
) error {
	for _, secret := range pendingSecretRestores {
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

func (s *Synchronizer) prepareSecretPath(path string) string {
	return strings.ReplaceAll(path, "/", string(s.folderDelimiter))
}

func (p *syncPayload) hasPendingChanges() bool {
	return p.hasPendingServiceUpdates() || p.hasPendingSecretRestores()
}

func (p *syncPayload) hasPendingServiceUpdates() bool {
	return len(p.pendingServiceUpdates) > 0
}

func (p *syncPayload) hasPendingSecretRestores() bool {
	return len(p.pendingSecretRestores) > 0
}
