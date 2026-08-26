package e2e

import (
	"github.com/moby/moby/api/types/mount"
	dockernetwork "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
)

const (
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
