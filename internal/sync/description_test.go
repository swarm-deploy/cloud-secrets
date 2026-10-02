package sync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/contracts"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
	"go.uber.org/mock/gomock"
)

func TestSynchronizer_getUpdatedSecretPayload_PreservesDescription(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := contracts.NewMockProvider(ctrl)
	provider.EXPECT().GetSecretPayload(gomock.Any(), "prod/db/password").Return([]byte("payload"), nil)

	s := &Synchronizer{
		secretProvider:  provider,
		folderDelimiter: secretname.FolderDelimiter('-'),
	}

	got, err := s.getUpdatedSecretPayload(context.Background(), contracts.Secret{
		Path:        "prod/db/password",
		VersionID:   "version-2",
		Description: "Database password",
	})

	assert.NoError(t, err)
	assert.Equal(t, "prod-db-password-version-2", got.Path)
	assert.Equal(t, "version-2", got.ExternalID)
	assert.Equal(t, "Database password", got.Description)
	assert.Equal(t, []byte("payload"), got.Value)
}
