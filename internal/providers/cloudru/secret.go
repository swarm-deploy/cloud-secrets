package cloudru

import (
	"context"
	"fmt"
	"strconv"

	v2 "github.com/cloudru-tech/secret-manager-sdk/api/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
)

func (p *Provider) GetSecretPayload(ctx context.Context, key string) ([]byte, error) {
	resp, err := p.secretManager.V2.SecretService.Access(ctx, &v2.AccessSecretRequest{
		Path:      key,
		ProjectId: p.cfg.ProjectID,
	})
	if err != nil {
		return nil, fmt.Errorf("access secret %q: %w", key, err)
	}

	return resp.GetPayload().GetValue(), nil
}

func (p *Provider) CreateSecret(ctx context.Context, secret contracts.Secret, payload []byte) error {
	_, err := p.secretManager.V2.SecretService.Create(ctx, &v2.CreateSecretRequest{
		Path:      secret.Path,
		Payload:   wrapperspb.Bytes(payload),
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
		versionID := p.secretLatestVersionID(secret)
		if versionID == nil {
			return nil, fmt.Errorf("corrupted version of secret %q", secret.Path)
		}

		secretsMap[secret.Path] = contracts.Secret{
			Path:      secret.Path,
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
