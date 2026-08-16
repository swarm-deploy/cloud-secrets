package cli

import (
	"context"
	"fmt"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli/framework"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

type ListCommand struct {
	docker engine.Client
}

func NewListCommand(docker engine.Client) *ListCommand {
	return &ListCommand{docker: docker}
}

func (c *ListCommand) Definition() framework.Definition {
	return framework.Definition{
		Name:        "ls",
		Description: "List secrets in Swarm with mapping at external secrets",
	}
}

func (c *ListCommand) Run(ctx context.Context, exec *framework.Execution) error {
	secrets, err := c.docker.MapSecrets(ctx)
	if err != nil {
		return fmt.Errorf("get secrets from swarm: %w", err)
	}

	headers := []string{"ID", "Name", "External Path", "Version", "External Version"}
	table := make([][]string, len(secrets))

	i := 0
	for _, secret := range secrets {
		table[i] = []string{
			secret.ID,
			secret.Path,
			secret.ExternalPath,
			secret.LatestVersion().ID,
			secret.LatestVersion().ExternalID,
		}

		i++
	}

	err = exec.PrintTable(headers, table)
	if err != nil {
		return fmt.Errorf("print table: %w", err)
	}

	return nil
}
