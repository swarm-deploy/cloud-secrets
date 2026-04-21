package cloudru

import (
	"context"
	"fmt"
	"strconv"

	v1 "github.com/cloudru-tech/secret-manager-sdk/api/v1"
	v2 "github.com/cloudru-tech/secret-manager-sdk/api/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
)

func (p *Provider) GetSecret(ctx context.Context, key string) (contracts.Secret, error) {
	secret, err := p.secretManager.V2.SecretService.Access(ctx, &v2.AccessSecretRequest{
		Path:      key,
		ProjectId: p.cfg.ProjectID,
	})
	if err != nil {
		return contracts.Secret{}, err
	}

	return contracts.Secret{
		Path:  key,
		Value: secret.Payload.GetValue(),
	}, nil
}

func (p *Provider) CreateSecret(ctx context.Context, secret contracts.Secret) error {
	_, err := p.secretManager.V2.SecretService.Create(ctx, &v2.CreateSecretRequest{
		Path:      secret.Path,
		Payload:   wrapperspb.Bytes(secret.Value),
		ProjectId: p.cfg.ProjectID,
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *Provider) ListSecrets(ctx context.Context) (map[string]contracts.Secret, error) {
	secretsResp, err := p.secretManager.V2.SecretService.Search(ctx, &v2.SearchSecretRequest{
		ProjectId: p.cfg.ProjectID,
		Depth:     -1,
	})
	if err != nil {
		return nil, err
	}

	secretsMap := map[string]contracts.Secret{}

	for _, secret := range secretsResp.Secrets {
		secretValue, aerr := p.secretManager.SecretService.AccessSecretVersion(ctx, &v1.AccessSecretVersionRequest{
			SecretId:        secret.Id,
			SecretVersionId: "latest",
		})
		if aerr != nil {
			return nil, fmt.Errorf("access secret %q: %w", secret.Path, aerr)
		}

		versionID := p.secretLatestVersionID(secret)
		if versionID == nil {
			return nil, fmt.Errorf("corrupted version of secret %q", secret.Path)
		}

		secretsMap[secret.Path] = contracts.Secret{
			Path:      secret.Path,
			Value:     secretValue.GetData().GetValue(),
			VersionID: *versionID,
		}
	}

	return secretsMap, nil
}

func (p *Provider) secretLatestVersionID(secret *v2.Secret) *string {
	for _, version := range secret.Versions {
		if version.State == v2.VersionState_ENABLED {
			id := strconv.Itoa(int(version.Id))
			return &id
		}
	}

	return nil
}
