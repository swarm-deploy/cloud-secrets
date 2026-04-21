package contracts

import "context"

type Provider interface {
	// GetSecretPayload retrieves latest secret payload by external path.
	GetSecretPayload(ctx context.Context, key string) ([]byte, error)
	// CreateSecret stores secret metadata with a dedicated payload.
	CreateSecret(ctx context.Context, secret Secret, payload []byte) error
	// ListSecrets lists secret metadata without loading payload.
	ListSecrets(ctx context.Context) (map[string]Secret, error)
}

type Secret struct {
	// VersionID is the latest external version identifier.
	VersionID string
	// Path is the full secret path in external storage.
	Path string
}
