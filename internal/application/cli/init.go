package cli

import (
	"context"
	"errors"
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
	case string(config.ProviderNameCloudru):
		return c.initCloudru(ctx, exec)
	}

	return nil
}

func (c *InitCommand) initCloudru(ctx context.Context, exec *framework.Execution) error {
	const (
		credentialMethodIndexStatic = 0
		credentialMethodIndexFromSM = 1
	)

	credentialsMethodChoices := []string{
		"I will specify static credentials and update them in swarm myself",
		"cloud-secrets will take the credentials from the Secrets Manager and I will update them there",
	}

	credentialMethodIndex, err := exec.AskWithChoices(
		"Now we need to determine how cloud secrets will receive credentials for Cloud.Ru.",
		credentialsMethodChoices,
	)
	if err != nil {
		return err
	}

	clientID, err := exec.AskPassword("Enter Client ID")
	if err != nil {
		return fmt.Errorf("ask client id: %w", err)
	}

	clientSecret, err := exec.AskPassword("Enter Client Secret")
	if err != nil {
		return fmt.Errorf("ask client id: %w", err)
	}

	switch credentialMethodIndex {
	case credentialMethodIndexStatic:
		yes := exec.Confirm("Now we will create two secrets in Swarm: `cloud-secrets-cloudru-iam-client-id` and `cloud-secrets-cloudru-iam-client-secret`") //nolint:lll // plain text
		if !yes {
			return errors.New("you refused create secrets in Swarm")
		}

		err = c.docker.CreateSecret(ctx, engine.CreatingSecret{
			Path:         "cloud-secrets-cloudru-iam-client-id",
			Value:        []byte(clientID),
			ExternalPath: "cloud-secrets/cloudru-iam-client-id",
		})
		if err != nil {
			return fmt.Errorf("create client-id secret: %w", err)
		}

		err = c.docker.CreateSecret(ctx, engine.CreatingSecret{
			Path:         "cloud-secrets-cloudru-iam-client-secret",
			Value:        []byte(clientSecret),
			ExternalPath: "cloud-secrets/cloudru-iam-client-secret",
		})
		if err != nil {
			return fmt.Errorf("create client-secret secret: %w", err)
		}
	}

	return nil
}
