package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHttpClientListKeys(t *testing.T) {
	server := newVaultServer(t)
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)

	keys, err := client.ListKeys(context.Background(), "cloud-secrets")
	require.NoError(t, err)

	assert.Equal(t, []string{"db", "json", "nested/"}, keys)
}

func TestHttpClientGet(t *testing.T) {
	server := newVaultServer(t)
	t.Cleanup(server.Close)

	client := newTestClient(t, server.URL)

	t.Run("maps payload and version", func(t *testing.T) {
		secret, err := client.Get(context.Background(), "cloud-secrets/json", "password")
		require.NoError(t, err)

		assert.Equal(t, "3", secret.VersionID)
		assert.Equal(t, []string{"password", "username"}, secret.Keys)
		assert.Equal(t, []byte("secret"), secret.Payload)
	})

	t.Run("returns keys without payload when key is empty", func(t *testing.T) {
		secret, err := client.Get(context.Background(), "cloud-secrets/db", "")
		require.NoError(t, err)

		assert.Equal(t, "2", secret.VersionID)
		assert.Equal(t, []string{"value"}, secret.Keys)
		assert.Nil(t, secret.Payload)
	})
}

func newTestClient(t *testing.T, address string) *HttpClient {
	t.Helper()

	cfg := vaultapi.DefaultConfig()
	cfg.Address = address

	client, err := vaultapi.NewClient(cfg)
	require.NoError(t, err)

	client.SetToken("token")

	return NewHttpClient(client, "secret")
}

func newVaultServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isListRequest(r, "/v1/secret/metadata/cloud-secrets"):
			writeVaultData(w, map[string]interface{}{
				"keys": []interface{}{"db", "json", "nested/"},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/data/cloud-secrets/db":
			writeVaultData(w, map[string]interface{}{
				"data": map[string]interface{}{
					"value": "postgres://dsn",
				},
				"metadata": map[string]interface{}{
					"version": 2,
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/data/cloud-secrets/json":
			writeVaultData(w, map[string]interface{}{
				"data": map[string]interface{}{
					"username": "svc",
					"password": "secret",
				},
				"metadata": map[string]interface{}{
					"version": 3,
				},
			})

		default:
			http.NotFound(w, r)
		}
	}))
}

func isListRequest(r *http.Request, path string) bool {
	if r.URL.Path != path {
		return false
	}

	return r.Method == "LIST" || (r.Method == http.MethodGet && r.URL.Query().Get("list") == "true")
}

func writeVaultData(w http.ResponseWriter, data map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data": data,
	})
}
