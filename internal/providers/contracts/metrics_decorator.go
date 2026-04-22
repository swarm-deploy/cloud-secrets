package contracts

import (
	"context"
	"time"

	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
)

const (
	operationGetSecretPayload = "get_secret_payload"
	operationListSecrets      = "list_secrets"
)

type metricsDecorator struct {
	provider Provider
	metrics  metrics.Provider
}

// WithMetrics wraps provider calls with metrics recording.
func WithMetrics(provider Provider, metrics metrics.Provider) Provider {
	return &metricsDecorator{
		provider: provider,
		metrics:  metrics,
	}
}

func (d *metricsDecorator) GetSecretPayload(ctx context.Context, key string) ([]byte, error) {
	startedAt := time.Now()
	defer func() {
		d.metrics.RecordRequest(operationGetSecretPayload, time.Since(startedAt))
	}()

	return d.provider.GetSecretPayload(ctx, key)
}

func (d *metricsDecorator) ListSecrets(ctx context.Context) (map[string]Secret, error) {
	startedAt := time.Now()
	defer func() {
		d.metrics.RecordRequest(operationListSecrets, time.Since(startedAt))
	}()

	return d.provider.ListSecrets(ctx)
}
