package cloudru

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/caarlos0/env/v11"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestCertificateManagerConfig(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, value      string
		enabled, wantErr bool
	}{
		{name: "default disabled"}, {name: "explicit disabled", value: "false"},
		{name: "opt in", value: "true", enabled: true}, {name: "invalid flag", value: "wrong", wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			credentials := filepath.Join(t.TempDir(), "iam")
			require.NoError(t, os.WriteFile(credentials, []byte("credential"), 0600))
			environment := map[string]string{"CLOUDRU_PROJECT_ID": "project",
				"CLOUDRU_IAM_CLIENT_ID": credentials, "CLOUDRU_IAM_CLIENT_SECRET": credentials}
			if tt.value != "" {
				environment["CLOUDRU_CERTIFICATE_MANAGER_ENABLED"] = tt.value
			}
			var cfg Config
			err := env.ParseWithOptions(&cfg, env.Options{Prefix: "CLOUDRU_", Environment: environment})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.enabled, cfg.CertificateManager.Enabled)
		})
	}
}
