package cloudru

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	v2 "github.com/cloudru-tech/secret-manager-sdk/api/v2"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// CreateSecret creates a Cloud.ru Secret Manager secret in the configured scope.
func (p *Provider) CreateSecret(ctx context.Context, req contracts.CreateSecretRequest) (string, error) {
	if req.Key != "" {
		return "", fmt.Errorf("%w: key must be empty for Cloud.ru", contracts.ErrInvalidCreateRequest)
	}

	logicalPath, fullPath, err := p.createSecretPath(req.Path)
	if err != nil {
		return "", err
	}

	_, err = p.secretService.Create(ctx, &v2.CreateSecretRequest{
		ProjectId:   p.cfg.ProjectID,
		Path:        fullPath,
		Description: req.Description,
		Payload:     wrapperspb.Bytes(req.Value),
	})
	if err != nil {
		code := status.Code(err)
		if code == codes.AlreadyExists {
			return "", fmt.Errorf("%w: Cloud.ru secret %q", contracts.ErrSecretAlreadyExists, logicalPath)
		}
		if code == codes.InvalidArgument {
			return "", fmt.Errorf("%w: Cloud.ru rejected path %q: %w", contracts.ErrInvalidCreateRequest, logicalPath, err)
		}

		return "", fmt.Errorf("create Cloud.ru secret %q: %w", logicalPath, err)
	}

	return logicalPath, nil
}

func (p *Provider) GetSecretPayload(ctx context.Context, key string) ([]byte, error) {
	resp, err := p.secretService.Access(ctx, &v2.AccessSecretRequest{
		Path:      p.resolveSecretPath(key),
		ProjectId: p.cfg.ProjectID,
	})
	if err != nil {
		return nil, fmt.Errorf("access secret %q: %w", key, err)
	}

	return resp.GetPayload().GetValue(), nil
}

func (p *Provider) ListSecrets(ctx context.Context) (map[string]contracts.Secret, error) {
	req := &v2.SearchSecretRequest{
		ProjectId: p.cfg.ProjectID,
		Depth:     -1,
	}

	if rootFolder := string(p.cfg.RootFolder); rootFolder != "" {
		req.Path = rootFolder
	}

	secretsResp, err := p.secretService.Search(ctx, req)
	if err != nil {
		return nil, err
	}

	secretsMap := map[string]contracts.Secret{}

	for _, secret := range secretsResp.Secrets {
		mapped, merr := p.mapSecret(secret)
		if merr != nil {
			return nil, fmt.Errorf("map secret %q: %w", secret.Name, merr)
		}

		secretsMap[secret.Path] = mapped
	}

	return secretsMap, nil
}

func (p *Provider) createSecretPath(value string) (string, string, error) {
	normalized := strings.Trim(strings.TrimSpace(value), "/")
	if normalized == "" {
		return "", "", fmt.Errorf("%w: path is required", contracts.ErrInvalidCreateRequest)
	}

	for _, segment := range strings.Split(normalized, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", "", fmt.Errorf("%w: path contains an invalid segment", contracts.ErrInvalidCreateRequest)
		}
	}

	rootFolder := string(p.cfg.RootFolder)
	if rootFolder == "" {
		return normalized, normalized, nil
	}

	if p.cfg.RootFolderOmitPrefix {
		fullPath := p.resolveSecretPath(normalized)
		logicalPath, err := p.scopeSecretPath(fullPath)
		if err != nil {
			return "", "", fmt.Errorf("%w: %w", contracts.ErrInvalidCreateRequest, err)
		}

		return logicalPath, fullPath, nil
	}

	if normalized == rootFolder || !strings.HasPrefix(normalized, rootFolder+"/") {
		return "", "", fmt.Errorf(
			"%w: path must be inside root folder %q",
			contracts.ErrInvalidCreateRequest,
			rootFolder,
		)
	}

	return normalized, normalized, nil
}

func (p *Provider) mapSecret(secret *v2.Secret) (contracts.Secret, error) {
	versionID := p.secretLatestVersionID(secret)
	if versionID == nil {
		return contracts.Secret{}, fmt.Errorf("corrupted version of secret %q", secret.Path)
	}

	scopedPath, err := p.scopeSecretPath(secret.Path)
	if err != nil {
		return contracts.Secret{}, fmt.Errorf("scope secret path %q: %w", secret.Path, err)
	}

	return contracts.Secret{
		Path:        scopedPath,
		FullPath:    secret.Path,
		VersionID:   *versionID,
		Description: secret.GetDescription(),
	}, nil
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

func (p *Provider) resolveSecretPath(path string) string {
	if !p.cfg.RootFolderOmitPrefix {
		return path
	}

	rootFolder := string(p.cfg.RootFolder)
	if rootFolder == "" {
		return path
	}

	trimmedPath := strings.Trim(path, "/")
	if trimmedPath == "" {
		return rootFolder
	}

	if trimmedPath == rootFolder || strings.HasPrefix(trimmedPath, rootFolder+"/") {
		return trimmedPath
	}

	return rootFolder + "/" + trimmedPath
}

func (p *Provider) scopeSecretPath(path string) (string, error) {
	trimmedPath := strings.Trim(path, "/")
	if !p.cfg.RootFolderOmitPrefix {
		return trimmedPath, nil
	}

	rootFolder := string(p.cfg.RootFolder)
	if rootFolder == "" {
		return trimmedPath, nil
	}

	if trimmedPath == rootFolder {
		return "", errors.New("path matches root folder")
	}

	prefix := rootFolder + "/"
	if !strings.HasPrefix(trimmedPath, prefix) {
		return "", fmt.Errorf("path is outside root folder %q", rootFolder)
	}

	return strings.TrimPrefix(trimmedPath, prefix), nil
}
