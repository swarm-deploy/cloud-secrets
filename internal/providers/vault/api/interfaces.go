//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=api
package api

import "context"

type Client interface {
	ListKeys(ctx context.Context, path string) ([]string, error)
	Get(ctx context.Context, path, key string) (*Secret, error)
}

type Authenticator interface {
	Authenticate(ctx context.Context) error
}
