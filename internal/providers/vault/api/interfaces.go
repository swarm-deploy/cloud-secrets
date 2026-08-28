//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=api
package api

import "context"

type Client interface {
	ListKeys(ctx context.Context, path string) ([]string, error)
	Get(ctx context.Context, path, key string) (*Secret, error)

	CreateSecret(ctx context.Context, path string, data map[string]interface{}) (*Secret, error)
	CreateACLPolicy(ctx context.Context, req CreateACLPolicyRequest) error
	DeleteSecret(ctx context.Context, path string) error
	CreateToken(ctx context.Context, policies []string) (string, error)

	AppRoleAuthEnabled(ctx context.Context) (bool, error)
	EnableAppRoleAuth(ctx context.Context) error
	CreateAppRole(ctx context.Context, name string, policyName string) error
	ReadAppRoleRoleID(ctx context.Context, name string) (string, error)
	CreateAppRoleSecretID(ctx context.Context, name string) (string, error)
}

type Authenticator interface {
	Authenticate(ctx context.Context) error
}
