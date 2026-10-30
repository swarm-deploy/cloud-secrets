//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=api
package api

import "context"

type Client interface {
	// ListKeys lists KV document and folder names below path.
	ListKeys(ctx context.Context, path string) ([]string, error)
	// GetSecret reads KV document metadata or one key payload.
	GetSecret(ctx context.Context, path, key string) (*Secret, error)
	// CreateSecret writes a complete KV document for CLI and setup operations.
	CreateSecret(ctx context.Context, path string, data map[string]interface{}) (*Secret, error)
	// CreateSecretKey creates one key without replacing existing document keys.
	CreateSecretKey(ctx context.Context, path, key string, value []byte) error
	// DeleteSecret deletes KV document metadata.
	DeleteSecret(ctx context.Context, path string) error

	// CreateACLPolicy creates or replaces a Vault ACL policy.
	CreateACLPolicy(ctx context.Context, req CreateACLPolicyRequest) error

	// CreateToken creates a Vault token with the provided policies.
	CreateToken(ctx context.Context, policies []string) (string, error)

	// AppRoleAuthEnabled reports whether AppRole authentication is enabled.
	AppRoleAuthEnabled(ctx context.Context) (bool, error)
	// EnableAppRoleAuth enables AppRole authentication.
	EnableAppRoleAuth(ctx context.Context) error
	// CreateAppRole creates or replaces an AppRole.
	CreateAppRole(ctx context.Context, req CreateAppRoleRequest) error
	// ReadAppRoleRoleID reads an AppRole role ID.
	ReadAppRoleRoleID(ctx context.Context, name string) (string, error)
	// CreateAppRoleSecretID creates an AppRole secret ID.
	CreateAppRoleSecretID(ctx context.Context, name string) (string, error)
}

type Authenticator interface {
	// Authenticate refreshes or establishes Vault authentication.
	Authenticate(ctx context.Context) error
}

type CreateACLPolicyRequest struct {
	Name  string
	Rules string
}

type CreateAppRoleRequest struct {
	Name            string
	TokenPolicies   []string
	TokenTTL        string
	TokenMaxTTL     string
	SecretIDTTL     string
	SecretIDNumUses int
}
