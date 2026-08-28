package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli/framework"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

const (
	containerName = "cloud-secrets"
)

type SyncCommand struct {
	docker *engine.DockerClient
}

func (c *SyncCommand) Definition() framework.Definition {
	return framework.Definition{
		Name:        "sync",
		Description: "Run synchronization",
	}
}

func NewSyncCommand(docker *engine.DockerClient) *SyncCommand {
	return &SyncCommand{
		docker: docker,
	}
}

func (c *SyncCommand) Run(ctx context.Context, _ *framework.Execution) error {
	containerID, err := c.docker.GetContainerID(ctx, "org.opencontainers.image.title=cloud-secrets")
	if err != nil {
		return fmt.Errorf("find container id: %w", err)
	}

	err = c.docker.SighUPContainer(ctx, containerID)
	if err != nil {
		return fmt.Errorf("send SIGHUP to container (%s): %w", containerID, err)
	}

	slog.Info("sent SIGHUP to container",
		slog.Any("container_name", containerName),
		slog.Any("container_id", containerID),
	)

	return nil
}
