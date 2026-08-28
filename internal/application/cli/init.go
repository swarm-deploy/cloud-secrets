package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli/framework"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
)

var (
	vaultAuthMethodsTitles = []string{
		api.AuthMethodStatic.Title(),
		api.AuthMethodAppRole.Title(),
	}

	vaultAuthMethods = []api.AuthMethod{
		api.AuthMethodStatic,
		api.AuthMethodAppRole,
	}
)

type VaultCommand struct {
	docker engine.Client
}

func NewInitCommand(docker engine.Client) *VaultCommand {
	return &VaultCommand{
		docker: docker,
	}
}

func (c *VaultCommand) Definition() framework.Definition {
	return framework.Definition{
		Name:        "vault",
		Description: "Prepare Swarm and Vault to working with cloud-secrets",
		Arguments: []framework.Argument{
			{
				Name:        "auth-method",
				Description: "Authentication method for requests to Vault",
			},
		},
	}
}

func (c *VaultCommand) Run(ctx context.Context, exec *framework.Execution) error {
	authMethod, err := c.selectAuthMethod(exec)
	if err != nil {
		return err
	}

	switch authMethod { //nolint:gocritic // nn
	case api.AuthMethodStatic:
		return c.initVaultTokenStatic(ctx, exec)
	}

	return nil
}

func (c *VaultCommand) selectAuthMethod(exec *framework.Execution) (api.AuthMethod, error) {
	authMethodName := exec.Input.Argument("auth-method")
	if authMethodName != "" {
		for _, method := range vaultAuthMethods {
			if strings.ToLower(authMethodName) == string(method) {
				return method, nil
			}
		}
		return "", fmt.Errorf("given unknown auth method %q", authMethodName)
	}

	if !exec.Input.IsInteractive() {
		return "", errors.New("auth method not provided in command arguments")
	}

	authMethodIndex, err := exec.AskWithChoices("Choose auth method", vaultAuthMethodsTitles)
	if err != nil {
		return "", err
	}

	if authMethodIndex > len(vaultAuthMethods)-1 {
		return "", errors.New("invalid auth method")
	}

	return vaultAuthMethods[authMethodIndex], nil
}

func (c *VaultCommand) initVaultTokenStatic(ctx context.Context, exec *framework.Execution) error {
	token, err := exec.AskPassword("Enter static token for cloud-secrets")
	if err != nil {
		return err
	}

	if exec.Input.IsInteractive() {
		confirmed := exec.Confirm("Now we'll create a secret in Docker Swarm that will be used by the cloud-secrets service. The secret will be named `cloud-secrets-vault-token`") //nolint:lll // nn
		if !confirmed {
			return errors.New("operation not confirmed")
		}
	}

	err = c.docker.CreateSecret(ctx, engine.CreatingSecret{
		Path:         "cloud-secrets-vault-token",
		Value:        []byte(token),
		ExternalPath: "cloud-secrets/vault-token",
	})
	if err != nil {
		return fmt.Errorf("create secret in Docker Swarm: %w", err)
	}

	exec.PrintSuccess("The secret has been created. In the future, you can add a secret named \"cloud-secrets-vault-token\" to Vault and update the token directly from Vault.") //nolint:lll // nn

	return nil
}
