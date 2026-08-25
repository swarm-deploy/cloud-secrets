package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/mount"
	dockernetwork "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	dock "github.com/moby/moby/client"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

const (
	waitServiceHealthyTimeout = 15 * time.Second

	vaultRootToken      = "test-token" //nolint:gosec // e2e Vault dev token.
	vaultPolicyName     = "cloud-secrets"
	vaultMountPath      = "secret"
	vaultAuthSecretName = "cs-vault-auth-token" //nolint:gosec // test secret name, not a credential.
	vaultImage          = "hashicorp/vault:1.18"
	cloudSecretsImage   = "swarmdeployorg/cloud-secrets:local"
	helperImage         = "alpine:3.20"
	// Vault KV data at "path/key" is normalized to the Docker secret name by the engine.
	externalSecretPath   = "orders/db-dsn"
	vaultSecretPath      = "orders"
	vaultSecretKey       = "db-dsn"
	dockerSecretName     = "orders-db-dsn"
	sharedSecretFilePath = "/shared/value"
)

var errDockerSecretNotFound = errors.New("docker secret not found")

type DockerClient struct {
	client dock.APIClient
}

func NewDockerClient() (*DockerClient, error) {
	client, err := dock.New(dock.FromEnv, dock.WithAPIVersionFromEnv())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}

	return &DockerClient{client: client}, nil
}

func (c *DockerClient) Close() error {
	return c.client.Close()
}

func (c *DockerClient) DeployService(ctx context.Context, spec swarm.ServiceSpec) (string, error) {
	created, err := c.client.ServiceCreate(ctx, dock.ServiceCreateOptions{Spec: spec})
	if err != nil {
		return "", fmt.Errorf("deploy service %q: %w", spec.Name, err)
	}

	return created.ID, nil
}

func (c *DockerClient) WaitServiceHealthy(ctx context.Context, serviceID string) error {
	ctx, cancel := context.WithTimeout(ctx, waitServiceHealthyTimeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastStatus string
	for {
		healthy, status, err := c.serviceHealthy(ctx, serviceID)
		if err != nil {
			return err
		}
		if healthy {
			return nil
		}
		lastStatus = status

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait service %q healthy: %w: %s", serviceID, ctx.Err(), lastStatus)
		case <-ticker.C:
		}
	}
}

func (c *DockerClient) DeleteService(ctx context.Context, serviceID string) error {
	_, err := c.client.ServiceRemove(ctx, serviceID, dock.ServiceRemoveOptions{})
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("delete service %q: %w", serviceID, err)
	}

	return nil
}

func (c *DockerClient) waitServiceRemoved(ctx context.Context, serviceID string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		_, err := c.client.ServiceInspect(ctx, serviceID, dock.ServiceInspectOptions{})
		if err != nil {
			if errdefs.IsNotFound(err) {
				return nil
			}

			return fmt.Errorf("inspect removed service %q: %w", serviceID, err)
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait service %q removed: %w", serviceID, ctx.Err())
		case <-ticker.C:
		}
	}
}

func (c *DockerClient) GetSecret(ctx context.Context, name string) (*swarm.Secret, error) {
	secrets, err := c.client.SecretList(ctx, dock.SecretListOptions{
		Filters: dock.Filters{}.Add("name", name),
	})
	if err != nil {
		return nil, fmt.Errorf("list secret %q: %w", name, err)
	}

	for _, secret := range secrets.Items {
		if secret.Spec.Name == name {
			return &secret, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errDockerSecretNotFound, name)
}

func (c *DockerClient) CreateSecret(ctx context.Context, path string, value []byte) (string, error) {
	created, err := c.client.SecretCreate(ctx, dock.SecretCreateOptions{
		Spec: swarm.SecretSpec{
			Annotations: swarm.Annotations{
				Name: path,
			},
			Data: value,
		},
	})
	if err != nil {
		return "", fmt.Errorf("create secret %q: %w", path, err)
	}

	return created.ID, nil
}

func (c *DockerClient) ListSecrets(ctx context.Context, logicalPath string) ([]swarm.Secret, error) {
	secrets, err := c.client.SecretList(ctx, dock.SecretListOptions{
		Filters: dock.Filters{}.Add("label", "logical_path="+logicalPath),
	})
	if err != nil {
		return nil, fmt.Errorf("list secrets by logical_path %q: %w", logicalPath, err)
	}

	out := make([]swarm.Secret, 0, len(secrets.Items))
	for _, secret := range secrets.Items {
		if secret.Spec.Labels["logical_path"] == logicalPath {
			out = append(out, secret)
		}
	}

	return out, nil
}

func (c *DockerClient) createNetwork(ctx context.Context, name string) (string, error) {
	created, err := c.client.NetworkCreate(ctx, name, dock.NetworkCreateOptions{
		Driver:     "overlay",
		Scope:      "swarm",
		Attachable: true,
	})
	if err != nil {
		return "", fmt.Errorf("create network %q: %w", name, err)
	}

	return created.ID, nil
}

func (c *DockerClient) deleteNetwork(ctx context.Context, networkID string) error {
	_, err := c.client.NetworkRemove(ctx, networkID, dock.NetworkRemoveOptions{})
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("delete network %q: %w", networkID, err)
	}

	return nil
}

func (c *DockerClient) createVolume(ctx context.Context, name string) error {
	_, err := c.client.VolumeCreate(ctx, dock.VolumeCreateOptions{
		Name:   name,
		Driver: "local",
	})
	if err != nil {
		return fmt.Errorf("create volume %q: %w", name, err)
	}

	return nil
}

func (c *DockerClient) deleteVolume(ctx context.Context, name string) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var lastErr error
	for {
		_, err := c.client.VolumeRemove(ctx, name, dock.VolumeRemoveOptions{Force: true})
		if err == nil || errdefs.IsNotFound(err) {
			return nil
		}
		if !errdefs.IsConflict(err) && !strings.Contains(err.Error(), "volume is in use") {
			return fmt.Errorf("delete volume %q: %w", name, err)
		}
		lastErr = err

		select {
		case <-ctx.Done():
			return fmt.Errorf("delete volume %q: %w: %v", name, ctx.Err(), lastErr)
		case <-ticker.C:
		}
	}
}

func (c *DockerClient) deleteSecret(ctx context.Context, name string) error {
	_, err := c.client.SecretRemove(ctx, name, dock.SecretRemoveOptions{})
	if err != nil && !errdefs.IsNotFound(err) {
		return fmt.Errorf("delete secret %q: %w", name, err)
	}

	return nil
}

func (c *DockerClient) serviceHealthy(ctx context.Context, serviceID string) (bool, string, error) {
	service, err := c.client.ServiceInspect(ctx, serviceID, dock.ServiceInspectOptions{})
	if err != nil {
		return false, "", fmt.Errorf("inspect service %q: %w", serviceID, err)
	}

	tasks, err := c.client.TaskList(ctx, dock.TaskListOptions{
		Filters: dock.Filters{}.Add("service", serviceID),
	})
	if err != nil {
		return false, "", fmt.Errorf("list tasks for service %q: %w", serviceID, err)
	}

	expectedState := swarm.TaskStateRunning
	if isCompletionService(service.Service.Spec) {
		expectedState = swarm.TaskStateComplete
	}

	var healthy int
	statuses := make([]string, 0, len(tasks.Items))
	for _, task := range tasks.Items {
		statuses = append(statuses, fmt.Sprintf("%s:%s:%s", task.ID, task.Status.State, task.Status.Err))

		if task.Status.State == swarm.TaskStateRejected || task.Status.State == swarm.TaskStateFailed {
			return false, strings.Join(statuses, ", "), fmt.Errorf(
				"service %q task %q failed: state=%s err=%s",
				service.Service.Spec.Name,
				task.ID,
				task.Status.State,
				task.Status.Err,
			)
		}
		if task.Status.State == expectedState {
			healthy++
		}
	}

	desired := desiredTasks(service.Service.Spec)
	if healthy >= desired {
		return true, strings.Join(statuses, ", "), nil
	}

	return false, strings.Join(statuses, ", "), nil
}

func desiredTasks(spec swarm.ServiceSpec) int {
	if spec.Mode.Replicated != nil && spec.Mode.Replicated.Replicas != nil {
		return int(*spec.Mode.Replicated.Replicas)
	}
	if spec.Mode.ReplicatedJob != nil {
		if spec.Mode.ReplicatedJob.TotalCompletions != nil {
			return int(*spec.Mode.ReplicatedJob.TotalCompletions)
		}
		if spec.Mode.ReplicatedJob.MaxConcurrent != nil {
			return int(*spec.Mode.ReplicatedJob.MaxConcurrent)
		}
	}

	return 1
}

func isCompletionService(spec swarm.ServiceSpec) bool {
	if spec.Mode.ReplicatedJob != nil || spec.Mode.GlobalJob != nil {
		return true
	}

	return spec.TaskTemplate.RestartPolicy != nil &&
		spec.TaskTemplate.RestartPolicy.Condition == swarm.RestartPolicyConditionNone
}

func oneReplica() swarm.ServiceMode {
	replicas := uint64(1)

	return swarm.ServiceMode{
		Replicated: &swarm.ReplicatedService{
			Replicas: &replicas,
		},
	}
}

func managerPlacement() *swarm.Placement {
	return &swarm.Placement{
		Constraints: []string{"node.role == manager"},
	}
}

func networkAttachment(networkID string, aliases ...string) []swarm.NetworkAttachmentConfig {
	return []swarm.NetworkAttachmentConfig{
		{
			Target:  networkID,
			Aliases: aliases,
		},
	}
}

func volumeMount(name, target string) mount.Mount {
	return mount.Mount{
		Type:   mount.TypeVolume,
		Source: name,
		Target: target,
	}
}

func bindMount(source, target string, readOnly bool) mount.Mount {
	return mount.Mount{
		Type:     mount.TypeBind,
		Source:   source,
		Target:   target,
		ReadOnly: readOnly,
	}
}

func tcpPort(target, published uint32) swarm.PortConfig {
	return swarm.PortConfig{
		Protocol:      dockernetwork.TCP,
		TargetPort:    target,
		PublishedPort: published,
		PublishMode:   swarm.PortConfigPublishModeIngress,
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

func cloudSecretsServiceSpec(name string, image string, networkID string, authSecretID string) swarm.ServiceSpec {
	return swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: name,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image: image,
				Env: []string{
					"CS_PROVIDER=vault",
					"CS_REFRESH_INTERVAL=1s",
					"CS_LOG_LEVEL=debug",
					"VAULT_ADDR=http://vault:8200",
					"VAULT_AUTH_TOKEN=/run/secrets/" + vaultAuthSecretName,
					"VAULT_MOUNT_PATH=" + vaultMountPath,
				},
				Mounts: []mount.Mount{
					bindMount("/var/run/docker.sock", "/var/run/docker.sock", true),
				},
				Secrets: []*swarm.SecretReference{
					engine.NewSecretRef(vaultAuthSecretName, vaultAuthSecretName, authSecretID),
				},
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

func ordersServiceSpec(
	name string,
	image string,
	networkID string,
	volumeName string,
	secret *swarm.Secret,
) swarm.ServiceSpec {
	return swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: name,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image: image,
				Command: []string{
					"sh",
					"-ec",
				},
				Args: []string{
					"while true; do cat /run/secrets/" + dockerSecretName + " > " + sharedSecretFilePath + "; sleep 1; done",
				},
				Mounts: []mount.Mount{
					volumeMount(volumeName, "/shared"),
				},
				Secrets: []*swarm.SecretReference{
					engine.NewSecretRef(dockerSecretName, secret.Spec.Name, secret.ID),
				},
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

func verifierServiceSpec(
	name string,
	image string,
	networkID string,
	volumeName string,
	expectedValue string,
) swarm.ServiceSpec {
	return swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: name,
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: &swarm.ContainerSpec{
				Image: image,
				Command: []string{
					"sh",
					"-ec",
				},
				Args: []string{
					`attempts=0
while [ "$attempts" -lt 15 ]; do
  actual="$(cat ` + sharedSecretFilePath + ` 2>/dev/null || true)"
  if [ "$actual" = "$EXPECTED_VALUE" ]; then
    exit 0
  fi
  attempts=$((attempts + 1))
  sleep 1
done
echo "expected $EXPECTED_VALUE, got $actual"
exit 1`,
				},
				Env: []string{
					"EXPECTED_VALUE=" + expectedValue,
				},
				Mounts: []mount.Mount{
					volumeMount(volumeName, "/shared"),
				},
			},
			RestartPolicy: &swarm.RestartPolicy{
				Condition: swarm.RestartPolicyConditionNone,
			},
			Placement: managerPlacement(),
			Networks:  networkAttachment(networkID),
		},
		Mode: oneReplica(),
	}
}
