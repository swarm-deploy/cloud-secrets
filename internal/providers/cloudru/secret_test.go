package cloudru

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProvider_resolveSecretPath(t *testing.T) {
	t.Parallel()

	provider := Provider{
		cfg: Config{
			RootFolder:           RootFolder("prod/apps"),
			RootFolderOmitPrefix: true,
		},
	}

	assert.Equal(t, "prod/apps/api/db/password", provider.resolveSecretPath("api/db/password"))
	assert.Equal(t, "prod/apps/api/db/password", provider.resolveSecretPath("prod/apps/api/db/password"))
}

func TestProvider_scopeSecretPath(t *testing.T) {
	t.Parallel()

	t.Run("keeps full path when omit prefix disabled", func(t *testing.T) {
		t.Parallel()

		provider := Provider{
			cfg: Config{
				RootFolder: RootFolder("prod/apps"),
			},
		}

		path, err := provider.scopeSecretPath("prod/apps/api/db/password")
		assert.NoError(t, err)
		assert.Equal(t, "prod/apps/api/db/password", path)
	})

	t.Run("omits configured root prefix", func(t *testing.T) {
		t.Parallel()

		provider := Provider{
			cfg: Config{
				RootFolder:           RootFolder("prod/apps"),
				RootFolderOmitPrefix: true,
			},
		}

		path, err := provider.scopeSecretPath("prod/apps/api/db/password")
		assert.NoError(t, err)
		assert.Equal(t, "api/db/password", path)
	})

	t.Run("fails for path outside root folder", func(t *testing.T) {
		t.Parallel()

		provider := Provider{
			cfg: Config{
				RootFolder:           RootFolder("prod/apps"),
				RootFolderOmitPrefix: true,
			},
		}

		_, err := provider.scopeSecretPath("prod/other/api/db/password")
		assert.EqualError(t, err, "path is outside root folder \"prod/apps\"")
	})
}
