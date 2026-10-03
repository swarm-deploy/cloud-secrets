package sync

import (
	"errors"
	"testing"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
	"go.uber.org/mock/gomock"
)

func TestCertificatePairRotation(t *testing.T) {
	t.Parallel()
	for _, failKey := range []bool{false, true} {
		t.Run(map[bool]string{false: "one rollout for both parts", true: "retrieval failure preserves previous pair"}[failKey], func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			engineClient := engine.NewMockClient(ctrl)
			provider := contracts.NewMockProvider(ctrl)
			existing := map[string]*engine.ExistingSecret{}
			external := map[string]contracts.Secret{}
			originalRefs := []*swarm.SecretReference{}
			updatedRefs := []*swarm.SecretReference{}
			for _, part := range []struct{ suffix, label string }{{"crt", "certificate"}, {"pk", "private_key"}} {
				path := "certs-domain.com-" + part.suffix
				labels := map[string]string{"type": "certificate", "certificate.part": part.label}
				secret := engine.ExistingSecret{ID: path + "-old", Path: path, ExternalPath: "external/" + part.suffix, Managed: true,
					Versions: []engine.ExistingSecretVersion{{ID: path + "-old", ExternalID: "cert-id-1"}}}
				existing[path] = &secret
				external[path] = contracts.Secret{Path: path, FullPath: secret.ExternalPath, VersionID: "cert-id-2", Labels: labels}
				originalRefs = append(originalRefs, engine.NewSecretRef(path, path, secret.ID))
				updatedRefs = append(updatedRefs, engine.NewSecretRef(path, path+"-cert-id-2", path+"-new"))
				if failKey && part.suffix == "pk" {
					provider.EXPECT().GetSecretPayload(gomock.Any(), path).Return(nil, errors.New("private key unavailable"))
					continue
				}
				get := provider.EXPECT().GetSecretPayload(gomock.Any(), path).Return([]byte(part.suffix), nil)
				create := engineClient.EXPECT().CreateSecretVersion(gomock.Any(), secret, engine.CreatingSecretVersion{
					Path: path + "-cert-id-2", ExternalID: "cert-id-2", Value: []byte(part.suffix), Labels: labels,
				}).Return(engine.CreatedSecretVersion{ID: path + "-new", Name: path + "-cert-id-2"}, nil)
				if failKey {
					// Map order is intentionally unspecified; the certificate may be prepared before the key fails.
					get.AnyTimes()
					create.AnyTimes()
					continue
				}
				engineClient.EXPECT().RemoveSecret(gomock.Any(), secret.ID).Return(nil)
				engineClient.EXPECT().CreateSecret(gomock.Any(), engine.CreatingSecret{
					Path: path, ExternalPath: secret.ExternalPath, ExternalVersionID: "cert-id-2", Value: []byte(part.suffix), Labels: labels,
				}).Return(nil)
			}
			engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{newService("service", "web", originalRefs...)}, nil)
			engineClient.EXPECT().MapSecrets(gomock.Any()).Return(existing, nil)
			provider.EXPECT().ListSecrets(gomock.Any()).Return(external, nil)
			if !failKey {
				engineClient.EXPECT().UpdateService(gomock.Any(), newService("service", "web", updatedRefs...)).Return(nil).Times(1)
			}
			synchronizer := NewSynchronizer(engineClient, provider, metrics.NewGroup(metrics.CreateGroupParams{Namespace: "certificate_test"}).Secrets, false, secretname.FolderDelimiterDash)
			result, err := synchronizer.Sync(t.Context())
			if failKey {
				require.ErrorContains(t, err, "private key unavailable")
			} else {
				require.NoError(t, err)
				assert.Equal(t, 2, result.Updated)
				assert.Equal(t, 2, result.RemovedSecrets)
			}
		})
	}
}

func TestCertificateLabelsOnInitialCreation(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	engineClient := engine.NewMockClient(ctrl)
	provider := contracts.NewMockProvider(ctrl)
	labels := map[string]string{"type": "certificate", "certificate.part": "certificate"}
	path := "certs-domain.com-crt"
	engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{}, nil)
	engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{}, nil)
	provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{
		path: {Path: path, FullPath: "external", VersionID: "cert-id-1", Labels: labels},
	}, nil)
	provider.EXPECT().GetSecretPayload(gomock.Any(), path).Return([]byte("PEM"), nil)
	engineClient.EXPECT().CreateSecret(gomock.Any(), engine.CreatingSecret{Path: path, ExternalPath: "external",
		ExternalVersionID: "cert-id-1", Value: []byte("PEM"), Labels: labels}).Return(nil)
	s := NewSynchronizer(engineClient, provider, metrics.NewGroup(metrics.CreateGroupParams{Namespace: "certificate_test"}).Secrets, false, secretname.FolderDelimiterDash)
	result, err := s.Sync(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, result.Created)
}

func TestCertificateNormalizedNameCollision(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, path string
		delimiter  secretname.FolderDelimiter
		wantErr    bool
	}{
		{"dash collision", "certs/domain.com/crt", secretname.FolderDelimiterDash, true},
		{"underscore remains distinct", "certs/domain.com/crt", secretname.FolderDelimiterUnderscore, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			provider := contracts.NewMockProvider(gomock.NewController(t))
			provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{
				"cert":     {Path: "certs-domain.com-crt", FullPath: "certificate-manager/cert/crt"},
				"ordinary": {Path: tt.path, FullPath: tt.path},
			}, nil)
			s := &Synchronizer{secretProvider: provider, folderDelimiter: tt.delimiter}
			payload := &syncPayload{}
			err := s.loadExternalState(t.Context(), payload)
			if tt.wantErr {
				require.ErrorContains(t, err, "same Docker secret")
				assert.Nil(t, payload.externalSecrets)
			} else {
				require.NoError(t, err)
				assert.Len(t, payload.externalSecrets, 2)
			}
		})
	}
}
