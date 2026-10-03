package certificateapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClient_Reads(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		path string
		body string
	}{
		{"certificates", "/v1/certificate", `{"certificates":[{"id":"cert-id","description":"TLS"}]}`},
		{"versions", "/v1/certificate/cert-id/versions", `{"versions":[{"id":"2","enabled":true,"status":"VERSION_STATUS_READY"}],"nextPageToken":"next"}`},
		{"material", "/v1/certificate/cert-id/version/2:private_key", `{"privateKey":"PEM key","version":{"id":2,"serverCertificate":"PEM leaf","intermediateCertificate":"PEM chain"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, tt.path, r.URL.Path)
				assert.Equal(t, "Bearer shared-token", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Accept"))
				body := tt.body
				switch tt.name {
				case "certificates":
					assert.Equal(t, "project & id", r.URL.Query().Get("projectId"))
					assert.Equal(t, "CERTIFICATE_STATE_ACTIVE", r.URL.Query().Get("state"))
				case "versions":
					assert.Equal(t, "100", r.URL.Query().Get("pageSize"))
					if calls == 2 {
						assert.Equal(t, "next", r.URL.Query().Get("pageToken"))
						body = `{"versions":[{"id":3,"enabled":true}]}`
					}
				}
				_, err := w.Write([]byte(body))
				assert.NoError(t, err)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, server.Client(), func(context.Context) (string, error) { return "shared-token", nil })
			require.NoError(t, err)
			switch tt.name {
			case "certificates":
				got, readErr := client.ListCertificates(t.Context(), "project & id")
				require.NoError(t, readErr)
				assert.Equal(t, []Certificate{{ID: "cert-id", Description: "TLS"}}, got)
			case "versions":
				got, readErr := client.ListVersions(t.Context(), "cert-id")
				require.NoError(t, readErr)
				require.Len(t, got, 2)
				assert.Equal(t, "2", got[0].ID.String())
				assert.Equal(t, "3", got[1].ID.String())
			case "material":
				got, readErr := client.GetMaterial(t.Context(), "cert-id", "2")
				require.NoError(t, readErr)
				assert.Equal(t, "2", got.Version.ID.String())
				assert.Equal(t, "PEM key", got.PrivateKey)
				assert.Equal(t, "PEM leaf", got.Version.ServerCertificate)
				assert.Equal(t, "PEM chain", got.Version.IntermediateCertificate)
			}
		})
	}
}

func TestClient_Errors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, body, want string
		status           int
		tokenErr         error
	}{
		{name: "forbidden", status: http.StatusForbidden, body: "sensitive key", want: "HTTP 403"},
		{name: "invalid JSON", status: http.StatusOK, body: "broken", want: "decode Certificate Manager response"},
		{name: "IAM failure", tokenErr: errors.New("IAM unavailable"), want: "IAM unavailable"},
		{name: "repeated page", status: http.StatusOK, body: `{"nextPageToken":"repeat"}`, want: "repeated a pagination token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Nil(t, tt.tokenErr, "no HTTP request on IAM failure")
				w.WriteHeader(tt.status)
				_, err := w.Write([]byte(tt.body))
				assert.NoError(t, err)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, server.Client(), func(context.Context) (string, error) { return "token", tt.tokenErr })
			require.NoError(t, err)
			_, err = client.ListVersions(t.Context(), "cert-id")
			require.ErrorContains(t, err, tt.want)
			assert.NotContains(t, err.Error(), "sensitive key")
		})
	}
}
