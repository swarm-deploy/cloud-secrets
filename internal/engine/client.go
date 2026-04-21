package engine

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/moby/moby/api/types/swarm"
	dock "github.com/moby/moby/client"
)

type Client struct {
	client dock.APIClient
}

func NewClient(client dock.APIClient) *Client {
	return &Client{client: client}
}

type CreatingSecret struct {
	Path  string
	Value []byte

	ExternalPath      string
	ExternalVersionID string
}

type CreatingSecretVersion struct {
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

func (c *Client) RemoveSecret(ctx context.Context, id string) error {
	_, err := c.client.SecretRemove(ctx, id, dock.SecretRemoveOptions{})
	if err != nil {
		return fmt.Errorf("remove secret in swarm: %w", err)
	}

	return nil
}

func (c *Client) CreateSecretVersion(
	ctx context.Context,
	secret ExistingSecret,
	version CreatingSecretVersion,
) (CreatedSecretVersion, error) {
	name := uuid.NewString()

	resp, err := c.client.SecretCreate(ctx, dock.SecretCreateOptions{
		Spec: swarm.SecretSpec{
			Annotations: swarm.Annotations{
				Name: name,
				Labels: map[string]string{
					"logical_path":        secret.Path,
					"external_path":       secret.ExternalPath,
					"external_version_id": version.ExternalID,
				},
			},
			Data: version.Value,
		},
	})
	if err != nil {
		return CreatedSecretVersion{}, fmt.Errorf("create secret: %w", err)
	}

	return CreatedSecretVersion{
		ID:   resp.ID,
		Name: name,
	}, err
}

func (c *Client) CreateSecret(ctx context.Context, spec CreatingSecret) error {
	_, err := c.client.SecretCreate(ctx, dock.SecretCreateOptions{
		Spec: swarm.SecretSpec{
			Annotations: swarm.Annotations{
				Name: spec.Path,
				Labels: map[string]string{
					"logical_path":        spec.Path,
					"external_path":       spec.Path,
					"external_version_id": spec.ExternalVersionID,
				},
			},
			Data: spec.Value,
		},
	})
	return err
}

func (c *Client) MapSecrets(ctx context.Context) (map[string]*ExistingSecret, error) {
	secrets, err := c.client.SecretList(ctx, dock.SecretListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list swarm secrets: %w", err)
	}

	slog.DebugContext(ctx, "[engine] fetched secrets", slog.Int("secrets.count", len(secrets.Items)))

	secretsMap := map[string]*ExistingSecret{}

	// collect secrets
	for _, secret := range secrets.Items {
		path := c.getLabel(secret.Spec.Labels, "logical_path")
		if path != secret.Spec.Name {
			continue
		}

		secretsMap[path] = &ExistingSecret{
			ID:           secret.ID,
			Path:         path,
			ExternalPath: c.getLabel(secret.Spec.Labels, "external_path"),
			Versions: []ExistingSecretVersion{
				ExistingSecretVersion{
					ID:         secret.ID,
					ExternalID: c.getLabel(secret.Spec.Labels, "external_version_id"),
					updatedAt:  secret.UpdatedAt,
				},
			},
			latestVersion: ExistingSecretVersion{
				ID:         secret.ID,
				ExternalID: c.getLabel(secret.Spec.Labels, "external_version_id"),
				updatedAt:  secret.UpdatedAt,
			},
		}
	}

	// collect versions
	for _, secret := range secrets.Items {
		path := c.getLabel(secret.Spec.Labels, "logical_path")
		if path == secret.Spec.Name {
			continue
		}

		parent, ok := secretsMap[path]
		if !ok {
			slog.WarnContext(ctx, "[engine] no found secret for version", slog.String("path", path))
			continue
		}

		version := ExistingSecretVersion{
			ID:         secret.ID,
			ExternalID: c.getLabel(secret.Spec.Labels, "external_version_id"),
			updatedAt:  secret.UpdatedAt,
		}

		parent.Versions = append(parent.Versions, version)

		if secret.UpdatedAt.After(parent.latestVersion.updatedAt) {
			parent.latestVersion = version
		}
	}

	return secretsMap, nil
}

func (c *Client) getLabel(labels map[string]string, key string) string {
	if label, ok := labels[key]; ok {
		return label
	}
	return ""
}

func (v *ExistingSecret) LatestVersion() ExistingSecretVersion {
	return v.latestVersion
}
