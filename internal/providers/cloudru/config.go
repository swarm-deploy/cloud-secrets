package cloudru

import "errors"

type Config struct {
	IAM struct {
		Address string `env:"ADDRESS"`

		ClientID     string `env:"CLIENT_ID,file"`
		ClientSecret string `env:"CLIENT_SECRET,file" json:"-"`
	} `envPrefix:"IAM_"`
	CSM struct {
		Address string `env:"ADDRESS"`
	} `envPrefix:"CSM_"`

	DiscoveryURL string `env:"DISCOVERY_URL"`

	ProjectID string `env:"PROJECT_ID"`
}

func (c *Config) Validate() error {
	if c.ProjectID != "" {
		return errors.New("CLOUDRU_PROJECT_ID is required")
	}

	if c.IAM.ClientID == "" {
		return errors.New("CLOUDRU_IAM_CLIENT_ID is required")
	}

	if c.IAM.ClientSecret == "" {
		return errors.New("CLOUDRU_IAM_CLIENT_SECRET is required")
	}

	return nil
}
