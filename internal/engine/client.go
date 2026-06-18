//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=engine
package engine

import (
	"context"
	"time"

	"github.com/moby/moby/api/types/swarm"
)

type Client interface {
	CreateSecretVersion(
		ctx context.Context,
		secret ExistingSecret,
		version CreatingSecretVersion,
	) (CreatedSecretVersion, error)
	UpdateService(ctx context.Context, service swarm.Service) error

	RemoveSecret(ctx context.Context, id string) error
	CreateSecret(ctx context.Context, spec CreatingSecret) error
	MapSecrets(ctx context.Context) (map[string]*ExistingSecret, error)
	MapServicesBySecrets(ctx context.Context) (map[string][]swarm.Service, error)
}

type CreatingSecret struct {
	Path  string
	Value []byte

	ExternalPath      string
	ExternalVersionID string
}

type CreatingSecretVersion struct {
	Path string

	ExternalID string

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

	Versions []ExistingSecretVersion

	latestVersion ExistingSecretVersion
}

type ExistingSecretVersion struct {
	ID string

	ExternalID string

	updatedAt time.Time
}
