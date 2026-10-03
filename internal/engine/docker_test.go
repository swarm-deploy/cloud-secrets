package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/moby/moby/api/types/swarm"
	dock "github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
)

func TestDockerSecretLabels(t *testing.T) {
	t.Parallel()
	for _, version := range []bool{false, true} {
		t.Run(map[bool]string{false: "parent", true: "version"}[version], func(t *testing.T) {
			t.Parallel()
			providerLabels := map[string]string{
				"type": "certificate", "certificate.part": "private_key", "certificate.domains": "domain.com",
				"certificate.expires_at": "2030-01-02T03:04:05Z",
				managedLabel:             "false", "logical_path": "override", "external_path": "override",
				"external_version_id": "override", "description": "override", "cloud-secrets.future": "override",
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				var spec swarm.SecretSpec
				require.NoError(t, json.NewDecoder(r.Body).Decode(&spec))
				expectedName := "certs-domain.com-pk"
				if version {
					expectedName += "-version-2"
				}
				assert.Equal(t, expectedName, spec.Name)
				assert.Equal(t, []byte("PEM key"), spec.Data)
				assert.Equal(t, map[string]string{
					"type": "certificate", "certificate.part": "private_key", "certificate.domains": "domain.com",
					"certificate.expires_at": "2030-01-02T03:04:05Z", managedLabel: "true",
					"logical_path": "certs-domain.com-pk", "external_path": "external-path",
					"external_version_id": "version-2", "description": "TLS",
				}, spec.Labels)
				w.WriteHeader(http.StatusCreated)
				_, err := w.Write([]byte(`{"ID":"created-id"}`))
				assert.NoError(t, err)
			}))
			defer server.Close()
			client, err := dock.New(dock.WithHost(server.URL), dock.WithHTTPClient(server.Client()), dock.WithAPIVersion("1.55"))
			require.NoError(t, err)
			defer func() { assert.NoError(t, client.Close()) }()
			engine := NewDockerClient(client, metrics.NopDocker{})
			if version {
				got, createErr := engine.CreateSecretVersion(t.Context(), ExistingSecret{Path: "certs-domain.com-pk", ExternalPath: "external-path"},
					CreatingSecretVersion{Path: "certs-domain.com-pk-version-2", Description: "TLS", ExternalID: "version-2", Value: []byte("PEM key"), Labels: providerLabels})
				require.NoError(t, createErr)
				assert.Equal(t, "created-id", got.ID)
			} else {
				require.NoError(t, engine.CreateSecret(t.Context(), CreatingSecret{Path: "certs-domain.com-pk", Description: "TLS",
					ExternalPath: "external-path", ExternalVersionID: "version-2", Value: []byte("PEM key"), Labels: providerLabels}))
			}
			assert.Equal(t, "false", providerLabels[managedLabel], "input metadata must remain unchanged")
		})
	}
}
