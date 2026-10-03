//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=contracts
package contracts

import "context"

type Provider interface {
	// Definition returns human-readable provider definition.
	Definition() ProviderDefinition
	// GetSecretPayload retrieves latest secret payload by provider path returned from ListSecrets.
	GetSecretPayload(ctx context.Context, path string) ([]byte, error)
	// ListSecrets lists secret metadata, validating grouped payloads when required by the provider.
	ListSecrets(ctx context.Context) (map[string]Secret, error)
}

type Secret struct {
	// VersionID is the latest external version identifier.
	VersionID string
	// Path is the provider path within the synchronization scope.
	Path string
	// FullPath is the full secret path in external storage.
	FullPath string
	// Description is the external secret description.
	Description string
	// Labels contains provider metadata to attach to the Docker secret.
	Labels map[string]string
}

type ProviderDefinition struct {
	// Name is the human-readable provider name.
	Name string
	// Links contains provider navigation links.
	Links Links
}

type Links struct {
	// Doc points to provider documentation.
	Doc string
	// Manager points to the configured provider management UI.
	Manager string
}
