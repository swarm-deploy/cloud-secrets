package cloudru

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	iamAuthV1 "github.com/cloudru-tech/iam-sdk/api/auth/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru/certificateapi"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/metadata"
)

func TestIAMSharedToken(t *testing.T) {
	t.Parallel()
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "cached", true: "expired"}[expired], func(t *testing.T) {
			t.Parallel()
			client := NewMockiamTokenClient(gomock.NewController(t))
			auth := iamInterceptor{iamClient: client, accessKey: "key", accessSecret: "secret"}
			if expired {
				auth.accessToken = "old"
				auth.accessTokenExpiresAt = time.Now().Add(-time.Minute)
			}
			client.EXPECT().GetToken(gomock.Any(), &iamAuthV1.GetTokenRequest{KeyId: "key", Secret: "secret"}).Return(
				&iamAuthV1.GetTokenResponse{AccessToken: "shared", ExpiresIn: 3600}, nil).Times(1)
			original := metadata.Pairs("authorization", "original")
			ctx := metadata.NewOutgoingContext(context.Background(), original)
			enriched, err := auth.enrich(ctx)
			require.NoError(t, err)
			md, ok := metadata.FromOutgoingContext(enriched)
			require.True(t, ok)
			assert.Equal(t, []string{"Bearer shared"}, md.Get("authorization"))
			assert.Equal(t, []string{"original"}, original.Get("authorization"))
			token, err := auth.getOrCreateToken(ctx)
			require.NoError(t, err)
			assert.Equal(t, "shared", token)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer shared", r.Header.Get("Authorization"))
				_, writeErr := w.Write([]byte(`{"certificates":[]}`))
				assert.NoError(t, writeErr)
			}))
			defer server.Close()
			httpClient, clientErr := certificateapi.NewClient(server.URL, server.Client(), auth.getOrCreateToken)
			require.NoError(t, clientErr)
			_, listErr := httpClient.ListCertificates(ctx, "project")
			require.NoError(t, listErr)
		})
	}
}
