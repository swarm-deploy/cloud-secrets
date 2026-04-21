package config

import (
	"log/slog"
	"time"

	"github.com/swarm-deploy/cloud-secrets/internal/providers/cloudru"
)

type Config struct {
	CloudRu cloudru.Config `envPrefix:"CLOUDRU_"`

	SwarmSecrets struct {
		RefreshInterval time.Duration `env:"REFRESH_INTERVAL" envDefault:"5m"`

		Log struct {
			Level slog.Level `env:"LEVEL"`
		} `envPrefix:"LOG_"`
	} `envPrefix:"CS_"`
}
