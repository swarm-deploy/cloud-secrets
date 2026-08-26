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

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/require"
	vaultclient "github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
	"github.com/swarm-deploy/dockertester"
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
	docker    *dockertester.Tester
	vault     vaultclient.Client
	networkID string
	runID     string
}

func setupVaultFixture(t *testing.T, ctx context.Context) *vaultFixture {
	t.Helper()

	docker, err := dockertester.NewTester()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, docker.Close())
	})

	docker.Swarm().RequireSwarmMode(ctx, t)

	requireNoPreexistingSecret(t, ctx, docker, vaultAuthSecretName)
	requireNoPreexistingSecret(t, ctx, docker, dockerSecretName)

	runID := "cs-e2e-" + strconv.FormatInt(time.Now().UnixNano(), 36)

	networkID, err := docker.Network().CreateSwarmOverlay(ctx, runID+"-net")
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.Network().Delete(cleanupCtx, networkID))
	})

	vaultPort, err := dockertester.FreeTCPPort()
	require.NoError(t, err)

	vaultServiceID, err := docker.Service().Deploy(ctx, vaultServiceSpec(runID+"-vault", vaultImage, networkID, vaultPort))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.Service().Delete(cleanupCtx, vaultServiceID))
		require.NoError(t, docker.Service().WaitRemoved(cleanupCtx, vaultServiceID))
	})
	require.NoError(t, docker.Service().WaitHealthy(ctx, vaultServiceID))

	vaultAddr := fmt.Sprintf("http://127.0.0.1:%d", vaultPort)
	require.NoError(t, waitVaultReady(ctx, vaultAddr))

	parsedVaultAddr, err := url.Parse(vaultAddr)
	require.NoError(t, err)

	vault, err := vaultclient.NewHttpClient(ctx, vaultMountPath, *parsedVaultAddr, vaultclient.AuthConfig{
		Token: vaultRootToken,
	})
	require.NoError(t, err)

	require.NoError(t, vault.CreateACLPolicy(ctx, vaultPolicyName))

	runtimeToken, err := vault.CreateToken(ctx, []string{vaultPolicyName})
	require.NoError(t, err)

	authSecretID, err := docker.Secret().CreateSecret(ctx, vaultAuthSecretName, []byte(runtimeToken))
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, deleteSecretWithRetry(cleanupCtx, docker, vaultAuthSecretName))
	})

	time.Sleep(500 * time.Millisecond)

	cloudSecretsServiceID, err := docker.Service().Deploy(
		ctx,
		cloudSecretsServiceSpec(runID+"-cloud-secrets", cloudSecretsImage, networkID, authSecretID),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, docker.Service().Delete(cleanupCtx, cloudSecretsServiceID))
		require.NoError(t, docker.Service().WaitRemoved(cleanupCtx, cloudSecretsServiceID))
	})
	require.NoError(t, docker.Service().WaitHealthy(ctx, cloudSecretsServiceID))

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
	require.NoError(t, f.docker.Volume().CreateLocal(f.ctx, volumeName))

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
	require.NoError(t, f.docker.Volume().CreateLocal(f.ctx, volumeName))

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

	serviceID, err := f.docker.Service().Deploy(
		f.ctx,
		ordersServiceSpec(name, helperImage, f.networkID, volumeName, secret),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, f.docker.Service().Delete(cleanupCtx, serviceID))
		require.NoError(t, f.docker.Service().WaitRemoved(cleanupCtx, serviceID))
	})

	require.NoError(t, f.docker.Service().WaitHealthy(f.ctx, serviceID))
}

func (f *vaultFixture) verifySharedVolumeValue(
	t *testing.T,
	name string,
	volumeName string,
	expectedValue string,
) {
	t.Helper()

	serviceID, err := f.docker.Service().Deploy(
		f.ctx,
		verifierServiceSpec(name, helperImage, f.networkID, volumeName, expectedValue),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		require.NoError(t, f.docker.Service().Delete(cleanupCtx, serviceID))
	})
	require.NoError(t, f.docker.Service().WaitHealthy(f.ctx, serviceID))
	require.NoError(t, f.docker.Service().Delete(f.ctx, serviceID))
	require.NoError(t, f.docker.Service().WaitRemoved(f.ctx, serviceID))
}

func (f *vaultFixture) waitPrimaryDockerSecret(t *testing.T, versionID string) *swarm.Secret {
	t.Helper()

	waitCtx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		secret, err := f.docker.Secret().GetSecret(waitCtx, dockerSecretName)
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
		secrets, err := f.docker.Secret().ListSecrets(waitCtx, dockerSecretName)
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
	require.NoError(t, f.docker.Volume().Delete(ctx, volumeName))
}

func deleteSyncedSecrets(ctx context.Context, docker *dockertester.Tester, logicalPath string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		secrets, err := docker.Secret().ListSecrets(ctx, logicalPath)
		if err != nil {
			return err
		}
		if len(secrets) == 0 {
			return nil
		}

		var deleteErr error
		for _, secret := range secrets {
			if err = docker.Secret().Delete(ctx, secret.ID); err != nil {
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

func deleteSecretWithRetry(ctx context.Context, docker *dockertester.Tester, name string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		err := docker.Secret().Delete(ctx, name)
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

func requireNoPreexistingSecret(t *testing.T, ctx context.Context, docker *dockertester.Tester, name string) {
	t.Helper()

	secret, err := docker.Secret().GetSecret(ctx, name)
	if err != nil {
		require.ErrorIs(t, err, dockertester.ErrSecretNotFound)

		return
	}

	t.Fatalf("Docker secret %q already exists with id %q; remove it before running e2e tests", name, secret.ID)
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
