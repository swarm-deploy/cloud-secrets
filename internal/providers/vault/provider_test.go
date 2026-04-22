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

	require.Len(t, secrets, 6)
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/db/value",
		VersionID: "2",
	}, secrets["cloud-secrets/db/value"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/json/password",
		VersionID: "3",
	}, secrets["cloud-secrets/json/password"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/json/username",
		VersionID: "3",
	}, secrets["cloud-secrets/json/username"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/nested/api/value",
		VersionID: "7",
	}, secrets["cloud-secrets/nested/api/value"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/mixed/password",
		VersionID: "5",
	}, secrets["cloud-secrets/mixed/password"])
	assert.Equal(t, contracts.Secret{
		Path:      "cloud-secrets/mixed/value",
		VersionID: "5",
	}, secrets["cloud-secrets/mixed/value"])
	assert.NotContains(t, secrets, "cloud-secrets/db")
	assert.NotContains(t, secrets, "cloud-secrets/nested/api")
}

func TestProviderGetSecretPayload(t *testing.T) {
	server := newVaultServer(t)
	t.Cleanup(server.Close)

	provider := newTestProvider(t, server.URL)

	t.Run("value key", func(t *testing.T) {
		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/db/value")
		require.NoError(t, err)
		assert.Equal(t, []byte("postgres://dsn"), payload)
	})

	t.Run("json key payload", func(t *testing.T) {
		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/json/password")
		require.NoError(t, err)
		assert.Equal(t, []byte("secret"), payload)
	})

	t.Run("multi field payload with value key", func(t *testing.T) {
		payload, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/mixed/password")
		require.NoError(t, err)
		assert.Equal(t, []byte("secret"), payload)
	})

	t.Run("base path is invalid in always multi-key mode", func(t *testing.T) {
		_, err := provider.GetSecretPayload(context.Background(), "cloud-secrets/db")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "key")
	})
}

func TestConfigValidate_NormalizesTokenFromFileContent(t *testing.T) {
	parsedAddress, err := url.Parse("https://vault.local")
	require.NoError(t, err)

	var token Token
	err = token.UnmarshalText([]byte(" \nroot-token\r\n"))
	require.NoError(t, err)

	cfg := Config{
		Address:   *parsedAddress,
		Token:     token,
		MountPath: "/secret/",
		Prefix:    "/cloud-secrets/",
	}

	err = cfg.Validate()
	require.NoError(t, err)
	assert.Equal(t, "root-token", cfg.Token.Value)
	assert.Equal(t, "secret", cfg.MountPath)
	assert.Equal(t, "cloud-secrets", cfg.Prefix)
}

func TestConfigValidate_RejectsEmptyToken(t *testing.T) {
	parsedAddress, err := url.Parse("https://vault.local")
	require.NoError(t, err)

	var token Token
	err = token.UnmarshalText([]byte("  \n\t"))
	require.NoError(t, err)

	cfg := Config{
		Address:   *parsedAddress,
		Token:     token,
		MountPath: "secret",
	}

	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VAULT_TOKEN is required")
}

func newTestProvider(t *testing.T, address string) *Provider {
	t.Helper()

	parsedAddress, err := url.Parse(address)
	require.NoError(t, err)

	provider, err := NewProvider(Config{
		Address:   *parsedAddress,
		Token:     Token{Value: "token"},
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
				"keys": []interface{}{"db", "json", "mixed", "nested/"},
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
		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/metadata/cloud-secrets/mixed":
			writeVaultData(w, map[string]interface{}{"current_version": 5})

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
		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/data/cloud-secrets/nested/api":
			writeVaultData(w, map[string]interface{}{
				"data": map[string]interface{}{
					"value": "nested-token",
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/secret/data/cloud-secrets/mixed":
			writeVaultData(w, map[string]interface{}{
				"data": map[string]interface{}{
					"value":    "postgres://dsn",
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
