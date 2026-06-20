package sync

import (
	"context"
	"log/slog"
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
	Secrets map[string]updatingServiceSecret
}

type UpdatedSecret struct {
	Name  string
	ID    string
	Path  string
	Value []byte

	ExternalID string
}

type updatingServiceSecret struct {
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

func (p *syncPayload) hasPendingChanges() bool {
	return p.hasPendingServiceUpdates() || p.hasPendingSecretRestores()
}

func (p *syncPayload) hasPendingServiceUpdates() bool {
	return len(p.pendingServiceUpdates) > 0
}

func (p *syncPayload) hasPendingSecretRestores() bool {
	return len(p.pendingSecretRestores) > 0
}
