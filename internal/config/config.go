package config

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru"
	"github.com/swarm-deploy/cloud-secrets/internal/secretname"
)

type Config struct {
	CloudRu cloudru.Config `envPrefix:"CLOUDRU_"`

	CloudSecrets struct {
		RefreshInterval time.Duration `env:"REFRESH_INTERVAL" envDefault:"5m"`

		SecretNameFolderDelimiter secretname.FolderDelimiter `env:"SECRET_NAME_FOLDER_DELIMITER" envDefault:"-"`

		Log struct {
			Level slog.Level `env:"LEVEL"`
		} `envPrefix:"LOG_"`
	} `envPrefix:"CS_"`
}

func Load() (*Config, error) {
	cfg, err := env.ParseAs[Config]()
	if err != nil {
		return nil, fmt.Errorf("read env: %w", err)
	}

	if err = cfg.CloudSecrets.SecretNameFolderDelimiter.Validate(); err != nil {
		return nil, fmt.Errorf("validate CS_SECRET_NAME_FOLDER_DELIMITER: %w", err)
	}

	if err = cfg.CloudRu.Validate(); err != nil {
		return nil, fmt.Errorf("validate cloudru config: %w", err)
	}

	return &cfg, nil
}
