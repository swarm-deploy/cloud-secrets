package vault //nolint:testpackage // Tests validate provider behavior using package internals.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
)

func TestProviderListSecrets(t *testing.T) {
	server := newVaultServer(t)
	t.Cleanup(server.Close)

	provider := newTestProvider(t, server.URL)

	secrets, err := provider.ListSecrets(context.Background())
	require.NoError(t, err)

	require.Len(t, secrets, 3)
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/db",
		VersionID: "2",
	}, secrets["cloud-secrets/db"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/nested/api",
		VersionID: "7",
	}, secrets["cloud-secrets/nested/api"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/json",
		VersionID: "3",
	}, secrets["cloud-secrets/json"])
}

func TestProviderGetSecretPayload(t *testing.T) {
	server := newVaultServer(t)
	t.Cleanup(server.Close)

	provider := newTestProvider(t, server.URL)

	t.Run("value key", func(t *testing.T) {
		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/db")
		require.NoError(t, err)
		assert.Equal(t, []byte("postgres://dsn"), payload)
	})

	t.Run("json payload", func(t *testing.T) {
		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/json")
		require.NoError(t, err)

		var data map[string]string
		require.NoError(t, json.Unmarshal(payload, &data))
		assert.Equal(t, map[string]string{
			"password": "secret",
			"username": "svc",
		}, data)
	})
}

func newTestProvider(t *testing.T, address string) *Provider {
	t.Helper()

	parsedAddress, err := url.Parse(address)
	require.NoError(t, err)

	provider, err := NewProvider(context.Background(), Config{
		Address:   *parsedAddress,
		Token:     "token",
		MountPath: "secret",
		Prefix:    "cloud-secrets",
	})
	require.NoError(t, err)

	return provider
}

func newVaultServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case isListRequest(r, "/v1/secret/metadata/cloud-secrets"):
			writeVaultData(w, map[string]interface{}{
				"keys": []interface{}{"db", "json", "nested/"},
			})
		case isListRequest(r, "/v1/secret/metadata/cloud-secrets/nested"):
			writeVaultData(w, map[string]interface{}{
				"keys": []interface{}{"api"},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/metadata/cloud-secrets/db":
			writeVaultData(w, map[string]interface{}{"current_version": 2})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/metadata/cloud-secrets/json":
			writeVaultData(w, map[string]interface{}{"current_version": 3})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/metadata/cloud-secrets/nested/api":
			writeVaultData(w, map[string]interface{}{"current_version": 7})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/data/cloud-secrets/db":
			writeVaultData(w, map[string]interface{}{
				"data": map[string]interface{}{
					"value": "postgres://dsn",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/data/cloud-secrets/json":
			writeVaultData(w, map[string]interface{}{
				"data": map[string]interface{}{
					"username": "svc",
					"password": "secret",
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
