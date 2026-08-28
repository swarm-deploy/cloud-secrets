package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHttpClientConfiguresAppRole(t *testing.T) {
	ctx := context.Background()
	requests := map[string]int{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests[r.Method+" "+r.URL.Path]++
		require.Equal(t, "setup-token", r.Header.Get("X-Vault-Token"))

		switch r.Method + " " + r.URL.Path {
		case "GET /v1/prod/metadata":
			writeJSON(t, w, map[string]interface{}{
				"data": map[string]interface{}{
					"keys": []interface{}{},
				},
			})
		case "GET /v1/sys/auth":
			writeJSON(t, w, map[string]interface{}{
				"data": map[string]interface{}{
					"approle/": map[string]interface{}{
						"type": "approle",
					},
				},
			})
		case "POST /v1/sys/auth/approle":
			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, "approle", body["type"])
			w.WriteHeader(http.StatusNoContent)
		case "PUT /v1/sys/policies/acl/cloud-secrets":
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, `path "prod/metadata" {
  capabilities = ["list"]
}

path "prod/metadata/*" {
  capabilities = ["read", "list"]
}

path "prod/data/*" {
  capabilities = ["read"]
}`, body["policy"])
			w.WriteHeader(http.StatusNoContent)
		case "PUT /v1/auth/approle/role/cloud-secrets":
			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.Equal(t, []interface{}{"cloud-secrets"}, body["token_policies"])
			require.Equal(t, "1h", body["token_ttl"])
			require.Equal(t, "4h", body["token_max_ttl"])
			require.Equal(t, "0", body["secret_id_ttl"])
			require.Equal(t, float64(0), body["secret_id_num_uses"])
			w.WriteHeader(http.StatusNoContent)
		case "GET /v1/auth/approle/role/cloud-secrets/role-id":
			writeJSON(t, w, map[string]interface{}{
				"data": map[string]interface{}{
					"role_id": "role-id",
				},
			})
		case "PUT /v1/auth/approle/role/cloud-secrets/secret-id":
			writeJSON(t, w, map[string]interface{}{
				"data": map[string]interface{}{
					"secret_id": "secret-id",
				},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	addr, err := url.Parse(server.URL)
	require.NoError(t, err)

	client, err := NewHttpClient(ctx, "prod", *addr, AuthConfig{
		Token: "setup-token",
	})
	require.NoError(t, err)

	enabled, err := client.AppRoleAuthEnabled(ctx)
	require.NoError(t, err)
	require.True(t, enabled)
	require.NoError(t, client.EnableAppRoleAuth(ctx))
	require.NoError(t, client.CreateACLPolicy(ctx, CreateACLPolicyRequest{
		Name: "cloud-secrets",
		Rules: `
path "prod/metadata" {
  capabilities = ["list"]
}

path "prod/metadata/*" {
  capabilities = ["read", "list"]
}

path "prod/data/*" {
  capabilities = ["read"]
}
`,
	}))
	require.NoError(t, client.CreateAppRole(ctx, "cloud-secrets", "cloud-secrets"))

	roleID, err := client.ReadAppRoleRoleID(ctx, "cloud-secrets")
	require.NoError(t, err)
	require.Equal(t, "role-id", roleID)

	secretID, err := client.CreateAppRoleSecretID(ctx, "cloud-secrets")
	require.NoError(t, err)
	require.Equal(t, "secret-id", secretID)

	require.Equal(t, 1, requests["GET /v1/prod/metadata"])
	require.Equal(t, 1, requests["GET /v1/sys/auth"])
	require.Equal(t, 1, requests["POST /v1/sys/auth/approle"])
	require.Equal(t, 1, requests["PUT /v1/sys/policies/acl/cloud-secrets"])
	require.Equal(t, 1, requests["PUT /v1/auth/approle/role/cloud-secrets"])
	require.Equal(t, 1, requests["GET /v1/auth/approle/role/cloud-secrets/role-id"])
	require.Equal(t, 1, requests["PUT /v1/auth/approle/role/cloud-secrets/secret-id"])
}

func TestHttpClientCreateACLPolicyRequiresRules(t *testing.T) {
	ctx := context.Background()
	client := &HttpClient{}

	err := client.CreateACLPolicy(ctx, CreateACLPolicyRequest{Name: "cloud-secrets"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "policy rules are empty")
}

func TestHttpClientReportsAppRoleMissing(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "setup-token", r.Header.Get("X-Vault-Token"))

		switch r.Method + " " + r.URL.Path {
		case "GET /v1/prod/metadata":
			writeJSON(t, w, map[string]interface{}{
				"data": map[string]interface{}{
					"keys": []interface{}{},
				},
			})
		case "GET /v1/sys/auth":
			writeJSON(t, w, map[string]interface{}{
				"data": map[string]interface{}{},
			})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	addr, err := url.Parse(server.URL)
	require.NoError(t, err)

	client, err := NewHttpClient(ctx, "prod", *addr, AuthConfig{
		Token: "setup-token",
	})
	require.NoError(t, err)

	enabled, err := client.AppRoleAuthEnabled(ctx)
	require.NoError(t, err)
	require.False(t, enabled)
}

func writeJSON(t *testing.T, w http.ResponseWriter, body interface{}) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	require.NoError(t, json.NewEncoder(w).Encode(body))
}
