package cloudru

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v2 "github.com/cloudru-tech/secret-manager-sdk/api/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru/certificateapi"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"go.uber.org/mock/gomock"
)

func certificateFixture(t *testing.T, domains []string, cn string) certificateapi.Material {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	require.NoError(t, err)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: cn}, DNSNames: domains,
		NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return certificateapi.Material{
		PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})),
		Version: certificateapi.Version{ID: "7", CertificateID: "cert-id", Enabled: true, Status: "VERSION_STATUS_READY",
			ExpiresAt:               ca.NotAfter.Format(time.RFC3339),
			ServerCertificate:       string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})),
			IntermediateCertificate: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))},
	}
}

func TestMapCertificate(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name            string
		domains         []string
		cn, base, label string
	}{
		{"SAN", []string{"www.domain.com", "domain.com", "domain.com"}, "ignored.example", "domain.com", "domain.com,www.domain.com"},
		{"wildcard", []string{"*.domain.com", "www.domain.com"}, "ignored.example", "domain.com", "*.domain.com,www.domain.com"},
		{"CN fallback", nil, "domain.com", "domain.com", "domain.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			material := certificateFixture(t, tt.domains, tt.cn)
			material.Version.ExpiresAt = "2099-01-01T00:00:00Z"
			secrets, values, err := mapCertificate(certificateapi.Certificate{ID: "cert-id", Description: "TLS"}, material, time.Now())
			require.NoError(t, err)
			require.Len(t, secrets, 2)
			require.Len(t, values, 2)
			for _, part := range []struct{ suffix, label string }{{"crt", "certificate"}, {"pk", "private_key"}} {
				path := "certs-" + tt.base + "-" + part.suffix
				secret, ok := secrets[path]
				require.True(t, ok)
				assert.Equal(t, path, secret.Path)
				assert.Equal(t, "cert-id-7", secret.VersionID)
				assert.Equal(t, "TLS", secret.Description)
				assert.Equal(t, map[string]string{"type": "certificate", "certificate.domains": tt.label,
					"certificate.expires_at": "2030-01-02T03:04:05Z", "certificate.part": part.label}, secret.Labels)
			}
			assert.Equal(t, material.Version.ServerCertificate+material.Version.IntermediateCertificate, string(values["certs-"+tt.base+"-crt"]))
			assert.Equal(t, material.PrivateKey, string(values["certs-"+tt.base+"-pk"]))
		})
	}
}

func TestMapCertificateFailures(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		change func(*certificateapi.Material)
		want   string
	}{
		{"leaf", func(m *certificateapi.Material) { m.Version.ServerCertificate = "invalid" }, "invalid certificate PEM"},
		{"chain", func(m *certificateapi.Material) { m.Version.IntermediateCertificate = "invalid" }, "invalid certificate PEM"},
		{"key", func(m *certificateapi.Material) { m.PrivateKey = "invalid" }, "private key pair"},
		{"mismatched key", func(m *certificateapi.Material) {
			m.PrivateKey = certificateFixture(t, []string{"domain.com"}, "").PrivateKey
		}, "private key pair"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			material := certificateFixture(t, []string{"domain.com"}, "")
			tt.change(&material)
			secrets, values, err := mapCertificate(certificateapi.Certificate{ID: "cert-id"}, material, time.Now())
			require.ErrorContains(t, err, tt.want)
			assert.Nil(t, secrets)
			assert.Nil(t, values)
		})
	}
}

func TestActiveCertificateVersion(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		versions []certificateapi.Version
		want     string
		wantErr  bool
	}{
		{"latest usable", []certificateapi.Version{
			{ID: "5", Enabled: true, Status: "VERSION_STATUS_READY", ExpiresAt: "2030-01-01T00:00:00Z"},
			{ID: "7", Enabled: true, Status: "VERSION_STATUS_WARNING", ExpiresAt: "2030-01-01T00:00:00Z"},
			{ID: "8", Enabled: false, Status: "VERSION_STATUS_READY"},
			{ID: "9", Enabled: true, Status: "VERSION_STATUS_PROCESSING"},
			{ID: "10", Enabled: true, Status: "VERSION_STATUS_READY", ExpiresAt: "2020-01-01T00:00:00Z"},
		}, "7", false},
		{"none", nil, "", false},
		{"invalid ID", []certificateapi.Version{{ID: "0", Enabled: true, Status: "VERSION_STATUS_READY", ExpiresAt: "2030-01-01T00:00:00Z"}}, "", true},
		{"invalid expiry", []certificateapi.Version{{ID: "1", Enabled: true, Status: "VERSION_STATUS_READY", ExpiresAt: "bad"}}, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := activeCertificateVersion(tt.versions, time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.wantErr, err != nil)
		})
	}
}

func TestProviderCertificateSource(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                                        string
		enabled, broken, collision, versionMismatch bool
	}{
		{name: "disabled"}, {name: "enabled", enabled: true}, {name: "invalid key", enabled: true, broken: true},
		{name: "name collision", enabled: true, collision: true}, {name: "wrong version", enabled: true, versionMismatch: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			material := certificateFixture(t, []string{"domain.com"}, "")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.True(t, tt.enabled, "disabled source must not make HTTP requests")
				assert.Equal(t, "Bearer token", r.Header.Get("Authorization"))
				var response any
				switch {
				case r.URL.Path == "/v1/certificate":
					assert.Equal(t, "project-id", r.URL.Query().Get("projectId"))
					response = map[string]any{"certificates": []certificateapi.Certificate{{ID: "cert-id"}}}
				case strings.HasSuffix(r.URL.Path, "/versions"):
					response = map[string]any{"versions": []certificateapi.Version{material.Version}}
				default:
					assert.Equal(t, "/v1/certificate/cert-id/version/7:private_key", r.URL.Path)
					copyMaterial := material
					if tt.broken {
						copyMaterial.PrivateKey = "broken"
					}
					if tt.versionMismatch {
						copyMaterial.Version.ID = "8"
					}
					response = copyMaterial
				}
				assert.NoError(t, json.NewEncoder(w).Encode(response))
			}))
			defer server.Close()
			client, err := certificateapi.NewClient(server.URL, server.Client(), func(context.Context) (string, error) { return "token", nil })
			require.NoError(t, err)
			secretClient := NewMocksecretService(gomock.NewController(t))
			ordinary := &v2.Secret{Path: "password", Versions: []*v2.SecretVersion{{Id: 1, State: v2.VersionState_ENABLED}}}
			if tt.collision {
				ordinary.Path = "certs-domain.com-crt"
			}
			secretClient.EXPECT().Search(gomock.Any(), &v2.SearchSecretRequest{ProjectId: "project-id", Depth: -1}).Return(
				&v2.SearchSecretResponse{Secrets: []*v2.Secret{ordinary}}, nil)
			p := &Provider{cfg: Config{ProjectID: "project-id"}, secretManager: secretClient, certificateManager: client}
			p.cfg.CertificateManager.Enabled = tt.enabled
			secrets, err := p.ListSecrets(t.Context())
			if tt.broken || tt.collision || tt.versionMismatch {
				require.Error(t, err)
				assert.Nil(t, secrets)
				assert.Empty(t, p.certificatePayloads)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"type": "secret"}, secrets[ordinary.Path].Labels)
			if !tt.enabled {
				assert.Len(t, secrets, 1)
				return
			}
			assert.Len(t, secrets, 3)
			assert.Equal(t, secrets["certs-domain.com-crt"].VersionID, secrets["certs-domain.com-pk"].VersionID)
			key, err := p.GetSecretPayload(t.Context(), "certs-domain.com-pk")
			require.NoError(t, err)
			assert.Equal(t, []byte(material.PrivateKey), key)
			key[0] = 'x'
			again, err := p.GetSecretPayload(t.Context(), "certs-domain.com-pk")
			require.NoError(t, err)
			assert.Equal(t, []byte(material.PrivateKey), again)
		})
	}
}

func TestProviderCertificateCollisionAndAtomicCache(t *testing.T) {
	t.Parallel()
	material := certificateFixture(t, []string{"domain.com"}, "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response any
		switch {
		case r.URL.Path == "/v1/certificate":
			response = map[string]any{"certificates": []certificateapi.Certificate{{ID: "cert-id"}, {ID: "another"}}}
		case strings.HasSuffix(r.URL.Path, "/versions"):
			response = map[string]any{"versions": []certificateapi.Version{material.Version}}
		default:
			material.Version.CertificateID = ""
			response = material
		}
		assert.NoError(t, json.NewEncoder(w).Encode(response))
	}))
	defer server.Close()
	client, err := certificateapi.NewClient(server.URL, server.Client(), func(context.Context) (string, error) { return "token", nil })
	require.NoError(t, err)
	old := map[string][]byte{"previous": []byte("old pair")}
	p := &Provider{certificateManager: client, certificatePayloads: old}
	err = p.addCertificates(t.Context(), map[string]contracts.Secret{})
	require.ErrorContains(t, err, "collision")
	assert.Equal(t, old, p.certificatePayloads)
}
