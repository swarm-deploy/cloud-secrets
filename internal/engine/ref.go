package engine

import "github.com/moby/moby/api/types/swarm"

const secretFileMode = 0444

func NewSecretRef(path, name, id string) *swarm.SecretReference {
	return &swarm.SecretReference{
		File: &swarm.SecretReferenceFileTarget{
			Name: path,
			UID:  "0",
			GID:  "0",
			Mode: secretFileMode,
		},
		SecretName: name,
		SecretID:   id,
	}
}
