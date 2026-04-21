package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

const (
	containerName = "cloud-secrets"
)

func main() {
	err := send(context.Background())
	if err != nil {
		slog.Error("failed to send signal", slog.Any("err", err))
		os.Exit(1)
	}
}

func send(ctx context.Context) error {
	containerID, err := findContainerID(ctx)
	if err != nil {
		return fmt.Errorf("find container id: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "kill", "--signal", "HUP", containerID)
	if output, killErr := cmd.CombinedOutput(); killErr != nil {
		return fmt.Errorf("send SIGHUP to container %q (%s): %v\n%s", containerName, containerID, killErr, string(output))
	}

	slog.Info("sent SIGHUP to container %q (%s)\n", containerName, containerID)

	return nil
}

func findContainerID(ctx context.Context) (string, error) {
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
