package sync

import (
	"strings"

	"github.com/moby/moby/api/types/swarm"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
)

type syncPayload struct {
	result Result

	services        []swarm.Service
	swarmSecretsMap map[string]*engine.ExistingSecret
	externalSecrets map[string]contracts.Secret

	pendingServiceUpdates     map[string]*ServiceTask
	pendingServiceUpdateOrder []*ServiceTask
	pendingVersionRemovals    []SecretVersionRemoval
	pendingSecretRestores     []UpdatedSecret
	pendingServiceOffset      int
}

type ServiceTask struct {
	Service swarm.Service
	Secrets map[string]updatingServiceSecret
}

type UpdatedSecret struct {
	Path         string
	Value        []byte
	ExternalPath string

	ExternalID string
}

type SecretVersionRemoval struct {
	Secret       *engine.ExistingSecret
	RemoveParent bool
}

type updatingServiceSecret struct {
	Name string
	ID   string
}

func (p *syncPayload) hasPendingChanges() bool {
	return p.hasPendingServiceUpdates() || p.hasPendingVersionRemovals() || p.hasPendingSecretRestores()
}

func (p *syncPayload) hasPendingServiceUpdates() bool {
	return len(p.pendingServiceUpdates) > 0
}

func (p *syncPayload) hasPendingVersionRemovals() bool {
	return len(p.pendingVersionRemovals) > 0
}

func (p *syncPayload) hasPendingSecretRestores() bool {
	return len(p.pendingSecretRestores) > 0
}

func (p *syncPayload) orphanedManagedSecretRemovals(
	folderDelimiter secretname.FolderDelimiter,
) []SecretVersionRemoval {
	usedSecretPaths := p.usedSecretPaths()
	existingExternalSecretPaths := p.externalSecretPaths(folderDelimiter)
	removals := make([]SecretVersionRemoval, 0)

	for path, secret := range p.swarmSecretsMap {
		if !secret.Managed {
			continue
		}

		if _, ok := usedSecretPaths[path]; ok {
			continue
		}

		if _, ok := existingExternalSecretPaths[path]; ok {
			continue
		}

		removals = append(removals, SecretVersionRemoval{
			Secret:       secret,
			RemoveParent: true,
		})
	}

	return removals
}

func (p *syncPayload) usedSecretPaths() map[string]struct{} {
	pathBySecretID := p.secretPathByID()
	usedPaths := make(map[string]struct{})

	for _, service := range p.services {
		containerSpec := service.Spec.TaskTemplate.ContainerSpec
		if containerSpec == nil {
			continue
		}

		for _, secretRef := range containerSpec.Secrets {
			path, ok := pathBySecretID[secretRef.SecretID]
			if !ok {
				continue
			}

			usedPaths[path] = struct{}{}
		}
	}

	return usedPaths
}

func (p *syncPayload) secretPathByID() map[string]string {
	pathBySecretID := make(map[string]string)

	for path, secret := range p.swarmSecretsMap {
		for _, version := range secret.Versions {
			pathBySecretID[version.ID] = path
		}
	}

	return pathBySecretID
}

func (p *syncPayload) externalSecretPaths(
	folderDelimiter secretname.FolderDelimiter,
) map[string]struct{} {
	paths := make(map[string]struct{})

	for _, secret := range p.externalSecrets {
		paths[strings.ReplaceAll(secret.Path, "/", string(folderDelimiter))] = struct{}{}
	}

	return paths
}
