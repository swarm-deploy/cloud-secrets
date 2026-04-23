package contracts

import "context"

type Provider interface {
	// Definition returns human-readable provider definition.
	Definition() ProviderDefinition
	// GetSecretPayload retrieves latest secret payload by external path.
	GetSecretPayload(ctx context.Context, key string) ([]byte, error)
	// ListSecrets lists secret metadata without loading payload.
	ListSecrets(ctx context.Context) (map[string]Secret, error)
}

type Secret struct {
	// VersionID is the latest external version identifier.
	VersionID string
	// Path is the full secret path in external storage.
	Path string
}

type ProviderDefinition struct {
	Name string
	URL  string
}
