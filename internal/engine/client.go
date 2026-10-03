//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=engine
package engine

import (
	"context"
	"time"

	"github.com/moby/moby/api/types/swarm"
)

type Client interface {
	// CreateSecretVersion creates a new version of an existing logical secret.
	CreateSecretVersion(
		ctx context.Context,
		secret ExistingSecret,
		version CreatingSecretVersion,
	) (CreatedSecretVersion, error)
	// UpdateService updates a Swarm service specification.
	UpdateService(ctx context.Context, service swarm.Service) error

	// RemoveSecret removes a Swarm secret by ID.
	// Throws SecretNotFoundError
	RemoveSecret(ctx context.Context, id string) error
	// CreateSecret creates a logical Swarm secret.
	CreateSecret(ctx context.Context, spec CreatingSecret) error
	// MapSecrets loads logical secrets and all their versions keyed by logical path.
	MapSecrets(ctx context.Context) (map[string]*ExistingSecret, error)
	// ListServices loads all Swarm services.
	ListServices(ctx context.Context) ([]swarm.Service, error)
}

type CreatingSecret struct {
	// Labels contains provider metadata; internal reconciliation labels take precedence.
	Labels map[string]string
	// Path is the logical Docker secret name.
	Path string
	// Value is the secret payload.
	Value []byte
	// Description is the external secret description.
	Description string
	// ExternalPath identifies the secret in its provider.
	ExternalPath string
	// ExternalVersionID identifies the synchronized provider version.
	ExternalVersionID string
}

type CreatingSecretVersion struct {
	// Labels contains provider metadata; internal reconciliation labels take precedence.
	Labels map[string]string
	// Path is the temporary versioned Docker secret name.
	Path string
	// Description is the external secret description.
	Description string
	// ExternalID identifies the synchronized provider version.
	ExternalID string
	// Value is the secret payload.
	Value []byte
}

type CreatedSecretVersion struct {
	ID   string
	Name string
}

type ExistingSecret struct {
	ID string

	Path         string
	ExternalPath string

	Managed bool

	Versions []ExistingSecretVersion

	latestVersion ExistingSecretVersion
}

type ExistingSecretVersion struct {
	ID string

	ExternalID string

	updatedAt time.Time
}
