package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/artarts36/gopipe"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
)

type Synchronizer struct {
	engine          engine.Client
	secretProvider  contracts.Provider
	metrics         metrics.Secrets
	cleanupOrphaned bool
	folderDelimiter secretname.FolderDelimiter

	pipeline *gopipe.Pipeline[*syncPayload]
}

func NewSynchronizer(
	engine engine.Client,
	secretProvider contracts.Provider,
	secretsMetrics metrics.Secrets,
	cleanupOrphaned bool,
	folderDelimiter secretname.FolderDelimiter,
) *Synchronizer {
	s := &Synchronizer{
		engine:          engine,
		secretProvider:  secretProvider,
		metrics:         secretsMetrics,
		cleanupOrphaned: cleanupOrphaned,
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

func (s *Synchronizer) Sync(ctx context.Context) (Result, error) {
	payload := &syncPayload{
		pendingServiceUpdates:  make(map[string]*ServiceTask),
		pendingVersionRemovals: make([]SecretVersionRemoval, 0),
		pendingSecretRestores:  make([]UpdatedSecret, 0),
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
		Name: stepRemoveOldVersions,
		When: gopipe.When(func(payload *syncPayload) bool {
			return payload.hasPendingVersionRemovals()
		}),
		Run: s.removePendingOldVersions,
	})

	s.pipeline.Add(gopipe.Step[*syncPayload]{
		Name: stepRestoreSecrets,
		When: gopipe.When(func(payload *syncPayload) bool {
			return payload.hasPendingSecretRestores()
		}),
		Run: s.restorePendingSecrets,
	})

	if s.cleanupOrphaned {
		s.pipeline.Add(gopipe.Step[*syncPayload]{
			Name: stepLoadSwarmState,
			Run:  s.loadSwarmState,
		})

		s.pipeline.Add(gopipe.Step[*syncPayload]{
			Name: stepCleanupOrphanedSecrets,
			Run:  s.cleanupOrphanedSecrets,
		})
	}
}
