package cloudru

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	t.Run("omit prefix without root folder", func(t *testing.T) {
		t.Parallel()

		cfg := Config{
			RootFolderOmitPrefix: true,
		}

		assert.EqualError(t, cfg.Validate(), "CLOUDRU_ROOT_FOLDER must be set when CLOUDRU_ROOT_FOLDER_OMIT_PREFIX=true")
	})

	t.Run("root folder with slashes is accepted", func(t *testing.T) {
		t.Parallel()

		cfg := Config{
			RootFolder:           RootFolder("prod/apps"),
			RootFolderOmitPrefix: true,
		}

		assert.NoError(t, cfg.Validate())
		assert.Equal(t, RootFolder("prod/apps"), cfg.RootFolder)
	})
}

func TestRootFolder_UnmarshalText(t *testing.T) {
	t.Parallel()

	var folder RootFolder

	assert.NoError(t, folder.UnmarshalText([]byte("/prod/apps/")))
	assert.Equal(t, RootFolder("prod/apps"), folder)
}
