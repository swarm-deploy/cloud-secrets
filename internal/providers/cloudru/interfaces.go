//go:generate mockgen -source=$GOFILE -destination=mocks.go -package=cloudru
package cloudru

import (
	"context"

	iamAuthV1 "github.com/cloudru-tech/iam-sdk/api/auth/v1"
	v2 "github.com/cloudru-tech/secret-manager-sdk/api/v2"
	"google.golang.org/grpc"
)

type secretService interface {
	// Search lists Secret Manager metadata within the configured project and folder.
	Search(context.Context, *v2.SearchSecretRequest, ...grpc.CallOption) (*v2.SearchSecretResponse, error)
	// Access reads the latest Secret Manager payload.
	Access(context.Context, *v2.AccessSecretRequest, ...grpc.CallOption) (*v2.AccessSecretResponse, error)
}

type iamTokenClient interface {
	// GetToken exchanges the configured IAM key and secret for an access token.
	GetToken(context.Context, *iamAuthV1.GetTokenRequest, ...grpc.CallOption) (*iamAuthV1.GetTokenResponse, error)
}
