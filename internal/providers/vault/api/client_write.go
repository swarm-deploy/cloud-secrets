package api

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

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

func (c *HttpClient) CreateACLPolicy(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("policy name is empty")
	}

	rules := fmt.Sprintf(`
path "%s/metadata" {
  capabilities = ["list"]
}

path "%s/metadata/*" {
  capabilities = ["read", "list"]
}

path "%s/data/*" {
  capabilities = ["read"]
}
`, c.mountPath, c.mountPath, c.mountPath)

	if err := c.client.Sys().PutPolicyWithContext(ctx, name, strings.TrimSpace(rules)); err != nil {
		return fmt.Errorf("create ACL policy %q: %w", name, err)
	}

	return nil
}
