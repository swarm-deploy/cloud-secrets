package e2e

import (
	"github.com/moby/moby/api/types/mount"
	dockernetwork "github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/swarm"
)

const (
	vaultRootToken                     = "test-token" //nolint:gosec // e2e Vault dev token.
	vaultPolicyName                    = "cloud-secrets"
	vaultAppRoleName                   = "cloud-secrets"
	vaultMountPath                     = "secret"
	vaultAuthSecretName                = "cs-vault-auth-token" //nolint:gosec // test secret name, not a credential.
	vaultAuthAppRoleRoleIDSecretName   = "cs-vault-auth-approle-role-id"
	vaultAuthAppRoleSecretIDSecretName = "cs-vault-auth-approle-secret-id" //nolint:gosec // test secret name, not a credential.
	vaultImage                         = "hashicorp/vault:1.18"
	cloudSecretsImage                  = "swarmdeployorg/cloud-secrets:local"
	// Vault KV data at "path/key" is normalized to the Docker secret name by the engine.
	externalSecretPath = "orders/db-dsn"
	vaultSecretPath    = "orders"
	vaultSecretKey     = "db-dsn"
	dockerSecretName   = "orders-db-dsn"
)

type cloudSecretsVaultAuthConfig struct {
	env     []string
	secrets []*swarm.SecretReference
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
