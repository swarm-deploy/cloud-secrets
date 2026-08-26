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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	f := setupVaultEnv(t, ctx)

	t.Run("create secret and sync to Docker", func(t *testing.T) {
		t.Cleanup(func() {
			f.cleanupScenario(t)
		})

		versionID := f.createVaultSecret(t, "test-value")
		syncedSecret := f.docker.Secret().Wait(f.ctx, t, versionID, dockerSecretLabelMatcher)

		f.assertDockerSecretValue(t, syncedSecret, "test-value")
	})

	t.Run("update secret and sync new Docker version", func(t *testing.T) {
		t.Cleanup(func() {
			f.cleanupScenario(t)
		})

		initialVersionID := f.createVaultSecret(t, "test-value")
		syncedSecret := f.docker.Secret().Wait(f.ctx, t, initialVersionID, dockerSecretLabelMatcher)

		f.assertDockerSecretValue(t, syncedSecret, "test-value")

		updatedVersionID := f.createVaultSecret(t, "new-value")
		updatedSecret := f.docker.Secret().Wait(f.ctx, t, updatedVersionID, dockerSecretLabelMatcher)
		f.assertDockerSecretValue(t, updatedSecret, "new-value")
	})
}

type vaultEnv struct {
	ctx       context.Context
	docker    *dockertester.Tester
	vault     vaultclient.Client
	networkID string
	runID     string
}

func setupVaultEnv(t *testing.T, ctx context.Context) *vaultEnv {
	t.Helper()

	docker, err := dockertester.NewTester()
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, docker.Close())
	})

	docker.Swarm().RequireSwarmMode(ctx, t)

	docker.Secret().RequireNotExists(ctx, t, vaultAuthSecretName)
	docker.Secret().RequireNotExists(ctx, t, dockerSecretName)

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
		require.NoError(t, docker.Secret().DeleteWithRetry(cleanupCtx, vaultAuthSecretName))
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

	return &vaultEnv{
		ctx:       ctx,
		docker:    docker,
		vault:     vault,
		networkID: networkID,
		runID:     runID,
	}
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

func (e *vaultEnv) assertDockerSecretValue(t *testing.T, secret *swarm.Secret, expectedValue string) {
	t.Helper()

	e.docker.Secret().AssertSecretValue(e.ctx, t, secret, expectedValue)
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
