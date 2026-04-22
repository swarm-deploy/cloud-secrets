package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
)

func (p *Provider) GetSecretPayload(ctx context.Context, key string) ([]byte, error) {
	secretPath := strings.Trim(key, "/")
	if secretPath == "" {
		return nil, errors.New("secret path is empty")
	}

	secret, err := p.client.Logical().ReadWithContext(ctx, p.dataPath(secretPath))
	if err != nil {
		return nil, fmt.Errorf("read secret %q: %w", secretPath, err)
	}
	if secret == nil {
		return nil, fmt.Errorf("secret %q not found", secretPath)
	}

	rawData, ok := secret.Data["data"]
	if !ok {
		return nil, fmt.Errorf("secret %q has no data payload", secretPath)
	}

	data, ok := rawData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("secret %q payload has unexpected type %T", secretPath, rawData)
	}

	payload, err := extractPayload(data)
	if err != nil {
		return nil, fmt.Errorf("extract payload for %q: %w", secretPath, err)
	}

	return payload, nil
}

func (p *Provider) ListSecrets(ctx context.Context) (map[string]contracts.Secret, error) {
	secrets := make(map[string]contracts.Secret)

	pathsToScan := []string{p.cfg.Prefix}
	seenPaths := map[string]struct{}{}

	for len(pathsToScan) > 0 {
		path := pathsToScan[0]
		pathsToScan = pathsToScan[1:]

		if _, ok := seenPaths[path]; ok {
			continue
		}
		seenPaths[path] = struct{}{}

		keys, err := p.listKeys(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("list keys for %q: %w", path, err)
		}

		for _, key := range keys {
			if strings.HasSuffix(key, "/") {
				pathsToScan = append(pathsToScan, joinPath(path, strings.TrimSuffix(key, "/")))
				continue
			}

			fullPath := joinPath(path, key)
			versionID, versionErr := p.readCurrentVersion(ctx, fullPath)
			if versionErr != nil {
				return nil, fmt.Errorf("read current version for %q: %w", fullPath, versionErr)
			}

			secrets[fullPath] = contracts.Secret{
				Path:      fullPath,
				VersionID: versionID,
			}
		}
	}

	return secrets, nil
}

func (p *Provider) listKeys(ctx context.Context, path string) ([]string, error) {
	secret, err := p.client.Logical().ListWithContext(ctx, p.metadataPath(path))
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

func (p *Provider) readCurrentVersion(ctx context.Context, path string) (string, error) {
	secret, err := p.client.Logical().ReadWithContext(ctx, p.metadataPath(path))
	if err != nil {
		return "", err
	}
	if secret == nil {
		return "", errors.New("secret metadata not found")
	}

	rawVersion, ok := secret.Data["current_version"]
	if !ok {
		return "", errors.New("current_version not found in metadata")
	}

	return parseVersion(rawVersion)
}

func parseVersion(rawVersion interface{}) (string, error) {
	switch value := rawVersion.(type) {
	case string:
		if value == "" {
			return "", errors.New("empty version")
		}
		return value, nil
	case int:
		return strconv.Itoa(value), nil
	case int32:
		return strconv.FormatInt(int64(value), 10), nil
	case int64:
		return strconv.FormatInt(value, 10), nil
	case float64:
		return strconv.FormatInt(int64(value), 10), nil
	case json.Number:
		return value.String(), nil
	default:
		return "", fmt.Errorf("unsupported version type %T", rawVersion)
	}
}

func extractPayload(data map[string]interface{}) ([]byte, error) {
	if value, ok := data["value"]; ok {
		return toBytes(value)
	}

	if len(data) == 1 {
		for _, value := range data {
			return toBytes(value)
		}
	}

	return json.Marshal(data)
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

func (p *Provider) dataPath(secretPath string) string {
	return p.apiPath("data", secretPath)
}

func (p *Provider) metadataPath(secretPath string) string {
	return p.apiPath("metadata", secretPath)
}

func (p *Provider) apiPath(kind string, secretPath string) string {
	secretPath = strings.Trim(secretPath, "/")
	if secretPath == "" {
		return fmt.Sprintf("%s/%s", p.cfg.MountPath, kind)
	}

	return fmt.Sprintf("%s/%s/%s", p.cfg.MountPath, kind, secretPath)
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
