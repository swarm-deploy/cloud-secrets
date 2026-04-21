package cloudru

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	iamAuthV1 "github.com/cloudru-tech/iam-sdk/api/auth/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type iamInterceptor struct {
	iamClient iamAuthV1.AuthServiceClient

	mu                   sync.Mutex
	accessToken          string
	accessTokenExpiresAt time.Time

	accessKey    string
	accessSecret string
}

func (i *iamInterceptor) intercept() grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req,
		reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		ctx, err := i.enrich(ctx)
		if err != nil {
			return fmt.Errorf("failed to enrich iam context: %w", err)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func (i *iamInterceptor) enrich(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(map[string]string{})
	}
	token, err := i.getOrCreateToken(ctx)
	if err != nil {
		return ctx, fmt.Errorf("fetch IAM access token: %w", err)
	}

	md.Set("authorization", "Bearer "+token)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func (i *iamInterceptor) getOrCreateToken(ctx context.Context) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	if i.accessToken != "" && i.accessTokenExpiresAt.After(time.Now()) {
		return i.accessToken, nil
	}

	slog.InfoContext(ctx, "[iam] request new token")

	resp, err := i.iamClient.GetToken(ctx, &iamAuthV1.GetTokenRequest{KeyId: i.accessKey, Secret: i.accessSecret})
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}

	slog.InfoContext(ctx, "[iam] new token fetched")

	i.accessToken = resp.AccessToken
	i.accessTokenExpiresAt = time.Now().Add(time.Second * time.Duration(resp.ExpiresIn))
	return i.accessToken, nil
}
