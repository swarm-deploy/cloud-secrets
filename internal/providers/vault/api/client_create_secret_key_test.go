package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	vaultapi "github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testVaultDocument struct {
	data    map[string]interface{}
	version int
}

type testVaultWrite struct {
	data        map[string]interface{}
	valueDigest [sha256.Size]byte
	cas         int
}

func TestHttpClient_CreateSecretKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		documents    []*testVaultDocument
		writeStatus  []int
		wantWrites   []testVaultWrite
		wantErr      error
	}{
		{
			name:        "creates new document with CAS zero",
			documents:   []*testVaultDocument{nil},
			writeStatus: []int{http.StatusOK},
			wantWrites: []testVaultWrite{
				{data: map[string]interface{}{}, valueDigest: sha256.Sum256([]byte("sensitive")), cas: 0},
			},
		},
		{
			name: "adds key without losing existing data",
			documents: []*testVaultDocument{
				{data: map[string]interface{}{"username": "app"}, version: 7},
			},
			writeStatus: []int{http.StatusOK},
			wantWrites: []testVaultWrite{
				{
					data:        map[string]interface{}{"username": "app"},
					valueDigest: sha256.Sum256([]byte("sensitive")),
					cas:         7,
				},
			},
		},
		{
			name: "rejects duplicate key",
			documents: []*testVaultDocument{
				{data: map[string]interface{}{"password": "existing"}, version: 4},
			},
			wantErr: ErrSecretKeyExists,
		},
		{
			name: "retries CAS conflict with fresh document",
			documents: []*testVaultDocument{
				{data: map[string]interface{}{"username": "app"}, version: 1},
				{data: map[string]interface{}{"username": "app", "token": "concurrent"}, version: 2},
			},
			writeStatus: []int{http.StatusBadRequest, http.StatusOK},
			wantWrites: []testVaultWrite{
				{
					data:        map[string]interface{}{"username": "app"},
					valueDigest: sha256.Sum256([]byte("sensitive")),
					cas:         1,
				},
				{
					data: map[string]interface{}{
						"username": "app",
						"token":    "concurrent",
					},
					valueDigest: sha256.Sum256([]byte("sensitive")),
					cas:         2,
				},
			},
		},
		{
			name: "returns conflict after retry limit",
			documents: []*testVaultDocument{
				{data: map[string]interface{}{"version": "one"}, version: 1},
				{data: map[string]interface{}{"version": "two"}, version: 2},
				{data: map[string]interface{}{"version": "three"}, version: 3},
			},
			writeStatus: []int{http.StatusBadRequest, http.StatusBadRequest, http.StatusBadRequest},
			wantWrites: []testVaultWrite{
				{
					data:        map[string]interface{}{"version": "one"},
					valueDigest: sha256.Sum256([]byte("sensitive")),
					cas:         1,
				},
				{
					data:        map[string]interface{}{"version": "two"},
					valueDigest: sha256.Sum256([]byte("sensitive")),
					cas:         2,
				},
				{
					data:        map[string]interface{}{"version": "three"},
					valueDigest: sha256.Sum256([]byte("sensitive")),
					cas:         3,
				},
			},
			wantErr: ErrCASConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			getIndex := 0
			writeIndex := 0
			var writes []testVaultWrite
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				assert.Equal(t, "/v1/secret/data/prod/db", request.URL.Path)

				switch request.Method {
				case http.MethodGet:
					document := tt.documents[getIndex]
					getIndex++
					if document == nil {
						writer.WriteHeader(http.StatusNotFound)
						require.NoError(t, json.NewEncoder(writer).Encode(map[string]interface{}{"errors": []string{}}))
						return
					}

					require.NoError(t, json.NewEncoder(writer).Encode(map[string]interface{}{
						"data": map[string]interface{}{
							"data": document.data,
							"metadata": map[string]interface{}{
								"created_time":  "2026-10-29T00:00:00Z",
								"deletion_time": "",
								"destroyed":     false,
								"version":       document.version,
							},
						},
					}))
				case http.MethodPut:
					var body struct {
						Data    map[string]interface{} `json:"data"`
						Options struct {
							CAS int `json:"cas"`
						} `json:"options"`
					}
					require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
					value, valueIsString := body.Data["password"].(string)
					assert.True(t, valueIsString, "Vault payload must be stored as text")
					delete(body.Data, "password")
					writes = append(writes, testVaultWrite{
						data:        body.Data,
						valueDigest: sha256.Sum256([]byte(value)),
						cas:         body.Options.CAS,
					})

					statusCode := tt.writeStatus[writeIndex]
					writeIndex++
					if statusCode == http.StatusBadRequest {
						writer.WriteHeader(statusCode)
						require.NoError(t, json.NewEncoder(writer).Encode(map[string]interface{}{
							"errors": []string{"check-and-set parameter did not match current version"},
						}))
						return
					}

					writer.WriteHeader(statusCode)
					require.NoError(t, json.NewEncoder(writer).Encode(map[string]interface{}{
						"data": map[string]interface{}{
							"created_time":  "2026-10-29T00:00:00Z",
							"deletion_time": "",
							"destroyed":     false,
							"version":       writeIndex,
						},
					}))
				default:
					t.Errorf("unexpected HTTP method %s", request.Method)
					writer.WriteHeader(http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()

			config := vaultapi.DefaultConfig()
			config.Address = server.URL
			vaultHTTPClient, err := vaultapi.NewClient(config)
			require.NoError(t, err)

			client := &HttpClient{
				client:    vaultHTTPClient,
				kv:        vaultHTTPClient.KVv2("secret"),
				mountPath: "secret",
			}
			err = client.CreateSecretKey(context.Background(), "prod/db", "password", []byte("sensitive"))

			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			}
			assert.Equal(t, tt.wantWrites, writes)
		})
	}
}
