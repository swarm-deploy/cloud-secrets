package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
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

const (
	vaultDefaultMountPath = "secret"

	vaultPolicyName  = "cloud-secrets"
	vaultAppRoleName = "cloud-secrets"

	vaultStaticTokenSecretName     = "cloud-secrets-vault-token"
	vaultAppRoleRoleIDSecretName   = "vault-approle-role-id"   //nolint:gosec // Docker secret name, not a credential.
	vaultAppRoleSecretIDSecretName = "vault-approle-secret-id" //nolint:gosec // Docker secret name, not a credential.

	vaultEnvAddr      = "VAULT_ADDR"
	vaultEnvMountPath = "VAULT_MOUNT_PATH"
	vaultEnvToken     = "VAULT_TOKEN"
)

type VaultCommand struct {
	docker         engine.Client
	newVaultClient vaultClientFactory
	lookupSetupEnv func(string) string
}

type vaultClientFactory func(
	ctx context.Context,
	mountPath string,
	addr url.URL,
	authConfig api.AuthConfig,
) (api.Client, error)

func NewInitCommand(docker engine.Client) *VaultCommand {
	return &VaultCommand{
		docker:         docker,
		newVaultClient: api.NewHttpClient,
		lookupSetupEnv: os.Getenv,
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
			{
				Name:        "vault-addr",
				Description: "Vault HTTP API endpoint",
			},
			{
				Name:        "vault-mount-path",
				Description: "Vault KV v2 mount path",
			},
		},
	}
}

func (c *VaultCommand) Run(ctx context.Context, exec *framework.Execution) error {
	authMethod, err := c.selectAuthMethod(exec)
	if err != nil {
		return err
	}

	switch authMethod {
	case api.AuthMethodStatic:
		return c.initVaultTokenStatic(ctx, exec)
	case api.AuthMethodAppRole:
		return c.initVaultAppRole(ctx, exec)
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

	if c.isInteractive(exec) {
		confirmed := exec.Confirm("Now we'll create a secret in Docker Swarm that will be used by the cloud-secrets service. The secret will be named `cloud-secrets-vault-token`") //nolint:lll // nn
		if !confirmed {
			return errors.New("operation not confirmed")
		}
	}

	err = c.docker.CreateSecret(ctx, engine.CreatingSecret{
		Path:         vaultStaticTokenSecretName,
		Value:        []byte(token),
		ExternalPath: "cloud-secrets/vault-token",
	})
	if err != nil {
		return fmt.Errorf("create secret in Docker Swarm: %w", err)
	}

	exec.PrintSuccess("The secret has been created. In the future, you can add a secret named \"cloud-secrets-vault-token\" to Vault and update the token directly from Vault.") //nolint:lll // nn

	return nil
}

func (c *VaultCommand) initVaultAppRole(ctx context.Context, exec *framework.Execution) error {
	addr, err := c.vaultAddr(exec)
	if err != nil {
		return err
	}

	mountPath, err := c.vaultMountPath(exec)
	if err != nil {
		return err
	}

	token, err := c.vaultSetupToken(exec)
	if err != nil {
		return err
	}

	client, err := c.newVaultClient(ctx, mountPath, *addr, api.AuthConfig{
		Token: token,
	})
	if err != nil {
		return err
	}

	if c.isInteractive(exec) {
		exec.PrintText(fmt.Sprintf(
			"Configure Vault AppRole %q and policy %q for KV v2 mount %q.",
			vaultAppRoleName,
			vaultPolicyName,
			mountPath,
		))
	}

	if err = c.ensureAppRoleAuth(ctx, exec, client); err != nil {
		return err
	}

	if err = client.CreateACLPolicy(ctx, NewCloudSecretsACLPolicyRequest(vaultPolicyName, mountPath)); err != nil {
		return err
	}
	exec.PrintSuccess(fmt.Sprintf("Policy %q configured", vaultPolicyName))

	if err = client.CreateAppRole(ctx, vaultAppRoleName, vaultPolicyName); err != nil {
		return err
	}
	exec.PrintSuccess(fmt.Sprintf("AppRole %q configured", vaultAppRoleName))

	roleID, err := client.ReadAppRoleRoleID(ctx, vaultAppRoleName)
	if err != nil {
		return err
	}
	exec.PrintSuccess("RoleID obtained")

	secretID, err := client.CreateAppRoleSecretID(ctx, vaultAppRoleName)
	if err != nil {
		return err
	}
	exec.PrintSuccess("SecretID generated")

	if err = c.createVaultAppRoleDockerSecret(ctx, vaultAppRoleRoleIDSecretName, roleID); err != nil {
		return err
	}
	exec.PrintSuccess(fmt.Sprintf("Docker secret %q created", vaultAppRoleRoleIDSecretName))

	if err = c.createVaultAppRoleDockerSecret(ctx, vaultAppRoleSecretIDSecretName, secretID); err != nil {
		return err
	}
	exec.PrintSuccess(fmt.Sprintf("Docker secret %q created", vaultAppRoleSecretIDSecretName))

	exec.PrintSuccess("Vault AppRole is ready for cloud-secrets.")

	return nil
}

func (c *VaultCommand) vaultAddr(exec *framework.Execution) (*url.URL, error) {
	addr := strings.TrimSpace(exec.Input.Argument("vault-addr"))
	if addr == "" {
		addr = strings.TrimSpace(c.lookupSetupEnv(vaultEnvAddr))
	}
	if addr == "" && c.isInteractive(exec) {
		var err error
		addr, err = exec.Ask("Enter Vault address", "")
		if err != nil {
			return nil, err
		}
	}
	if addr == "" {
		return nil, errors.New("vault address not provided in command arguments or VAULT_ADDR")
	}

	parsed, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse Vault address %q: %w", addr, err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("vault address %q must include scheme and host", addr)
	}

	return parsed, nil
}

func NewCloudSecretsACLPolicyRequest(name string, mountPath string) api.CreateACLPolicyRequest {
	mountPath = strings.Trim(strings.TrimSpace(mountPath), "/")

	return api.CreateACLPolicyRequest{
		Name: name,
		Rules: fmt.Sprintf(`
path "%s/metadata" {
  capabilities = ["list"]
}

path "%s/metadata/*" {
  capabilities = ["read", "list"]
}

path "%s/data/*" {
  capabilities = ["read"]
}
`, mountPath, mountPath, mountPath),
	}
}

func (c *VaultCommand) vaultMountPath(exec *framework.Execution) (string, error) {
	mountPath := strings.TrimSpace(exec.Input.Argument("vault-mount-path"))
	if mountPath == "" {
		mountPath = strings.TrimSpace(c.lookupSetupEnv(vaultEnvMountPath))
	}
	if mountPath == "" && c.isInteractive(exec) {
		var err error
		mountPath, err = exec.Ask("Enter Vault KV v2 mount path", vaultDefaultMountPath)
		if err != nil {
			return "", err
		}
	}
	if mountPath == "" {
		mountPath = vaultDefaultMountPath
	}

	mountPath = strings.Trim(mountPath, "/")
	if mountPath == "" {
		return "", errors.New("vault KV v2 mount path is empty")
	}

	return mountPath, nil
}

func (c *VaultCommand) vaultSetupToken(exec *framework.Execution) (string, error) {
	if !c.isInteractive(exec) {
		token := strings.TrimSpace(c.lookupSetupEnv(vaultEnvToken))
		if token == "" {
			return "", errors.New("vault setup token not provided in VAULT_TOKEN")
		}

		return token, nil
	}

	return exec.AskPassword("Enter administrative Vault token for setup")
}

func (c *VaultCommand) createVaultAppRoleDockerSecret(ctx context.Context, name string, value string) error {
	err := c.docker.CreateSecret(ctx, engine.CreatingSecret{
		Path:         name,
		Value:        []byte(value),
		ExternalPath: "cloud-secrets/" + name,
	})
	if err != nil {
		return fmt.Errorf("create Docker Swarm secret %q: %w", name, err)
	}

	return nil
}

func (c *VaultCommand) ensureAppRoleAuth(
	ctx context.Context,
	exec *framework.Execution,
	client api.Client,
) error {
	enabled, err := client.AppRoleAuthEnabled(ctx)
	if err != nil {
		return fmt.Errorf("check Vault AppRole auth method: %w", err)
	}
	if !enabled {
		if c.isInteractive(exec) {
			confirmed := exec.Confirm("Vault AppRole auth is not enabled. Enable it now?")
			if !confirmed {
				return errors.New("operation not confirmed")
			}
		}

		if err = client.EnableAppRoleAuth(ctx); err != nil {
			return err
		}
	}
	exec.PrintSuccess("AppRole auth enabled")

	return nil
}

func (c *VaultCommand) isInteractive(exec *framework.Execution) bool {
	return exec.Input.IsInteractive()
}
