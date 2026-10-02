package cloudru

import (
	"testing"

	v2 "github.com/cloudru-tech/secret-manager-sdk/api/v2"
	"github.com/stretchr/testify/assert"
)

func TestProvider_mapSecret_PreservesDescription(t *testing.T) {
	t.Parallel()

	provider := Provider{}
	mapped, err := provider.mapSecret(&v2.Secret{
		Path:        "prod/db/password",
		Description: "Database password",
		Versions: []*v2.SecretVersion{
			{Id: 7, State: v2.VersionState_ENABLED},
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, "prod/db/password", mapped.Path)
	assert.Equal(t, "prod/db/password", mapped.FullPath)
	assert.Equal(t, "7", mapped.VersionID)
	assert.Equal(t, "Database password", mapped.Description)
}
