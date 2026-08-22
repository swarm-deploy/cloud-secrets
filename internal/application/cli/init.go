package cli

import (
	"context"
	"fmt"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli/framework"
	"github.com/swarm-deploy/cloud-secrets/internal/config"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

type InitCommand struct {
	docker engine.Client
}

func NewInitCommand(docker engine.Client) *InitCommand {
	return &InitCommand{
		docker: docker,
	}
}

func (c *InitCommand) Definition() framework.Definition {
	return framework.Definition{
		Name:        "init",
		Description: "Prepare cluster to working with cloud-secrets",
	}
}

func (c *InitCommand) Run(ctx context.Context, exec *framework.Execution) error {
	secretProviderIndex, err := exec.AskWithChoices("Choose your secret provider", config.ProviderNames)
	if err != nil {
		return err
	}

	if secretProviderIndex < 0 || secretProviderIndex >= len(config.ProviderNames) {
		return fmt.Errorf("got invalid provider index %d", secretProviderIndex)
	}

	secretProviderName := config.ProviderNames[secretProviderIndex]

	switch secretProviderName {
	case string(config.ProviderNameCloudRU):
		return nil
	case string(config.ProviderNameVault):
		return c.initVault(ctx, exec)
	}

	return nil
}
