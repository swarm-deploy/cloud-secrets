package sync

import (
	"context"
	"testing"

	"github.com/moby/moby/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/swarm-deploy/cloud-secrets/internal/engine"
	"github.com/swarm-deploy/cloud-secrets/internal/metrics"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
	"go.uber.org/mock/gomock"
)

func TestSynchronizer_Sync(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(*engine.MockClient, *contracts.MockProvider)
		want  Result
	}{
		{
			name: "create missing secret",
			setup: func(engineClient *engine.MockClient, provider *contracts.MockProvider) {
				engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{}, nil)
				engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{}, nil)
				provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{
					"prod/db/password": {
						Path:      "prod/db/password",
						FullPath:  "prod/db/password",
						VersionID: "version-1",
					},
				}, nil)
				provider.EXPECT().GetSecretPayload(gomock.Any(), "prod/db/password").Return([]byte("payload-1"), nil)
				engineClient.EXPECT().CreateSecret(gomock.Any(), engine.CreatingSecret{
					Path:              "prod-db-password",
					Value:             []byte("payload-1"),
					ExternalPath:      "prod/db/password",
					ExternalVersionID: "version-1",
				}).Return(nil)
			},
			want: Result{Created: 1},
		},
		{
			name: "update secret version and rotate services",
			setup: func(engineClient *engine.MockClient, provider *contracts.MockProvider) {
				existingSecret := engine.ExistingSecret{
					ID:           "parent-secret-id",
					Path:         "prod-db-password",
					ExternalPath: "prod/db/password",
					Managed:      true,
					Versions: []engine.ExistingSecretVersion{
						{
							ID:         "parent-secret-id",
							ExternalID: "version-1",
						},
						{
							ID:         "old-version-secret-id",
							ExternalID: "version-0",
						},
					},
				}

				engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{
					newService(
						"service-id",
						"api",
						engine.NewSecretRef("prod-db-password", "prod-db-password", "parent-secret-id"),
					),
				}, nil)
				engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{
					"prod-db-password": &existingSecret,
				}, nil)
				provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{
					"prod/db/password": {
						Path:      "prod/db/password",
						FullPath:  "prod/db/password",
						VersionID: "version-2",
					},
				}, nil)
				provider.EXPECT().GetSecretPayload(gomock.Any(), "prod/db/password").Return([]byte("payload-2"), nil)
				engineClient.EXPECT().CreateSecretVersion(
					gomock.Any(),
					existingSecret,
					engine.CreatingSecretVersion{
						Path:       "prod-db-password-version-2",
						ExternalID: "version-2",
						Value:      []byte("payload-2"),
					},
				).Return(engine.CreatedSecretVersion{
					ID:   "new-version-secret-id",
					Name: "prod-db-password-version-2",
				}, nil)
				engineClient.EXPECT().UpdateService(gomock.Any(), newService(
					"service-id",
					"api",
					engine.NewSecretRef("prod-db-password", "prod-db-password-version-2", "new-version-secret-id"),
				)).Return(nil)
				engineClient.EXPECT().RemoveSecret(gomock.Any(), "parent-secret-id").Return(nil)
				engineClient.EXPECT().RemoveSecret(gomock.Any(), "old-version-secret-id").Return(nil)
				engineClient.EXPECT().CreateSecret(gomock.Any(), engine.CreatingSecret{
					Path:              "prod-db-password",
					Value:             []byte("payload-2"),
					ExternalPath:      "prod/db/password",
					ExternalVersionID: "version-2",
				}).Return(nil)
			},
			want: Result{Updated: 1, Removed: 2},
		},
		{
			name: "remove old versions for managed secret on same version",
			setup: func(engineClient *engine.MockClient, provider *contracts.MockProvider) {
				existingSecret := engine.ExistingSecret{
					ID:           "parent-secret-id",
					Path:         "prod-db-password",
					ExternalPath: "prod/db/password",
					Managed:      true,
					Versions: []engine.ExistingSecretVersion{
						{
							ID:         "parent-secret-id",
							ExternalID: "version-2",
						},
						{
							ID:         "old-version-secret-id",
							ExternalID: "version-1",
						},
					},
				}

				engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{
					newService(
						"service-id",
						"api",
						engine.NewSecretRef("prod-db-password", "prod-db-password-version-1", "old-version-secret-id"),
					),
				}, nil)
				engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{
					"prod-db-password": &existingSecret,
				}, nil)
				provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{
					"prod/db/password": {
						Path:      "prod/db/password",
						FullPath:  "prod/db/password",
						VersionID: "version-2",
					},
				}, nil)
				engineClient.EXPECT().UpdateService(gomock.Any(), newService(
					"service-id",
					"api",
					engine.NewSecretRef("prod-db-password", "prod-db-password", "parent-secret-id"),
				)).Return(nil)
				engineClient.EXPECT().RemoveSecret(gomock.Any(), "old-version-secret-id").Return(nil)
			},
			want: Result{Skipped: 1, Removed: 1},
		},
		{
			name: "keep old versions for unmanaged secret on same version",
			setup: func(engineClient *engine.MockClient, provider *contracts.MockProvider) {
				existingSecret := engine.ExistingSecret{
					ID:           "parent-secret-id",
					Path:         "prod-db-password",
					ExternalPath: "prod/db/password",
					Versions: []engine.ExistingSecretVersion{
						{
							ID:         "parent-secret-id",
							ExternalID: "version-2",
						},
						{
							ID:         "old-version-secret-id",
							ExternalID: "version-1",
						},
					},
				}

				engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{
					newService(
						"service-id",
						"api",
						engine.NewSecretRef("prod-db-password", "prod-db-password-version-1", "old-version-secret-id"),
					),
				}, nil)
				engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{
					"prod-db-password": &existingSecret,
				}, nil)
				provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{
					"prod/db/password": {
						Path:      "prod/db/password",
						FullPath:  "prod/db/password",
						VersionID: "version-2",
					},
				}, nil)
				engineClient.EXPECT().UpdateService(gomock.Any(), newService(
					"service-id",
					"api",
					engine.NewSecretRef("prod-db-password", "prod-db-password", "parent-secret-id"),
				)).Return(nil)
			},
			want: Result{Skipped: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			engineClient := engine.NewMockClient(ctrl)
			provider := contracts.NewMockProvider(ctrl)

			tt.setup(engineClient, provider)

			synchronizer := NewSynchronizer(
				engineClient,
				provider,
				metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test"}).Secrets,
				false,
				secretname.FolderDelimiter('-'),
			)

			got, err := synchronizer.Sync(context.Background())
			if !assert.NoError(t, err) {
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSynchronizer_Sync_CleanupOrphanedManagedSecrets(t *testing.T) {
	t.Parallel()

	existingSecret := engine.ExistingSecret{
		ID:      "parent-secret-id",
		Path:    "prod-db-password",
		Managed: true,
		Versions: []engine.ExistingSecretVersion{
			{
				ID:         "parent-secret-id",
				ExternalID: "version-2",
			},
			{
				ID:         "old-version-secret-id",
				ExternalID: "version-1",
			},
		},
	}

	ctrl := gomock.NewController(t)
	engineClient := engine.NewMockClient(ctrl)
	provider := contracts.NewMockProvider(ctrl)

	engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{}, nil).Times(2)
	engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{
		"prod-db-password": &existingSecret,
	}, nil).Times(2)
	provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{}, nil)
	engineClient.EXPECT().RemoveSecret(gomock.Any(), "parent-secret-id").Return(nil)
	engineClient.EXPECT().RemoveSecret(gomock.Any(), "old-version-secret-id").Return(nil)

	synchronizer := NewSynchronizer(
		engineClient,
		provider,
		metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test"}).Secrets,
		true,
		secretname.FolderDelimiter('-'),
	)

	got, err := synchronizer.Sync(context.Background())
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, Result{Removed: 2}, got)
}

func TestSynchronizer_Sync_KeepManagedSecretUsedByServiceID(t *testing.T) {
	t.Parallel()

	existingSecret := engine.ExistingSecret{
		ID:      "parent-secret-id",
		Path:    "prod-db-password",
		Managed: true,
		Versions: []engine.ExistingSecretVersion{
			{
				ID:         "parent-secret-id",
				ExternalID: "version-2",
			},
			{
				ID:         "old-version-secret-id",
				ExternalID: "version-1",
			},
		},
	}

	ctrl := gomock.NewController(t)
	engineClient := engine.NewMockClient(ctrl)
	provider := contracts.NewMockProvider(ctrl)

	engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{
		newService(
			"service-id",
			"api",
			newSecretRef("mounted-name", "prod-db-password-version-1", "old-version-secret-id"),
		),
	}, nil).Times(2)
	engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{
		"prod-db-password": &existingSecret,
	}, nil).Times(2)
	provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{}, nil)

	synchronizer := NewSynchronizer(
		engineClient,
		provider,
		metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test"}).Secrets,
		true,
		secretname.FolderDelimiter('-'),
	)

	got, err := synchronizer.Sync(context.Background())
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, Result{}, got)
}

func TestSynchronizer_Sync_KeepManagedSecretPresentInExternalSource(t *testing.T) {
	t.Parallel()

	existingSecret := engine.ExistingSecret{
		ID:      "parent-secret-id",
		Path:    "prod-db-password",
		Managed: true,
		Versions: []engine.ExistingSecretVersion{
			{
				ID:         "parent-secret-id",
				ExternalID: "version-2",
			},
			{
				ID:         "old-version-secret-id",
				ExternalID: "version-1",
			},
		},
	}

	ctrl := gomock.NewController(t)
	engineClient := engine.NewMockClient(ctrl)
	provider := contracts.NewMockProvider(ctrl)

	engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{}, nil).Times(2)
	engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{
		"prod-db-password": &existingSecret,
	}, nil).Times(2)
	provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{
		"prod/db/password": {
			Path:      "prod/db/password",
			FullPath:  "prod/db/password",
			VersionID: "version-2",
		},
	}, nil)
	engineClient.EXPECT().RemoveSecret(gomock.Any(), "old-version-secret-id").Return(nil)

	synchronizer := NewSynchronizer(
		engineClient,
		provider,
		metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test"}).Secrets,
		true,
		secretname.FolderDelimiter('-'),
	)

	got, err := synchronizer.Sync(context.Background())
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, Result{Skipped: 1, Removed: 1}, got)
}

func TestSynchronizer_Sync_SkipUnmanagedOrphanedSecretCleanup(t *testing.T) {
	t.Parallel()

	existingSecret := engine.ExistingSecret{
		ID:   "parent-secret-id",
		Path: "prod-db-password",
		Versions: []engine.ExistingSecretVersion{
			{
				ID: "parent-secret-id",
			},
			{
				ID: "old-version-secret-id",
			},
		},
	}

	ctrl := gomock.NewController(t)
	engineClient := engine.NewMockClient(ctrl)
	provider := contracts.NewMockProvider(ctrl)

	engineClient.EXPECT().ListServices(gomock.Any()).Return([]swarm.Service{}, nil).Times(2)
	engineClient.EXPECT().MapSecrets(gomock.Any()).Return(map[string]*engine.ExistingSecret{
		"prod-db-password": &existingSecret,
	}, nil).Times(2)
	provider.EXPECT().ListSecrets(gomock.Any()).Return(map[string]contracts.Secret{}, nil)

	synchronizer := NewSynchronizer(
		engineClient,
		provider,
		metrics.NewGroup(metrics.CreateGroupParams{Namespace: "test"}).Secrets,
		true,
		secretname.FolderDelimiter('-'),
	)

	got, err := synchronizer.Sync(context.Background())
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, Result{}, got)
}

func newService(id string, name string, secrets ...*swarm.SecretReference) swarm.Service { //nolint:unparam // test
	return swarm.Service{
		ID: id,
		Spec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name: name,
			},
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{
					Secrets: secrets,
				},
			},
		},
	}
}

func newSecretRef(fileName string, secretName string, id string) *swarm.SecretReference {
	return &swarm.SecretReference{
		File: &swarm.SecretReferenceFileTarget{
			Name: fileName,
			UID:  "0",
			GID:  "0",
			Mode: 0444,
		},
		SecretName: secretName,
		SecretID:   id,
	}
}
