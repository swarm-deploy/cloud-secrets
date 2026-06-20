package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/moby/moby/api/types/swarm"
	dock "github.com/moby/moby/client"
)

// ListServices loads all services from Swarm.
func (c *DockerClient) ListServices(ctx context.Context) ([]swarm.Service, error) {
	startedAt := time.Now()

	resp, err := c.client.ServiceList(ctx, dock.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services in swarm: %w", err)
	}
	c.metrics.RecordRequest("list_services", time.Since(startedAt))

	return resp.Items, nil
}

func (c *DockerClient) UpdateService(ctx context.Context, service swarm.Service) error {
	startedAt := time.Now()

	warnings, err := c.client.ServiceUpdate(ctx, service.ID, dock.ServiceUpdateOptions{
		Version: service.Version,
		Spec:    service.Spec,
	})
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}

	c.metrics.RecordRequest("update_service", time.Since(startedAt))

	for _, warning := range warnings.Warnings {
		slog.WarnContext(ctx, "[engine] warning on service update",
			slog.String("service", service.Spec.Name),
			slog.String("warning", warning),
		)
	}

	return nil
}
