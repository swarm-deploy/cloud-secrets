//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=contracts
package contracts

import "context"

type Provider interface {
	// Definition returns human-readable provider definition.
	Definition() ProviderDefinition
	// GetSecretPayload retrieves latest secret payload by provider path returned from ListSecrets.
	GetSecretPayload(ctx context.Context, key string) ([]byte, error)
	// ListSecrets lists secret metadata without loading payload.
	ListSecrets(ctx context.Context) (map[string]Secret, error)
}

type Secret struct {
	// VersionID is the latest external version identifier.
	VersionID string
	// Path is the provider path within the synchronization scope.
	Path string
	// FullPath is the full secret path in external storage.
	FullPath string
}

type ProviderDefinition struct {
	Name string
	URL  string
}
