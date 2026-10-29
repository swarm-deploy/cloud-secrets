//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=contracts
package contracts

import (
	"context"
	"errors"
)

var (
	// ErrInvalidCreateRequest indicates that a provider rejected create request fields.
	ErrInvalidCreateRequest = errors.New("invalid create secret request")
	// ErrSecretAlreadyExists indicates that create would replace an existing secret or key.
	ErrSecretAlreadyExists = errors.New("secret already exists")
	// ErrCreateConflict indicates that concurrent provider changes prevented a safe create.
	ErrCreateConflict = errors.New("concurrent secret creation conflict")
)

type Provider interface {
	// Definition returns human-readable provider definition.
	Definition() ProviderDefinition
	// CreateSecret creates a new secret in the configured synchronization scope.
	CreateSecret(ctx context.Context, req CreateSecretRequest) (string, error)
	// GetSecretPayload retrieves latest secret payload by provider path returned from ListSecrets.
	GetSecretPayload(ctx context.Context, path string) ([]byte, error)
	// ListSecrets lists secret metadata without loading payload.
	ListSecrets(ctx context.Context) (map[string]Secret, error)
}

type CreateSecretRequest struct {
	// Path is the provider-relative path within the synchronization scope.
	Path string
	// Value is the secret payload.
	Value []byte
	// Key is the Vault KV document key and must be empty for Cloud.ru.
	Key string
	// Description is the Cloud.ru secret description and must be empty for Vault.
	Description string
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
