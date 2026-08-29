//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=api
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"

	vaultapi "github.com/hashicorp/vault/api"
)

type HttpClient struct {
	client    *vaultapi.Client
	kv        *vaultapi.KVv2
	mountPath string
}

type Secret struct {
	VersionID string
	Keys      []string
	Payload   []byte
}

func NewHttpClient(
	ctx context.Context,
	mountPath string,
	addr url.URL,
	authConfig AuthConfig,
) (Client, error) {
	clientBuild := func() (Client, error) {
		vaultCfg := vaultapi.DefaultConfig()
		vaultCfg.Address = addr.String()

		client, err := vaultapi.NewClient(vaultCfg)
		if err != nil {
			return nil, fmt.Errorf("create vault client: %w", err)
		}

		httpClient := &HttpClient{
			client:    client,
			kv:        client.KVv2(mountPath),
			mountPath: strings.Trim(mountPath, "/"),
		}

		if authConfig.Token != "" {
			client.SetToken(authConfig.Token)

			return httpClient, nil
		}

		roleAuthenticator, err := NewAppRoleAuthenticator(
			client,
			authConfig.AppRole,
		)
		if err != nil {
			return nil, fmt.Errorf("create app role authenticator: %w", err)
		}

		decoratedClient := &AuthenticatedClient{
			client:        httpClient,
			authenticator: roleAuthenticator,
		}

		return decoratedClient, nil
	}

	client, err := clientBuild()
	if err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "[vault] ping")

	if _, err = client.ListKeys(ctx, "/"); err != nil {
		return nil, fmt.Errorf("ping vault: %w", err)
	}

	slog.InfoContext(ctx, "[vault] client created", slog.String("path", mountPath))

	return client, nil
}

func (c *HttpClient) ListKeys(ctx context.Context, path string) ([]string, error) {
	secret, err := c.client.Logical().ListWithContext(ctx, c.metadataPath(path))
	if err != nil {
		return nil, err
	}
	if secret == nil {
		return nil, nil
	}

	rawKeys, ok := secret.Data["keys"]
	if !ok {
		return nil, nil
	}

	switch keys := rawKeys.(type) {
	case []interface{}:
		out := make([]string, 0, len(keys))
		for _, key := range keys {
			keyString, keyIsString := key.(string)
			if !keyIsString {
				return nil, fmt.Errorf("unexpected key type %T", key)
			}
			out = append(out, keyString)
		}
		return out, nil
	case []string:
		return keys, nil
	default:
		return nil, fmt.Errorf("unexpected keys payload type %T", rawKeys)
	}
}

func (c *HttpClient) GetSecret(ctx context.Context, path, key string) (*Secret, error) {
	path = strings.Trim(path, "/")

	secret, err := c.kv.Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("read secret %q: %w", path, err)
	}

	keys := make([]string, 0, len(secret.Data))
	for secretKey := range secret.Data {
		keys = append(keys, secretKey)
	}
	sort.Strings(keys)

	out := &Secret{
		Keys: keys,
	}

	versionID, err := c.versionID(ctx, path, secret)
	if err != nil {
		return nil, err
	}
	out.VersionID = versionID

	if key == "" {
		return out, nil
	}

	value, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("secret %q key %q not found", path, key)
	}

	payload, err := toBytes(value)
	if err != nil {
		return nil, fmt.Errorf("convert payload for %q key %q: %w", path, key, err)
	}

	out.Payload = payload

	return out, nil
}

func (c *HttpClient) metadataPath(secretPath string) string {
	secretPath = strings.Trim(secretPath, "/")
	if secretPath == "" {
		return fmt.Sprintf("%s/metadata", c.mountPath)
	}

	return fmt.Sprintf("%s/metadata/%s", c.mountPath, secretPath)
}

func (c *HttpClient) versionID(ctx context.Context, path string, secret *vaultapi.KVSecret) (string, error) {
	if secret.VersionMetadata != nil {
		return fmt.Sprintf("%d", secret.VersionMetadata.Version), nil
	}

	metadata, err := c.kv.GetMetadata(ctx, path)
	if err != nil {
		return "", fmt.Errorf("read metadata for %q: %w", path, err)
	}

	return fmt.Sprintf("%d", metadata.CurrentVersion), nil
}

func toBytes(value interface{}) ([]byte, error) {
	switch typed := value.(type) {
	case string:
		return []byte(typed), nil
	case []byte:
		return typed, nil
	case json.Number:
		return []byte(typed.String()), nil
	default:
		return json.Marshal(value)
	}
}
