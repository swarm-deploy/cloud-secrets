package vault

import (
	"context"
	"fmt"
	"strings"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	vaultclient "github.com/swarm-deploy/cloud-secrets/internal/providers/vault/api"
)

// Provider reads secrets from HashiCorp Vault KV v2.
type Provider struct {
	cfg Config

	client vaultclient.Client
}

func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	cfg.MountPath = strings.Trim(cfg.MountPath, "/")

	client, err := vaultclient.NewHttpClient(ctx, cfg.MountPath, cfg.Addr, cfg.Auth)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	provider := &Provider{
		cfg:    cfg,
		client: client,
	}

	return provider, nil
}

func (p *Provider) Definition() contracts.ProviderDefinition {
	return contracts.ProviderDefinition{
		Name: "HashiCorp Vault",
	}
}

func (p *Provider) GetSecretPayload(ctx context.Context, path string) ([]byte, error) {
	sp, err := parseSecretPath(path)
	if err != nil {
		return nil, fmt.Errorf("invalid secret path %q: %w", path, err)
	}

	secret, err := p.client.GetSecret(ctx, sp.Path, sp.Key)
	if err != nil {
		return nil, fmt.Errorf("read parent secret %q for key %q: %w", sp.Path, sp.Key, err)
	}

	return secret.Payload, nil
}

//nolint:gocognit // clear flow for traversal and key expansion
func (p *Provider) ListSecrets(ctx context.Context) (map[string]contracts.Secret, error) {
	secrets := make(map[string]contracts.Secret)

	pathsToScan := []string{""}
	seenPaths := map[string]struct{}{}

	for len(pathsToScan) > 0 {
		path := pathsToScan[0]
		pathsToScan = pathsToScan[1:]

		if _, ok := seenPaths[path]; ok {
			continue
		}
		seenPaths[path] = struct{}{}

		keys, err := p.client.ListKeys(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list keys for %q: %w", path, err)
		}

		for _, key := range keys {
			if strings.HasSuffix(key, "/") {
				pathsToScan = append(pathsToScan, joinPath(path, strings.TrimSuffix(key, "/")))
				continue
			}

			fullPath := joinPath(path, key)
			secret, getErr := p.client.GetSecret(ctx, fullPath, "")
			if getErr != nil {
				return nil, fmt.Errorf("read secret %q: %w", fullPath, getErr)
			}

			for _, keyName := range secret.Keys {
				if keyName == "" {
					return nil, fmt.Errorf("secret %q contains empty key", fullPath)
				}
				if strings.Contains(keyName, "/") {
					return nil, fmt.Errorf("secret %q has unsupported key %q containing /", fullPath, keyName)
				}

				secretPath := joinPath(fullPath, keyName)
				secrets[secretPath] = contracts.Secret{
					Path:      secretPath,
					VersionID: secret.VersionID,
				}
			}
		}
	}

	return secrets, nil
}

func joinPath(parent string, child string) string {
	parent = strings.Trim(parent, "/")
	child = strings.Trim(child, "/")

	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}

	return parent + "/" + child
}
