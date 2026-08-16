package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli/framework"
)

const (
	containerName = "cloud-secrets"
)

type SyncCommand struct {
}

func (c *SyncCommand) Definition() framework.Definition {
	return framework.Definition{
		Name:        "sync",
		Description: "Run synchronization",
	}
}

func (c *SyncCommand) Run(ctx context.Context, _ *framework.Execution) error {
	containerID, err := c.findContainerID(ctx)
	if err != nil {
		return fmt.Errorf("find container id: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "kill", "--signal", "HUP", containerID)
	if output, killErr := cmd.CombinedOutput(); killErr != nil {
		return fmt.Errorf("send SIGHUP to container %q (%s): %v\n%s", containerName, containerID, killErr, string(output))
	}

	slog.Info("sent SIGHUP to container",
		slog.Any("container_name", containerName),
		slog.Any("container_id", containerID),
	)

	return nil
}

func (c *SyncCommand) findContainerID(ctx context.Context) (string, error) {
	// Use shell as requested to resolve container ID by exact container name.
	cmd := exec.CommandContext(ctx, "sh", "-c",
		"docker ps --filter name="+containerName+" --format '{{.ID}}' | head -n1",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		return "", errors.New("container not found")
	}

	return containerID, nil
}
