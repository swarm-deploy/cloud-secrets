package cloudru

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru/certificateapi"
)

func TestProvider_Definition(t *testing.T) {
	t.Parallel()

	provider := &Provider{cfg: Config{ProjectID: "project-id"}}

	definition := provider.Definition()

	assert.Equal(t, "Cloud.ru", definition.Name)
	assert.Equal(t, "https://cloud.ru/docs/scsm/ug/index", definition.Links.Doc)
	assert.Equal(
		t,
		"https://console.cloud.ru/spa/secret-manager/list?projectId=project-id",
		definition.Links.Manager,
	)
}

func TestProviderResolveCertificateEndpoint(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                       string
		enabled                    bool
		discovered, override, want string
	}{
		{name: "disabled"},
		{name: "discovery", enabled: true, discovered: "https://cm.example", want: "https://cm.example"},
		{name: "public fallback", enabled: true, want: certificateapi.DefaultAddress},
		{name: "explicit override", enabled: true, discovered: "https://cm.example", override: "https://custom.example", want: "https://custom.example"},
		{name: "explicit public endpoint", enabled: true, discovered: "https://cm.example", override: certificateapi.DefaultAddress, want: certificateapi.DefaultAddress},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				endpoints := EndpointsResponse{Endpoints: []Endpoint{{ID: "iam", Address: "iam.example:443"}, {ID: "secret-manager", Address: "sm.example:443"}}}
				if tt.discovered != "" {
					endpoints.Endpoints = append(endpoints.Endpoints, Endpoint{ID: "certificate-manager", Address: tt.discovered})
				}
				assert.NoError(t, json.NewEncoder(w).Encode(endpoints))
			}))
			defer server.Close()
			p := &Provider{cfg: Config{DiscoveryURL: server.URL}}
			p.cfg.CertificateManager.Enabled = tt.enabled
			p.cfg.CertificateManager.Address = tt.override
			require.NoError(t, p.resolveEndpoints(t.Context()))
			assert.Equal(t, tt.want, p.cfg.CertificateManager.Address)
		})
	}
}
