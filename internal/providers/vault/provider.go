package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

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
		Links: contracts.Links{
			Doc: "https://developer.hashicorp.com/vault/docs",
		},
	}
}

// CreateSecret creates a new key in a Vault KV v2 document.
func (p *Provider) CreateSecret(ctx context.Context, req contracts.CreateSecretRequest) (contracts.Secret, error) {
	path, err := normalizeCreatePath(req.Path)
	if err != nil {
		return contracts.Secret{}, err
	}

	key := strings.TrimSpace(req.Key)
	if key == "" {
		return contracts.Secret{}, fmt.Errorf("%w: key is required for Vault KV v2", contracts.ErrInvalidCreateRequest)
	}
	if strings.Contains(key, "/") {
		return contracts.Secret{}, fmt.Errorf("%w: key must not contain /", contracts.ErrInvalidCreateRequest)
	}
	if req.Description != "" {
		return contracts.Secret{}, fmt.Errorf(
			"%w: description is not supported by Vault KV v2",
			contracts.ErrInvalidCreateRequest,
		)
	}
	if !utf8.Valid(req.Value) {
		return contracts.Secret{}, fmt.Errorf(
			"%w: value must be valid UTF-8 for Vault KV v2",
			contracts.ErrInvalidCreateRequest,
		)
	}

	err = p.client.CreateSecretKey(ctx, path, key, req.Value)
	if err != nil {
		switch {
		case errors.Is(err, vaultclient.ErrSecretKeyExists):
			return contracts.Secret{}, fmt.Errorf(
				"%w: Vault document %q already contains key %q",
				contracts.ErrSecretAlreadyExists,
				path,
				key,
			)
		case errors.Is(err, vaultclient.ErrCASConflict):
			return contracts.Secret{}, fmt.Errorf(
				"%w: Vault document %q changed concurrently",
				contracts.ErrCreateConflict,
				path,
			)
		default:
			return contracts.Secret{}, fmt.Errorf("create Vault secret %q key %q: %w", path, key, err)
		}
	}

	return contracts.Secret{Path: path}, nil
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

func normalizeCreatePath(value string) (string, error) {
	normalized := strings.Trim(strings.TrimSpace(value), "/")
	if normalized == "" {
		return "", fmt.Errorf("%w: path is required", contracts.ErrInvalidCreateRequest)
	}

	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: path contains an invalid segment", contracts.ErrInvalidCreateRequest)
		}
	}

	return normalized, nil
}
