package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/moby/moby/api/types/swarm"
	dock "github.com/moby/moby/client"
)

// MapServicesBySecrets maps services by using secrets. key = secret.file.name.
func (c *Client) MapServicesBySecrets(ctx context.Context) (map[string][]swarm.Service, error) {
	resp, err := c.client.ServiceList(ctx, dock.ServiceListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list services in swarm: %w", err)
	}

	serviceSecretMap := map[string][]swarm.Service{}

	for _, item := range resp.Items {
		for _, secret := range item.Spec.TaskTemplate.ContainerSpec.Secrets {
			if _, ok := serviceSecretMap[secret.File.Name]; !ok {
				serviceSecretMap[secret.File.Name] = []swarm.Service{}
			}

			serviceSecretMap[secret.File.Name] = append(serviceSecretMap[secret.File.Name], item)
		}
	}

	return serviceSecretMap, nil
}

func (c *Client) UpdateService(ctx context.Context, service swarm.Service) error {
	warnings, err := c.client.ServiceUpdate(ctx, service.ID, dock.ServiceUpdateOptions{
		Version: service.Version,
		Spec:    service.Spec,
	})
	if err != nil {
		return fmt.Errorf("update service: %w", err)
	}

	for _, warning := range warnings.Warnings {
		slog.WarnContext(ctx, "[engine] warning on service update",
			slog.String("service", service.Spec.Name),
			slog.String("warning", warning),
		)
	}

	return nil
}
