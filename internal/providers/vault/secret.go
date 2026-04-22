package vault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
)

func (p *Provider) GetSecretPayload(ctx context.Context, key string) ([]byte, error) {
	secretPath := strings.Trim(key, "/")
	if secretPath == "" {
		return nil, errors.New("secret path is empty")
	}

	basePath, keyName, splitErr := splitSecretKeyPath(secretPath)
	if splitErr != nil {
		return nil, fmt.Errorf("invalid secret path %q: %w", secretPath, splitErr)
	}

	baseData, err := p.readSecretData(ctx, basePath)
	if err != nil {
		return nil, fmt.Errorf("read parent secret %q for key %q: %w", basePath, keyName, err)
	}

	value, ok := baseData[keyName]
	if !ok {
		return nil, fmt.Errorf("secret %q key %q not found", basePath, keyName)
	}

	payload, payloadErr := toBytes(value)
	if payloadErr != nil {
		return nil, fmt.Errorf("convert payload for %q key %q: %w", basePath, keyName, payloadErr)
	}

	return payload, nil
}

//nolint:gocognit // clear flow for traversal and key expansion
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

			keyNames, keysErr := p.readSecretKeys(ctx, fullPath)
			if keysErr != nil {
				return nil, fmt.Errorf("read keys for %q: %w", fullPath, keysErr)
			}

			for _, keyName := range keyNames {
				if keyName == "" {
					return nil, fmt.Errorf("secret %q contains empty key", fullPath)
				}
				if strings.Contains(keyName, "/") {
					return nil, fmt.Errorf("secret %q has unsupported key %q containing /", fullPath, keyName)
				}

				secretPath := joinPath(fullPath, keyName)
				secrets[secretPath] = contracts.Secret{
					Path:      secretPath,
					VersionID: versionID,
				}
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

func (p *Provider) readSecretKeys(ctx context.Context, path string) ([]string, error) {
	data, err := p.readSecretData(ctx, path)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys, nil
}

func (p *Provider) readSecretData(ctx context.Context, path string) (map[string]interface{}, error) {
	secret, err := p.client.Logical().ReadWithContext(ctx, p.dataPath(path))
	if err != nil {
		return nil, fmt.Errorf("read secret %q: %w", path, err)
	}
	if secret == nil {
		return nil, fmt.Errorf("secret %q not found", path)
	}

	rawData, ok := secret.Data["data"]
	if !ok {
		return nil, fmt.Errorf("secret %q has no data payload", path)
	}

	data, ok := rawData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("secret %q payload has unexpected type %T", path, rawData)
	}

	return data, nil
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

func splitSecretKeyPath(path string) (secretPath string, keyName string, err error) {
	idx := strings.LastIndex(path, "/")
	if idx <= 0 || idx == len(path)-1 {
		return "", "", fmt.Errorf("secret path %q does not contain a key segment", path)
	}

	secretPath = path[:idx]
	keyName = path[idx+1:]
	return secretPath, keyName, nil
}
