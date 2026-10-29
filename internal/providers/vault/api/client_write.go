package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

var (
	// ErrSecretKeyExists indicates that a KV document already contains the requested key.
	ErrSecretKeyExists = errors.New("secret key already exists")
	// ErrCASConflict indicates that repeated concurrent KV updates prevented a safe write.
	ErrCASConflict = errors.New("vault KV check-and-set conflict")
)

const createSecretKeyAttempts = 3

func (c *HttpClient) CreateSecret(ctx context.Context, path string, data map[string]interface{}) (*Secret, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, fmt.Errorf("secret path is empty")
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("secret %q data is empty", path)
	}

	secret, err := c.kv.Put(ctx, path, data)
	if err != nil {
		return nil, fmt.Errorf("write secret %q: %w", path, err)
	}

	versionID, err := c.versionID(ctx, path, secret)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return &Secret{
		VersionID: versionID,
		Keys:      keys,
	}, nil
}

// CreateSecretKey adds one key to a KV document using check-and-set writes.
func (c *HttpClient) CreateSecretKey(ctx context.Context, path, key string, value []byte) error {
	path = strings.Trim(path, "/")
	if path == "" {
		return fmt.Errorf("secret path is empty")
	}
	if key == "" {
		return fmt.Errorf("secret key is empty")
	}

	for range createSecretKeyAttempts {
		data, cas, err := c.secretDataForCreate(ctx, path, key, value)
		if err != nil {
			return err
		}

		_, err = c.kv.Put(ctx, path, data, vaultapi.WithCheckAndSet(cas))
		if err == nil {
			return nil
		}
		if !isCASConflict(err) {
			return fmt.Errorf("create secret %q key %q: %w", path, key, err)
		}
	}

	return fmt.Errorf("%w for secret %q key %q", ErrCASConflict, path, key)
}

func (c *HttpClient) secretDataForCreate(
	ctx context.Context,
	path string,
	key string,
	value []byte,
) (map[string]interface{}, int, error) {
	secret, err := c.kv.Get(ctx, path)
	if err != nil && !errors.Is(err, vaultapi.ErrSecretNotFound) {
		return nil, 0, fmt.Errorf("read secret %q before create: %w", path, err)
	}

	data := map[string]interface{}{key: string(value)}
	if errors.Is(err, vaultapi.ErrSecretNotFound) {
		return data, 0, nil
	}
	if _, exists := secret.Data[key]; exists {
		return nil, 0, fmt.Errorf("%w: secret %q key %q", ErrSecretKeyExists, path, key)
	}
	if secret.VersionMetadata == nil {
		return nil, 0, fmt.Errorf("secret %q has no version metadata", path)
	}

	data = make(map[string]interface{}, len(secret.Data)+1)
	for existingKey, existingValue := range secret.Data {
		data[existingKey] = existingValue
	}
	data[key] = string(value)

	return data, secret.VersionMetadata.Version, nil
}

func isCASConflict(err error) bool {
	var responseErr *vaultapi.ResponseError
	if !errors.As(err, &responseErr) || responseErr.StatusCode != http.StatusBadRequest {
		return false
	}

	for _, message := range responseErr.Errors {
		if strings.Contains(strings.ToLower(message), "check-and-set") {
			return true
		}
	}

	return false
}

func (c *HttpClient) DeleteSecret(ctx context.Context, path string) error {
	path = strings.Trim(path, "/")
	if path == "" {
		return fmt.Errorf("secret path is empty")
	}

	if err := c.kv.DeleteMetadata(ctx, path); err != nil {
		return fmt.Errorf("delete secret %q metadata: %w", path, err)
	}

	return nil
}

func (c *HttpClient) CreateACLPolicy(ctx context.Context, req CreateACLPolicyRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fmt.Errorf("policy name is empty")
	}
	req.Rules = strings.TrimSpace(req.Rules)
	if req.Rules == "" {
		return fmt.Errorf("policy rules are empty")
	}

	if err := c.client.Sys().PutPolicyWithContext(ctx, req.Name, req.Rules); err != nil {
		return fmt.Errorf("create ACL policy %q: %w", req.Name, err)
	}

	return nil
}

func (c *HttpClient) CreateToken(ctx context.Context, policies []string) (string, error) {
	secret, err := c.client.Auth().Token().CreateWithContext(ctx, &vaultapi.TokenCreateRequest{
		Policies:        policies,
		DisplayName:     "cloud-secrets-e2e",
		TTL:             "1h",
		NoDefaultPolicy: true,
	})
	if err != nil {
		return "", fmt.Errorf("create Vault token: %w", err)
	}
	if secret == nil || secret.Auth == nil || secret.Auth.ClientToken == "" {
		return "", fmt.Errorf("create Vault token: Vault returned empty token")
	}

	return secret.Auth.ClientToken, nil
}
