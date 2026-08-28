package cli

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	go_console "github.com/DrSmithFr/go-console"
	"github.com/DrSmithFr/go-console/input"
	"github.com/DrSmithFr/go-console/input/argument"
	"github.com/DrSmithFr/go-console/output"
	"github.com/DrSmithFr/go-console/question"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/swarm-deploy/cloud-secrets/internal/application/cli/framework"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
)

func TestVaultCommandRunAppRoleAlreadyEnabled(t *testing.T) {
	ctx := context.Background()
	engineCtrl := gomock.NewController(t)
	docker := engine.NewMockClient(engineCtrl)
	gomock.InOrder(
		docker.EXPECT().CreateSecret(ctx, engine.CreatingSecret{
			Path:         vaultAppRoleRoleIDSecretName,
			Value:        []byte("role-id"),
			ExternalPath: "cloud-secrets/" + vaultAppRoleRoleIDSecretName,
		}),
		docker.EXPECT().CreateSecret(ctx, engine.CreatingSecret{
			Path:         vaultAppRoleSecretIDSecretName,
			Value:        []byte("generated-sensitive-value"),
			ExternalPath: "cloud-secrets/" + vaultAppRoleSecretIDSecretName,
		}),
	)
	vaultCtrl := gomock.NewController(t)
	vault := api.NewMockClient(vaultCtrl)
	gomock.InOrder(
		vault.EXPECT().AppRoleAuthEnabled(ctx).Return(true, nil),
		vault.EXPECT().CreateACLPolicy(ctx, NewCloudSecretsACLPolicyRequest(vaultPolicyName, "prod")).Return(nil),
		vault.EXPECT().CreateAppRole(ctx, NewCloudSecretsAppRoleRequest(vaultAppRoleName, vaultPolicyName)).Return(nil),
		vault.EXPECT().ReadAppRoleRoleID(ctx, vaultAppRoleName).Return("role-id", nil),
		vault.EXPECT().CreateAppRoleSecretID(ctx, vaultAppRoleName).Return("generated-sensitive-value", nil),
	)
	cmd := newTestVaultCommand(docker, vault, map[string]string{
		vaultEnvToken: "setup-token",
	})

	exec, out := newVaultTestExecution(t, []string{"approle", "http://vault:8200", "prod"}, false, "")

	err := cmd.Run(ctx, exec)
	require.NoError(t, err)

	require.Equal(t, "http://vault:8200", cmd.createdSetupAddr.String())
	require.Equal(t, "setup-token", cmd.createdSetupToken)
	require.Equal(t, "prod", cmd.createdSetupMountPath)

	outputText := out.Fetch()
	require.NotContains(t, outputText, "setup-token")
	require.NotContains(t, outputText, "generated-sensitive-value")
	require.Contains(t, outputText, "Vault AppRole is ready for cloud-secrets.")
}

func TestNewCloudSecretsACLPolicyRequest(t *testing.T) {
	req := NewCloudSecretsACLPolicyRequest("cloud-secrets", "/prod/")

	require.Equal(t, "cloud-secrets", req.Name)
	require.Equal(t, `path "prod/metadata" {
  capabilities = ["list"]
}

path "prod/metadata/*" {
  capabilities = ["read", "list"]
}

path "prod/data/*" {
  capabilities = ["read"]
}`, strings.TrimSpace(req.Rules))
}

func TestNewCloudSecretsAppRoleRequest(t *testing.T) {
	req := NewCloudSecretsAppRoleRequest("cloud-secrets", "cloud-secrets-policy")

	require.Equal(t, "cloud-secrets", req.Name)
	require.Equal(t, []string{"cloud-secrets-policy"}, req.TokenPolicies)
	require.Equal(t, "1h", req.TokenTTL)
	require.Equal(t, "4h", req.TokenMaxTTL)
	require.Equal(t, "0", req.SecretIDTTL)
	require.Zero(t, req.SecretIDNumUses)
}

func TestVaultCommandRunAppRoleEnablesAuthWhenMissing(t *testing.T) {
	ctx := context.Background()
	engineCtrl := gomock.NewController(t)
	docker := engine.NewMockClient(engineCtrl)
	gomock.InOrder(
		docker.EXPECT().CreateSecret(ctx, engine.CreatingSecret{
			Path:         vaultAppRoleRoleIDSecretName,
			Value:        []byte("role-id"),
			ExternalPath: "cloud-secrets/" + vaultAppRoleRoleIDSecretName,
		}),
		docker.EXPECT().CreateSecret(ctx, engine.CreatingSecret{
			Path:         vaultAppRoleSecretIDSecretName,
			Value:        []byte("generated-sensitive-value"),
			ExternalPath: "cloud-secrets/" + vaultAppRoleSecretIDSecretName,
		}),
	)
	vaultCtrl := gomock.NewController(t)
	vault := api.NewMockClient(vaultCtrl)
	gomock.InOrder(
		vault.EXPECT().AppRoleAuthEnabled(ctx).Return(false, nil),
		vault.EXPECT().EnableAppRoleAuth(ctx).Return(nil),
		vault.EXPECT().CreateACLPolicy(ctx, NewCloudSecretsACLPolicyRequest(
			vaultPolicyName,
			vaultDefaultMountPath,
		)).Return(nil),
		vault.EXPECT().CreateAppRole(ctx, NewCloudSecretsAppRoleRequest(vaultAppRoleName, vaultPolicyName)).Return(nil),
		vault.EXPECT().ReadAppRoleRoleID(ctx, vaultAppRoleName).Return("role-id", nil),
		vault.EXPECT().CreateAppRoleSecretID(ctx, vaultAppRoleName).Return("generated-sensitive-value", nil),
	)
	cmd := newTestVaultCommand(docker, vault, map[string]string{
		vaultEnvAddr:  "http://vault:8200",
		vaultEnvToken: "setup-token",
	})

	exec, _ := newVaultTestExecution(t, []string{"approle"}, false, "")

	err := cmd.Run(ctx, exec)
	require.NoError(t, err)

	require.Equal(t, vaultDefaultMountPath, cmd.createdSetupMountPath)
}

func TestVaultCommandRunAppRoleVaultAPIFailure(t *testing.T) {
	ctx := context.Background()
	engineCtrl := gomock.NewController(t)
	docker := engine.NewMockClient(engineCtrl)
	vaultCtrl := gomock.NewController(t)
	vault := api.NewMockClient(vaultCtrl)
	vault.EXPECT().AppRoleAuthEnabled(ctx).Return(false, errors.New("permission denied"))
	cmd := newTestVaultCommand(docker, vault, map[string]string{
		vaultEnvToken: "setup-token",
	})

	exec, _ := newVaultTestExecution(t, []string{"approle", "http://vault:8200"}, false, "")

	err := cmd.Run(ctx, exec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "check Vault AppRole auth method")
	require.Contains(t, err.Error(), "permission denied")
}

func TestVaultCommandRunAppRoleDockerSecretCreationFailure(t *testing.T) {
	ctx := context.Background()
	engineCtrl := gomock.NewController(t)
	docker := engine.NewMockClient(engineCtrl)
	gomock.InOrder(
		docker.EXPECT().CreateSecret(ctx, engine.CreatingSecret{
			Path:         vaultAppRoleRoleIDSecretName,
			Value:        []byte("role-id"),
			ExternalPath: "cloud-secrets/" + vaultAppRoleRoleIDSecretName,
		}),
		docker.EXPECT().CreateSecret(ctx, engine.CreatingSecret{
			Path:         vaultAppRoleSecretIDSecretName,
			Value:        []byte("generated-sensitive-value"),
			ExternalPath: "cloud-secrets/" + vaultAppRoleSecretIDSecretName,
		}).Return(errors.New("docker unavailable")),
	)
	vaultCtrl := gomock.NewController(t)
	vault := api.NewMockClient(vaultCtrl)
	gomock.InOrder(
		vault.EXPECT().AppRoleAuthEnabled(ctx).Return(true, nil),
		vault.EXPECT().CreateACLPolicy(ctx, NewCloudSecretsACLPolicyRequest(
			vaultPolicyName,
			vaultDefaultMountPath,
		)).Return(nil),
		vault.EXPECT().CreateAppRole(ctx, NewCloudSecretsAppRoleRequest(vaultAppRoleName, vaultPolicyName)).Return(nil),
		vault.EXPECT().ReadAppRoleRoleID(ctx, vaultAppRoleName).Return("role-id", nil),
		vault.EXPECT().CreateAppRoleSecretID(ctx, vaultAppRoleName).Return("generated-sensitive-value", nil),
	)
	cmd := newTestVaultCommand(docker, vault, map[string]string{
		vaultEnvToken: "setup-token",
	})

	exec, out := newVaultTestExecution(t, []string{"approle", "http://vault:8200"}, false, "")

	err := cmd.Run(ctx, exec)
	require.Error(t, err)
	require.Contains(t, err.Error(), `create Docker Swarm secret "vault-approle-secret-id"`)
	require.Contains(t, err.Error(), "docker unavailable")
	require.NotContains(t, out.Fetch(), "generated-sensitive-value")
}

func TestVaultCommandRunRequiresAuthMethodNonInteractive(t *testing.T) {
	ctx := context.Background()
	cmd := newTestVaultCommand(nil, nil, nil)
	exec, _ := newVaultTestExecution(t, nil, false, "")

	err := cmd.Run(ctx, exec)
	require.Error(t, err)
	require.Contains(t, err.Error(), "auth method not provided")
}

type testVaultCommand struct {
	*VaultCommand

	createdSetupAddr      url.URL
	createdSetupToken     string
	createdSetupMountPath string
}

func newTestVaultCommand(
	docker engine.Client,
	vault api.Client,
	env map[string]string,
) *testVaultCommand {
	cmd := &testVaultCommand{
		VaultCommand: NewInitCommand(docker),
	}
	cmd.newVaultClient = func(
		_ context.Context,
		mountPath string,
		addr url.URL,
		authConfig api.AuthConfig,
	) (api.Client, error) {
		cmd.createdSetupAddr = addr
		cmd.createdSetupToken = authConfig.Token
		cmd.createdSetupMountPath = mountPath

		return vault, nil
	}
	cmd.lookupSetupEnv = func(key string) string {
		return env[key]
	}

	return cmd
}

func newVaultTestExecution(
	t *testing.T,
	args []string,
	interactive bool,
	inputText string,
) (*framework.Execution, *output.BufferedOutput) {
	t.Helper()

	in := input.NewArgvInput(append([]string{"vault"}, args...))
	out := output.NewBufferedOutput(false, nil)
	script := go_console.NewScriptCustom(in, out, true)

	definition := NewInitCommand(nil).Definition()
	for _, arg := range definition.Arguments {
		script.AddInputArgument(argument.New(arg.Name, argument.Optional).SetDescription(arg.Description))
	}
	in.Parse()
	in.Validate()
	in.SetInteractive(interactive)

	return &framework.Execution{
		Script:   script,
		Question: question.NewHelper(strings.NewReader(inputText), out),
	}, out
}
