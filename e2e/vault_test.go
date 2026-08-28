package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/require"
	vaultclient "github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
	"github.com/swarm-deploy/dockertester"
)

func TestVaultWithStaticToken(t *testing.T) {
	testVault(t, setupVaultStaticTokenAuth)
}

func TestVaultWithAppRole(t *testing.T) {
	testVault(t, setupVaultAppRoleAuth)
}

type vaultAuthSetup func(
	t *testing.T,
	ctx context.Context,
	docker *dockertester.Tester,
	vault vaultclient.Client,
	vaultAddr url.URL,
) cloudSecretsVaultAuthConfig

func testVault(t *testing.T, setupAuth vaultAuthSetup) {
	if os.Getenv("CS_E2E") != "1" {
		t.Skip("set CS_E2E=1 to run Docker Swarm e2e tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	f := setupVaultEnv(t, ctx, setupAuth)

	t.Run("create secret and sync to Docker", func(t *testing.T) {
		t.Cleanup(func() {
			f.cleanupScenario(t)
		})

		versionID := f.createVaultSecret(t, "test-value")
		syncedSecret := f.docker.Secret().Wait(f.ctx, t, versionID, dockerSecretLabelMatcher)

		f.docker.Secret().AssertSecretValue(ctx, t, syncedSecret, "test-value")
	})

	t.Run("update secret and sync new Docker version", func(t *testing.T) {
		t.Cleanup(func() {
			f.cleanupScenario(t)
		})

		initialVersionID := f.createVaultSecret(t, "test-value")
		syncedSecret := f.docker.Secret().Wait(f.ctx, t, initialVersionID, dockerSecretLabelMatcher)

		f.docker.Secret().AssertSecretValue(ctx, t, syncedSecret, "test-value")

		updatedVersionID := f.createVaultSecret(t, "new-value")
		updatedSecret := f.docker.Secret().Wait(f.ctx, t, updatedVersionID, dockerSecretLabelMatcher)
		f.docker.Secret().AssertSecretValue(ctx, t, updatedSecret, "new-value")
	})
}

type vaultEnv struct {
	ctx       context.Context
	docker    *dockertester.Tester
	vault     vaultclient.Client
	networkID string
	runID     string
}

func setupVaultEnv(t *testing.T, ctx context.Context, setupAuth vaultAuthSetup) *vaultEnv {
	t.Helper()

	docker, err := dockertester.NewTester()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, docker.Close())
	})

	docker.Swarm().RequireSwarmMode(ctx, t)
	docker.Secret().RequireNotExists(ctx, t, dockerSecretName)

	runID := "cs-e2e-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	networkID, vaultServiceID, vaultPort := setupVaultService(t, ctx, docker, runID)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.Service().Delete(cleanupCtx, vaultServiceID))
		require.NoError(t, docker.Service().WaitRemoved(cleanupCtx, vaultServiceID))
		require.NoError(t, docker.Network().Delete(cleanupCtx, networkID))
	})

	vaultAddr := fmt.Sprintf("http://127.0.0.1:%d", vaultPort)
	require.NoError(t, waitVaultReady(ctx, vaultAddr))

	parsedVaultAddr, err := url.Parse(vaultAddr)
	require.NoError(t, err)

	vault, err := vaultclient.NewHttpClient(ctx, vaultMountPath, *parsedVaultAddr, vaultclient.AuthConfig{
		Token: vaultRootToken,
	})
	require.NoError(t, err)

	require.NoError(t, vault.CreateACLPolicy(ctx, vaultclient.CreateACLPolicyRequest{
		Name: vaultPolicyName,
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
`, vaultMountPath, vaultMountPath, vaultMountPath),
	}))

	authConfig := setupAuth(t, ctx, docker, vault, *parsedVaultAddr)

	time.Sleep(500 * time.Millisecond)

	cloudSecretsServiceID, err := docker.Service().Deploy(
		ctx,
		cloudSecretsServiceSpec(runID+"-cloud-secrets", cloudSecretsImage, networkID, authConfig),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.Service().Delete(cleanupCtx, cloudSecretsServiceID))
		require.NoError(t, docker.Service().WaitRemoved(cleanupCtx, cloudSecretsServiceID))
	})
	require.NoError(t, docker.Service().WaitHealthy(ctx, cloudSecretsServiceID))

	return &vaultEnv{
		ctx:       ctx,
		docker:    docker,
		vault:     vault,
		networkID: networkID,
		runID:     runID,
	}
}

func setupVaultService(
	t *testing.T,
	ctx context.Context,
	docker *dockertester.Tester,
	runID string,
) (string, string, uint32) {
	t.Helper()

	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		networkID, err := docker.Network().CreateSwarmOverlay(ctx, fmt.Sprintf("%s-net-%d", runID, attempt))
		if err != nil {
			lastErr = err
			continue
		}

		vaultPort, err := dockertester.FreeTCPPort()
		require.NoError(t, err)

		vaultServiceID, err := docker.Service().Deploy(
			ctx,
			vaultServiceSpec(fmt.Sprintf("%s-vault-%d", runID, attempt), vaultImage, networkID, vaultPort),
		)
		if err == nil {
			err = docker.Service().WaitHealthy(ctx, vaultServiceID)
		}
		if err == nil {
			return networkID, vaultServiceID, vaultPort
		}

		lastErr = err

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if vaultServiceID != "" {
			_ = docker.Service().Delete(cleanupCtx, vaultServiceID)
			_ = docker.Service().WaitRemoved(cleanupCtx, vaultServiceID)
		}
		_ = docker.Network().Delete(cleanupCtx, networkID)
		cleanupCancel()

		time.Sleep(time.Duration(attempt) * time.Second)
	}

	require.NoError(t, lastErr)
	return "", "", 0
}

func setupVaultStaticTokenAuth(
	t *testing.T,
	ctx context.Context,
	docker *dockertester.Tester,
	vault vaultclient.Client,
	_ url.URL,
) cloudSecretsVaultAuthConfig {
	t.Helper()

	docker.Secret().RequireNotExists(ctx, t, vaultAuthSecretName)

	runtimeToken, err := vault.CreateToken(ctx, []string{vaultPolicyName})
	require.NoError(t, err)

	authSecretID := createDockerSecret(t, ctx, docker, vaultAuthSecretName, runtimeToken)

	return cloudSecretsStaticTokenAuthConfig(authSecretID)
}

func setupVaultAppRoleAuth(
	t *testing.T,
	ctx context.Context,
	docker *dockertester.Tester,
	_ vaultclient.Client,
	vaultAddr url.URL,
) cloudSecretsVaultAuthConfig {
	t.Helper()

	docker.Secret().RequireNotExists(ctx, t, vaultAuthAppRoleRoleIDSecretName)
	docker.Secret().RequireNotExists(ctx, t, vaultAuthAppRoleSecretIDSecretName)

	roleID, secretID, err := createVaultAppRole(ctx, vaultAddr)
	require.NoError(t, err)

	_, err = vaultclient.NewHttpClient(ctx, vaultMountPath, vaultAddr, vaultclient.AuthConfig{
		AppRole: vaultclient.AppRoleConfig{
			RoleID:   roleID,
			SecretID: secretID,
		},
	})
	require.NoError(t, err)

	roleIDSecretID := createDockerSecret(t, ctx, docker, vaultAuthAppRoleRoleIDSecretName, roleID)
	secretIDSecretID := createDockerSecret(t, ctx, docker, vaultAuthAppRoleSecretIDSecretName, secretID)

	return cloudSecretsAppRoleAuthConfig(roleIDSecretID, secretIDSecretID)
}

func createDockerSecret(
	t *testing.T,
	ctx context.Context,
	docker *dockertester.Tester,
	name string,
	value string,
) string {
	t.Helper()

	secretID, err := docker.Secret().CreateSecret(ctx, name, []byte(value))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.Secret().DeleteWithRetry(cleanupCtx, name))
	})

	return secretID
}

func createVaultAppRole(ctx context.Context, vaultAddr url.URL) (string, string, error) {
	client, err := newVaultRootAPIClient(vaultAddr)
	if err != nil {
		return "", "", err
	}

	if err = client.Sys().EnableAuthWithOptionsWithContext(ctx, "approle", &vaultapi.EnableAuthOptions{
		Type: "approle",
	}); err != nil {
		return "", "", fmt.Errorf("enable Vault AppRole auth: %w", err)
	}

	if _, err = client.Logical().WriteWithContext(ctx, "auth/approle/role/"+vaultAppRoleName, map[string]interface{}{
		"token_policies":     []string{vaultPolicyName},
		"token_ttl":          "1h",
		"token_max_ttl":      "4h",
		"secret_id_ttl":      "0",
		"secret_id_num_uses": 0,
	}); err != nil {
		return "", "", fmt.Errorf("create Vault AppRole %q: %w", vaultAppRoleName, err)
	}

	roleIDSecret, err := client.Logical().ReadWithContext(ctx, "auth/approle/role/"+vaultAppRoleName+"/role-id")
	if err != nil {
		return "", "", fmt.Errorf("read Vault AppRole %q role_id: %w", vaultAppRoleName, err)
	}
	roleID, err := readVaultString(roleIDSecret, "role_id")
	if err != nil {
		return "", "", fmt.Errorf("read Vault AppRole %q role_id: %w", vaultAppRoleName, err)
	}

	secretIDSecret, err := client.Logical().WriteWithContext(
		ctx,
		"auth/approle/role/"+vaultAppRoleName+"/secret-id",
		map[string]interface{}{},
	)
	if err != nil {
		return "", "", fmt.Errorf("create Vault AppRole %q secret_id: %w", vaultAppRoleName, err)
	}
	secretID, err := readVaultString(secretIDSecret, "secret_id")
	if err != nil {
		return "", "", fmt.Errorf("create Vault AppRole %q secret_id: %w", vaultAppRoleName, err)
	}

	return roleID, secretID, nil
}

func newVaultRootAPIClient(addr url.URL) (*vaultapi.Client, error) {
	vaultCfg := vaultapi.DefaultConfig()
	vaultCfg.Address = addr.String()

	client, err := vaultapi.NewClient(vaultCfg)
	if err != nil {
		return nil, fmt.Errorf("create Vault root API client: %w", err)
	}
	client.SetToken(vaultRootToken)

	return client, nil
}

func readVaultString(secret *vaultapi.Secret, key string) (string, error) {
	if secret == nil {
		return "", fmt.Errorf("Vault returned empty secret")
	}

	raw, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("Vault secret does not contain %q", key)
	}

	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("Vault secret %q has unexpected type %T", key, raw)
	}
	if value == "" {
		return "", fmt.Errorf("Vault secret %q is empty", key)
	}

	return value, nil
}

func (e *vaultEnv) createVaultSecret(t *testing.T, value string) string {
	t.Helper()

	secret, err := e.vault.CreateSecret(e.ctx, vaultSecretPath, map[string]interface{}{
		vaultSecretKey: value,
	})
	require.NoError(t, err)
	require.NotNil(t, secret, "Vault secret for %s", externalSecretPath)
	require.NotEmpty(t, secret.VersionID, "Vault secret version for %s", externalSecretPath)

	return secret.VersionID
}

func dockerSecretLabelMatcher(labels map[string]string) bool {
	return labels["logical_path"] == dockerSecretName
}

func (e *vaultEnv) cleanupScenario(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, e.vault.DeleteSecret(ctx, vaultSecretPath))
	require.NoError(t, e.docker.Secret().DeleteByLabels(ctx, map[string]string{
		"logical_path": dockerSecretName,
	}))
}

func waitVaultReady(ctx context.Context, addr string) error {
	client := http.Client{
		Timeout: time.Second,
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			strings.TrimRight(addr, "/")+"/v1/sys/health?standbyok=true&perfstandbyok=true",
			nil,
		)
		if err != nil {
			return err
		}

		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
			lastErr = fmt.Errorf("Vault health status: %s", resp.Status)
		} else {
			lastErr = err
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait Vault ready: %w: %v", ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func vaultServiceSpec(name string, image string, networkID string, publishedPort uint32) swarm.ServiceSpec {
	return swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: name,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image: image,
				Command: []string{
					"vault",
				},
				Args: []string{
					"server",
					"-dev",
					"-dev-root-token-id=" + vaultRootToken,
					"-dev-listen-address=0.0.0.0:8200",
				},
				Env: []string{
					"VAULT_ADDR=http://127.0.0.1:8200",
					"VAULT_TOKEN=" + vaultRootToken,
				},
			},
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionAny,
			},
			Placement: managerPlacement(),
			Networks:  networkAttachment(networkID, "vault"),
		},
		Mode: oneReplica(),
		EndpointSpec: &swarm.EndpointSpec{
			Ports: []swarm.PortConfig{
				tcpPort(8200, publishedPort),
			},
		},
	}
}

func cloudSecretsStaticTokenAuthConfig(authSecretID string) cloudSecretsVaultAuthConfig {
	return cloudSecretsVaultAuthConfig{
		env: []string{
			"VAULT_AUTH_TOKEN=/run/secrets/" + vaultAuthSecretName,
		},
		secrets: []*swarm.SecretReference{
			dockertester.SecretRef(vaultAuthSecretName, vaultAuthSecretName, authSecretID),
		},
	}
}

func cloudSecretsAppRoleAuthConfig(roleIDSecretID string, secretIDSecretID string) cloudSecretsVaultAuthConfig {
	return cloudSecretsVaultAuthConfig{
		env: []string{
			"VAULT_AUTH_APPROLE_ROLE_ID=/run/secrets/" + vaultAuthAppRoleRoleIDSecretName,
			"VAULT_AUTH_APPROLE_SECRET_ID=/run/secrets/" + vaultAuthAppRoleSecretIDSecretName,
		},
		secrets: []*swarm.SecretReference{
			dockertester.SecretRef(
				vaultAuthAppRoleRoleIDSecretName,
				vaultAuthAppRoleRoleIDSecretName,
				roleIDSecretID,
			),
			dockertester.SecretRef(
				vaultAuthAppRoleSecretIDSecretName,
				vaultAuthAppRoleSecretIDSecretName,
				secretIDSecretID,
			),
		},
	}
}

func cloudSecretsServiceSpec(
	name string,
	image string,
	networkID string,
	auth cloudSecretsVaultAuthConfig,
) swarm.ServiceSpec {
	env := []string{
		"CS_PROVIDER=vault",
		"CS_REFRESH_INTERVAL=1s",
		"CS_LOG_LEVEL=debug",
		"VAULT_ADDR=http://vault:8200",
		"VAULT_MOUNT_PATH=" + vaultMountPath,
	}
	env = append(env, auth.env...)

	return swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: name,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image: image,
				Env:   env,
				Mounts: []mount.Mount{
					bindMount("/var/run/docker.sock", "/var/run/docker.sock", true),
				},
				Secrets: auth.secrets,
			},
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionAny,
			},
			Placement: managerPlacement(),
			Networks:  networkAttachment(networkID),
		},
		Mode: oneReplica(),
	}
}
