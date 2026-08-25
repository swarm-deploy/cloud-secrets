package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/moby/moby/api/types/swarm"
	dock "github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	vaultclient "github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
)

func TestVault(t *testing.T) {
	if os.Getenv("CS_E2E") != "1" {
		t.Skip("set CS_E2E=1 to run Docker Swarm e2e tests")
	}

	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fixture := setupVaultFixture(t, ctx)

	t.Run("create secret and sync to Docker", fixture.runCreateScenario)
	t.Run("update secret and sync new Docker version", fixture.runUpdateScenario)
}

type vaultFixture struct {
	ctx       context.Context
	docker    *DockerClient
	vault     vaultclient.Client
	networkID string
	runID     string
}

func setupVaultFixture(t *testing.T, ctx context.Context) *vaultFixture {
	t.Helper()

	docker, err := NewDockerClient()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, docker.Close())
	})

	_, err = docker.client.SwarmInspect(ctx, dock.SwarmInspectOptions{})
	require.NoError(t, err, "Docker Swarm must be initialized before running Vault e2e tests")

	requireNoPreexistingSecret(t, ctx, docker, vaultAuthSecretName)
	requireNoPreexistingSecret(t, ctx, docker, dockerSecretName)

	runID := "cs-e2e-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	networkID, err := docker.createNetwork(ctx, runID+"-net")
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.deleteNetwork(cleanupCtx, networkID))
	})

	vaultPort, err := freeTCPPort()
	require.NoError(t, err)

	vaultServiceID, err := docker.DeployService(ctx, vaultServiceSpec(runID+"-vault", vaultImage, networkID, vaultPort))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.DeleteService(cleanupCtx, vaultServiceID))
		require.NoError(t, docker.waitServiceRemoved(cleanupCtx, vaultServiceID))
	})
	require.NoError(t, docker.WaitServiceHealthy(ctx, vaultServiceID))

	vaultAddr := fmt.Sprintf("http://127.0.0.1:%d", vaultPort)
	require.NoError(t, waitVaultReady(ctx, vaultAddr))

	parsedVaultAddr, err := url.Parse(vaultAddr)
	require.NoError(t, err)

	vault, err := vaultclient.NewHttpClient(ctx, vaultMountPath, *parsedVaultAddr, vaultclient.AuthConfig{
		Token: vaultRootToken,
	})
	require.NoError(t, err)

	require.NoError(t, vault.CreateACLPolicy(ctx, vaultPolicyName))

	runtimeToken, err := createVaultToken(ctx, vaultAddr, []string{vaultPolicyName})
	require.NoError(t, err)

	authSecretID, err := docker.CreateSecret(ctx, vaultAuthSecretName, []byte(runtimeToken))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, deleteSecretWithRetry(cleanupCtx, docker, vaultAuthSecretName))
	})

	time.Sleep(500 * time.Millisecond)

	cloudSecretsServiceID, err := docker.DeployService(
		ctx,
		cloudSecretsServiceSpec(runID+"-cloud-secrets", cloudSecretsImage, networkID, authSecretID),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.DeleteService(cleanupCtx, cloudSecretsServiceID))
		require.NoError(t, docker.waitServiceRemoved(cleanupCtx, cloudSecretsServiceID))
	})
	require.NoError(t, docker.WaitServiceHealthy(ctx, cloudSecretsServiceID))

	return &vaultFixture{
		ctx:       ctx,
		docker:    docker,
		vault:     vault,
		networkID: networkID,
		runID:     runID,
	}
}

func (f *vaultFixture) runCreateScenario(t *testing.T) {
	t.Helper()

	volumeName := f.runID + "-create-orders"
	require.NoError(t, f.docker.createVolume(f.ctx, volumeName))

	t.Cleanup(func() {
		f.cleanupScenario(t, volumeName)
	})

	versionID := f.createVaultSecret(t, "test-value")
	syncedSecret := f.waitPrimaryDockerSecret(t, versionID)

	f.deployOrdersService(t, f.runID+"-orders-create", volumeName, syncedSecret)
	f.verifySharedVolumeValue(t, f.runID+"-verify-create", volumeName, "test-value")
}

func (f *vaultFixture) runUpdateScenario(t *testing.T) {
	t.Helper()

	volumeName := f.runID + "-update-orders"
	require.NoError(t, f.docker.createVolume(f.ctx, volumeName))

	t.Cleanup(func() {
		f.cleanupScenario(t, volumeName)
	})

	initialVersionID := f.createVaultSecret(t, "test-value")
	syncedSecret := f.waitPrimaryDockerSecret(t, initialVersionID)

	f.deployOrdersService(t, f.runID+"-orders-update", volumeName, syncedSecret)
	f.verifySharedVolumeValue(t, f.runID+"-verify-initial", volumeName, "test-value")

	updatedVersionID := f.createVaultSecret(t, "new-value")
	f.waitDockerSecretVersion(t, updatedVersionID)
	f.verifySharedVolumeValue(t, f.runID+"-verify-updated", volumeName, "new-value")
}

func (f *vaultFixture) createVaultSecret(t *testing.T, value string) string {
	t.Helper()

	secret, err := f.vault.CreateSecret(f.ctx, vaultSecretPath, map[string]interface{}{
		vaultSecretKey: value,
	})
	require.NoError(t, err)
	require.NotNil(t, secret, "Vault secret for %s", externalSecretPath)
	require.NotEmpty(t, secret.VersionID, "Vault secret version for %s", externalSecretPath)

	return secret.VersionID
}

func (f *vaultFixture) deployOrdersService(
	t *testing.T,
	name string,
	volumeName string,
	secret *swarm.Secret,
) {
	t.Helper()

	serviceID, err := f.docker.DeployService(
		f.ctx,
		ordersServiceSpec(name, helperImage, f.networkID, volumeName, secret),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, f.docker.DeleteService(cleanupCtx, serviceID))
		require.NoError(t, f.docker.waitServiceRemoved(cleanupCtx, serviceID))
	})

	require.NoError(t, f.docker.WaitServiceHealthy(f.ctx, serviceID))
}

func (f *vaultFixture) verifySharedVolumeValue(
	t *testing.T,
	name string,
	volumeName string,
	expectedValue string,
) {
	t.Helper()

	serviceID, err := f.docker.DeployService(
		f.ctx,
		verifierServiceSpec(name, helperImage, f.networkID, volumeName, expectedValue),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, f.docker.DeleteService(cleanupCtx, serviceID))
	})
	require.NoError(t, f.docker.WaitServiceHealthy(f.ctx, serviceID))
	require.NoError(t, f.docker.DeleteService(f.ctx, serviceID))
	require.NoError(t, f.docker.waitServiceRemoved(f.ctx, serviceID))
}

func (f *vaultFixture) waitPrimaryDockerSecret(t *testing.T, versionID string) *swarm.Secret {
	t.Helper()

	waitCtx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		secret, err := f.docker.GetSecret(waitCtx, dockerSecretName)
		if err == nil && secret.Spec.Labels["external_version_id"] == versionID {
			return secret
		}
		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("secret %q has external_version_id=%q, want %q",
				dockerSecretName,
				secret.Spec.Labels["external_version_id"],
				versionID,
			)
		}

		select {
		case <-waitCtx.Done():
			require.NoError(t, lastErr)
			require.NoError(t, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (f *vaultFixture) waitDockerSecretVersion(t *testing.T, versionID string) {
	t.Helper()

	waitCtx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		secrets, err := f.docker.ListSecrets(waitCtx, dockerSecretName)
		if err == nil {
			for _, secret := range secrets {
				if secret.Spec.Labels["external_version_id"] == versionID {
					return
				}
			}

			lastErr = fmt.Errorf(
				"no secret with logical_path=%q and external_version_id=%q",
				dockerSecretName,
				versionID,
			)
		} else {
			lastErr = err
		}

		select {
		case <-waitCtx.Done():
			require.NoError(t, lastErr)
			require.NoError(t, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func (f *vaultFixture) cleanupScenario(t *testing.T, volumeName string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	require.NoError(t, f.vault.DeleteSecret(ctx, vaultSecretPath))
	require.NoError(t, deleteSyncedSecrets(ctx, f.docker, dockerSecretName))
	require.NoError(t, f.docker.deleteVolume(ctx, volumeName))
}

func deleteSyncedSecrets(ctx context.Context, docker *DockerClient, logicalPath string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		secrets, err := docker.ListSecrets(ctx, logicalPath)
		if err != nil {
			return err
		}
		if len(secrets) == 0 {
			return nil
		}

		var deleteErr error
		for _, secret := range secrets {
			if err = docker.deleteSecret(ctx, secret.ID); err != nil {
				deleteErr = err
			}
		}
		if deleteErr == nil {
			continue
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("delete synced secrets for %q: %w: %v", logicalPath, ctx.Err(), deleteErr)
		case <-ticker.C:
		}
	}
}

func deleteSecretWithRetry(ctx context.Context, docker *DockerClient, name string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		err := docker.deleteSecret(ctx, name)
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("delete secret %q: %w: %v", name, ctx.Err(), err)
		case <-ticker.C:
		}
	}
}

func requireNoPreexistingSecret(t *testing.T, ctx context.Context, docker *DockerClient, name string) {
	t.Helper()

	secret, err := docker.GetSecret(ctx, name)
	if err != nil {
		require.ErrorIs(t, err, errDockerSecretNotFound)

		return
	}

	t.Fatalf("Docker secret %q already exists with id %q; remove it before running e2e tests", name, secret.ID)
}

func createVaultToken(ctx context.Context, addr string, policies []string) (string, error) {
	cfg := vaultapi.DefaultConfig()
	cfg.Address = addr

	client, err := vaultapi.NewClient(cfg)
	if err != nil {
		return "", fmt.Errorf("create Vault token client: %w", err)
	}
	client.SetToken(vaultRootToken)

	secret, err := client.Auth().Token().CreateWithContext(ctx, &vaultapi.TokenCreateRequest{
		Policies:        policies,
		DisplayName:     "cloud-secrets-e2e",
		TTL:             "1h",
		NoDefaultPolicy: true,
	})
	if err != nil {
		return "", fmt.Errorf("create Vault token: %w", err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return "", fmt.Errorf("create Vault token: Vault returned empty token")
	}

	return secret.Auth.ClientToken, nil
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

func freeTCPPort() (uint32, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate tcp port: %w", err)
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("unexpected listener address %T", listener.Addr())
	}

	return uint32(addr.Port), nil
}
