package contracts

import "context"

type Provider interface {
	GetSecret(ctx context.Context, key string) (Secret, error)
	CreateSecret(ctx context.Context, secret Secret) error
	ListSecrets(ctx context.Context) (map[string]Secret, error)
}

type Secret struct {
	VersionID string
	Path      string
	Value     []byte
}
